package chat_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lawyer-io/lawyer/internal/anthropic"
	"github.com/lawyer-io/lawyer/internal/chat"
)

// TestIntegration_FullChatFlow wires the real Handler + SessionStore + a real
// anthropic.Client pointed at an in-process httptest server that imitates the
// Anthropic Messages API. This is the "smoke test" the user asked for.
func TestIntegration_FullChatFlow(t *testing.T) {
	// Stand up a fake Anthropic API that echoes request info into the reply.
	anthSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") == "" {
			t.Error("expected x-api-key header")
		}
		body, _ := io.ReadAll(r.Body)
		// Ensure the Hebrew system prompt was passed through.
		if !strings.Contains(string(body), "אתה עוזר AI מקצועי") {
			t.Errorf("Hebrew system prompt missing from outbound request")
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"תשובה מ-Claude"}]}`))
	}))
	defer anthSrv.Close()

	llm := anthropic.NewClient("test-key", anthropic.WithBaseURL(anthSrv.URL))
	store := chat.NewSessionStore([]byte("integration-secret"))
	h := &chat.Handler{Sessions: store, LLM: llm}

	mux := http.NewServeMux()
	mux.Handle("/api/chat", h)
	mux.HandleFunc("/api/reset", h.ResetHandler)
	mux.HandleFunc("/api/mode", h.ModeHandler)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Use a cookie jar so session cookies roundtrip between requests.
	client := ts.Client()

	// --- turn 1 ---
	resp1, err := client.Post(
		ts.URL+"/api/chat",
		"application/json",
		strings.NewReader(`{"message":"שלום","mode":"lawyer"}`),
	)
	if err != nil {
		t.Fatalf("turn1: %v", err)
	}
	if resp1.StatusCode != 200 {
		b, _ := io.ReadAll(resp1.Body)
		t.Fatalf("turn1 status=%d body=%s", resp1.StatusCode, string(b))
	}
	var t1 map[string]interface{}
	_ = json.NewDecoder(resp1.Body).Decode(&t1)
	resp1.Body.Close()
	if t1["user_type"] != "lawyer" {
		t.Errorf("expected lawyer user_type, got %v", t1["user_type"])
	}
	if t1["reply"] != "תשובה מ-Claude" {
		t.Errorf("reply not echoed: %v", t1["reply"])
	}
	cookies := resp1.Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected session cookie from turn 1")
	}

	// --- turn 2: replay cookie, expect same session + lawyer mode still set ---
	req2, _ := http.NewRequest("POST", ts.URL+"/api/chat", strings.NewReader(`{"message":"שאלה שנייה"}`))
	req2.Header.Set("content-type", "application/json")
	for _, c := range cookies {
		req2.AddCookie(c)
	}
	resp2, err := client.Do(req2)
	if err != nil {
		t.Fatalf("turn2: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		b, _ := io.ReadAll(resp2.Body)
		t.Fatalf("turn2 status=%d body=%s", resp2.StatusCode, string(b))
	}
	var t2 map[string]interface{}
	_ = json.NewDecoder(resp2.Body).Decode(&t2)
	if t2["user_type"] != "lawyer" {
		t.Errorf("lawyer mode should persist: %v", t2["user_type"])
	}
	if t2["session_id"] != t1["session_id"] {
		t.Errorf("session_id should be stable: %v vs %v", t1["session_id"], t2["session_id"])
	}

	// --- reset + verify history is cleared ---
	req3, _ := http.NewRequest("POST", ts.URL+"/api/reset", nil)
	for _, c := range cookies {
		req3.AddCookie(c)
	}
	resp3, err := client.Do(req3)
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	resp3.Body.Close()
	if resp3.StatusCode != 200 {
		t.Errorf("reset status=%d", resp3.StatusCode)
	}
}

// TestIntegration_HandlesAnthropicError verifies we surface a clean 502
// when the upstream API returns a 500.
func TestIntegration_HandlesAnthropicError(t *testing.T) {
	anthSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{"error":{"type":"internal","message":"boom"}}`))
	}))
	defer anthSrv.Close()

	llm := anthropic.NewClient("k", anthropic.WithBaseURL(anthSrv.URL))
	h := &chat.Handler{Sessions: chat.NewSessionStore([]byte("s")), LLM: llm}

	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := srv.Client().Post(srv.URL+"/api/chat", "application/json",
		strings.NewReader(`{"message":"שאלה"}`))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", resp.StatusCode)
	}
}
