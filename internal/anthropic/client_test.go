package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClient_Chat_HappyPath(t *testing.T) {
	var gotReq messagesRequest
	var gotHeaders http.Header

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header.Clone()
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &gotReq); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"שלום עולם"}]}`))
	}))
	defer srv.Close()

	c := NewClient("test-key", WithBaseURL(srv.URL))

	history := []Message{{Role: "user", Content: "hi"}, {Role: "assistant", Content: "hello"}}
	reply, err := c.Chat(context.Background(), "system prompt", history, "שאלה")
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if reply != "שלום עולם" {
		t.Fatalf("reply mismatch: got %q", reply)
	}

	if gotHeaders.Get("x-api-key") != "test-key" {
		t.Errorf("expected x-api-key=test-key, got %q", gotHeaders.Get("x-api-key"))
	}
	if gotHeaders.Get("anthropic-version") != APIVersion {
		t.Errorf("unexpected anthropic-version header: %q", gotHeaders.Get("anthropic-version"))
	}
	if gotReq.Model != DefaultModel {
		t.Errorf("expected model %q, got %q", DefaultModel, gotReq.Model)
	}
	if gotReq.System != "system prompt" {
		t.Errorf("system prompt lost: got %q", gotReq.System)
	}
	if len(gotReq.Messages) != 3 {
		t.Fatalf("expected 3 messages (2 history + 1 new), got %d", len(gotReq.Messages))
	}
	if gotReq.Messages[2].Role != "user" || gotReq.Messages[2].Content != "שאלה" {
		t.Errorf("new user message not appended correctly: %+v", gotReq.Messages[2])
	}
}

func TestClient_Chat_MissingAPIKey(t *testing.T) {
	c := NewClient("")
	_, err := c.Chat(context.Background(), "sys", nil, "hi")
	if err == nil {
		t.Fatal("expected error when api key is empty")
	}
	if err != ErrMissingAPIKey {
		t.Errorf("expected ErrMissingAPIKey, got %v", err)
	}
}

func TestClient_Chat_APIErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"type":"internal","message":"boom"}}`))
	}))
	defer srv.Close()

	c := NewClient("k", WithBaseURL(srv.URL))
	_, err := c.Chat(context.Background(), "", nil, "hi")
	if err == nil {
		t.Fatal("expected error on 500")
	}
	if !strings.Contains(err.Error(), "status=500") {
		t.Errorf("error should mention status=500, got: %v", err)
	}
}

func TestClient_Chat_APIErrorBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK) // 2xx but error payload
		_, _ = w.Write([]byte(`{"error":{"type":"overloaded_error","message":"try later"}}`))
	}))
	defer srv.Close()

	c := NewClient("k", WithBaseURL(srv.URL))
	_, err := c.Chat(context.Background(), "", nil, "hi")
	if err == nil {
		t.Fatal("expected error from error payload")
	}
	if !strings.Contains(err.Error(), "overloaded_error") {
		t.Errorf("error should mention overloaded_error, got: %v", err)
	}
}

func TestClient_Chat_JoinsMultipleTextBlocks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"חלק א'"},{"type":"text","text":" חלק ב'"},{"type":"tool_use","text":"ignored"}]}`))
	}))
	defer srv.Close()

	c := NewClient("k", WithBaseURL(srv.URL))
	reply, err := c.Chat(context.Background(), "", nil, "שאלה")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reply != "חלק א' חלק ב'" {
		t.Errorf("text concatenation wrong: got %q", reply)
	}
}

func TestBuildSystemPrompt_SelectsAddendum(t *testing.T) {
	client := BuildSystemPrompt("client", "", "")
	lawyer := BuildSystemPrompt("lawyer", "", "")
	if !strings.Contains(client, "מצב נוכחי: לקוח") {
		t.Error("client addendum missing")
	}
	if !strings.Contains(lawyer, "מצב נוכחי: עורך/ת דין") {
		t.Error("lawyer addendum missing")
	}
	if strings.Contains(client, "מצב נוכחי: עורך/ת דין") {
		t.Error("client prompt leaked lawyer addendum")
	}
}

func TestBuildSystemPrompt_InjectsRealEstateContext(t *testing.T) {
	got := BuildSystemPrompt("lawyer", "", "נתונים על גבעתיים: 15 עסקאות")
	if !strings.Contains(got, "נתונים על גבעתיים") {
		t.Error("real-estate context not injected")
	}
}

func TestBuildSystemPrompt_DefaultsToClient(t *testing.T) {
	got := BuildSystemPrompt("", "", "")
	if !strings.Contains(got, "מצב נוכחי: לקוח") {
		t.Error("empty user type should default to client mode")
	}
}
