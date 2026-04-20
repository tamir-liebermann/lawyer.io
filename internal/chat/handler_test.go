package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/lawyer-io/lawyer/internal/anthropic"
)

// --- mocks ---

type fakeLLM struct {
	mu          sync.Mutex
	systemSeen  []string
	historySeen [][]anthropic.Message
	userSeen    []string
	reply       string
	err         error
}

func (f *fakeLLM) Chat(_ context.Context, system string, history []anthropic.Message, userMsg string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.systemSeen = append(f.systemSeen, system)
	f.historySeen = append(f.historySeen, append([]anthropic.Message(nil), history...))
	f.userSeen = append(f.userSeen, userMsg)
	if f.err != nil {
		return "", f.err
	}
	return f.reply, nil
}

type fakeRE struct {
	city    string
	summary string
	err     error
}

func (f *fakeRE) SummaryForCity(_ context.Context, city string) (string, error) {
	f.city = city
	return f.summary, f.err
}

// --- helpers ---

func newHandler(llm ChatClient, re RealEstateFetcher) (*Handler, *SessionStore) {
	s := NewSessionStore([]byte("test-secret"))
	h := &Handler{Sessions: s, LLM: llm, RealEstat: re}
	return h, s
}

func post(t *testing.T, h *Handler, path, body string, cookies []*http.Cookie) (*http.Response, []byte) {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("content-type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	if path == "/api/reset" {
		h.ResetHandler(w, req)
	} else if path == "/api/mode" || strings.HasPrefix(path, "/api/mode?") {
		h.ModeHandler(w, req)
	} else {
		h.ServeHTTP(w, req)
	}
	resp := w.Result()
	b, _ := io.ReadAll(resp.Body)
	return resp, b
}

// --- tests ---

func TestHandler_HappyPath_ClientMode(t *testing.T) {
	llm := &fakeLLM{reply: "תשובה"}
	h, _ := newHandler(llm, nil)

	resp, body := post(t, h, "/api/chat", `{"message":"שלום"}`, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["reply"] != "תשובה" {
		t.Errorf("reply mismatch: %v", got["reply"])
	}
	if got["user_type"] != "client" {
		t.Errorf("expected default client mode, got %v", got["user_type"])
	}
	if got["session_id"] == nil || got["session_id"] == "" {
		t.Error("session_id missing")
	}
	if !strings.Contains(llm.systemSeen[0], "מצב נוכחי: לקוח") {
		t.Error("client addendum missing from system prompt")
	}
}

func TestHandler_LawyerMode_Persists(t *testing.T) {
	llm := &fakeLLM{reply: "ok"}
	h, _ := newHandler(llm, nil)

	// First request: set lawyer mode via body
	resp1, _ := post(t, h, "/api/chat", `{"message":"שאלה","mode":"lawyer"}`, nil)
	if resp1.StatusCode != 200 {
		t.Fatalf("first call failed: %d", resp1.StatusCode)
	}
	cookies := resp1.Cookies()

	// Second request: no mode in body, but cookie should preserve lawyer.
	resp2, body2 := post(t, h, "/api/chat", `{"message":"שאלה 2"}`, cookies)
	if resp2.StatusCode != 200 {
		t.Fatalf("second call failed: %d body=%s", resp2.StatusCode, body2)
	}
	var got map[string]interface{}
	_ = json.Unmarshal(body2, &got)
	if got["user_type"] != "lawyer" {
		t.Errorf("lawyer mode not persisted, got %v", got["user_type"])
	}
	if !strings.Contains(llm.systemSeen[1], "מצב נוכחי: עורך/ת דין") {
		t.Error("lawyer addendum missing from second request's system prompt")
	}
}

func TestHandler_URLParamMode(t *testing.T) {
	llm := &fakeLLM{reply: "ok"}
	h, _ := newHandler(llm, nil)

	req := httptest.NewRequest("POST", "/api/chat?mode=lawyer", strings.NewReader(`{"message":"שאלה"}`))
	req.Header.Set("content-type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	resp := w.Result()
	b, _ := io.ReadAll(resp.Body)
	var got map[string]interface{}
	_ = json.Unmarshal(b, &got)
	if got["user_type"] != "lawyer" {
		t.Errorf("URL param ?mode=lawyer not applied: %v", got["user_type"])
	}
}

func TestHandler_RejectsNonPOST(t *testing.T) {
	h, _ := newHandler(&fakeLLM{reply: "ok"}, nil)
	req := httptest.NewRequest("GET", "/api/chat", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Result().StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Result().StatusCode)
	}
}

func TestHandler_RejectsInvalidJSON(t *testing.T) {
	h, _ := newHandler(&fakeLLM{reply: "ok"}, nil)
	resp, _ := post(t, h, "/api/chat", `{not json`, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHandler_RejectsEmptyMessage(t *testing.T) {
	h, _ := newHandler(&fakeLLM{reply: "ok"}, nil)
	resp, _ := post(t, h, "/api/chat", `{"message":"   "}`, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHandler_LLMErrorMissingKey(t *testing.T) {
	llm := &fakeLLM{err: anthropic.ErrMissingAPIKey}
	h, _ := newHandler(llm, nil)
	resp, _ := post(t, h, "/api/chat", `{"message":"שאלה"}`, nil)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", resp.StatusCode)
	}
}

func TestHandler_LLMGenericError(t *testing.T) {
	llm := &fakeLLM{err: errors.New("boom")}
	h, _ := newHandler(llm, nil)
	resp, _ := post(t, h, "/api/chat", `{"message":"שאלה"}`, nil)
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", resp.StatusCode)
	}
}

func TestHandler_InjectsRealEstateContext(t *testing.T) {
	re := &fakeRE{summary: "גבעתיים: 10 עסקאות"}
	llm := &fakeLLM{reply: "ok"}
	h, _ := newHandler(llm, re)

	body := `{"message":"מה המחירים בגבעתיים?","mode":"lawyer"}`
	resp, _ := post(t, h, "/api/chat", body, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if re.city != "גבעתיים" {
		t.Errorf("fetcher should be called with city גבעתיים, got %q", re.city)
	}
	if !strings.Contains(llm.systemSeen[0], "גבעתיים: 10 עסקאות") {
		t.Error("real-estate summary not injected into system prompt")
	}
}

func TestHandler_NoRealEstateFetchWithoutHint(t *testing.T) {
	re := &fakeRE{summary: "שלא יגיע"}
	llm := &fakeLLM{reply: "ok"}
	h, _ := newHandler(llm, re)
	// message mentions a city but no market hint keyword.
	_, _ = post(t, h, "/api/chat", `{"message":"שלום מגבעתיים"}`, nil)
	if re.city != "" {
		t.Errorf("fetcher should not be called without hint, was called with %q", re.city)
	}
}

func TestHandler_SessionHistoryGrowsAcrossCalls(t *testing.T) {
	llm := &fakeLLM{reply: "תשובה"}
	h, _ := newHandler(llm, nil)

	resp1, _ := post(t, h, "/api/chat", `{"message":"ראשון"}`, nil)
	cookies := resp1.Cookies()
	_, _ = post(t, h, "/api/chat", `{"message":"שני"}`, cookies)

	if len(llm.historySeen) != 2 {
		t.Fatalf("expected 2 LLM calls, got %d", len(llm.historySeen))
	}
	// Second call should include the first user+assistant pair in history.
	if len(llm.historySeen[1]) != 2 {
		t.Errorf("expected 2 msgs in history on second call, got %d", len(llm.historySeen[1]))
	}
}

func TestResetHandler_ClearsHistory(t *testing.T) {
	llm := &fakeLLM{reply: "תשובה"}
	h, s := newHandler(llm, nil)

	resp1, _ := post(t, h, "/api/chat", `{"message":"ראשון"}`, nil)
	cookies := resp1.Cookies()
	_, _ = post(t, h, "/api/reset", ``, cookies)

	// Retrieve the sid to check history was cleared.
	r := httptest.NewRequest("GET", "/", nil)
	for _, c := range cookies {
		r.AddCookie(c)
	}
	sess, _ := s.Get(r)
	if got := s.History(SID(sess)); len(got) != 0 {
		t.Errorf("expected empty history after reset, got %d messages", len(got))
	}
}

func TestModeHandler_SetsAndRejects(t *testing.T) {
	h, _ := newHandler(&fakeLLM{reply: "ok"}, nil)

	resp, body := post(t, h, "/api/mode", `{"mode":"lawyer"}`, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, body)
	}

	resp2, _ := post(t, h, "/api/mode", `{"mode":"nope"}`, nil)
	if resp2.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 on invalid mode, got %d", resp2.StatusCode)
	}
}

func TestSuggestedActions_DifferByMode(t *testing.T) {
	client := SuggestedActions(UserTypeClient)
	lawyer := SuggestedActions(UserTypeLawyer)
	if len(client) == 0 || len(lawyer) == 0 {
		t.Fatal("suggestions should be non-empty for both modes")
	}
	// They shouldn't be identical.
	if strings.Join(client, "|") == strings.Join(lawyer, "|") {
		t.Error("client and lawyer suggested actions should differ")
	}
}

func TestDetectCityQuery(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"מה המחירים בגבעתיים?", "גבעתיים"},
		{"עסקאות בתל אביב בחודש האחרון", "תל אביב"},
		{"שוק הדירות בירושלים", "ירושלים"},
		{"שלום מה שלומך", ""},                   // no hint, no city
		{"שלום מגבעתיים", ""},                   // city but no hint
		{"מחיר דירה ברמת גן", "רמת גן"},
	}
	for _, tc := range cases {
		got := detectCityQuery(tc.in)
		if got != tc.want {
			t.Errorf("detectCityQuery(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// sanity: response Content-Type is JSON.
func TestHandler_ResponseIsJSON(t *testing.T) {
	h, _ := newHandler(&fakeLLM{reply: "ok"}, nil)
	resp, _ := post(t, h, "/api/chat", `{"message":"שאלה"}`, nil)
	ct := resp.Header.Get("content-type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content-type should be json, got %q", ct)
	}
}

// sanity: decoder respects max bytes.
func TestHandler_RejectsOversizedBody(t *testing.T) {
	h, _ := newHandler(&fakeLLM{reply: "ok"}, nil)
	big := bytes.Repeat([]byte("a"), 1<<17) // 128 KB > 64 KB limit
	req := httptest.NewRequest("POST", "/api/chat", bytes.NewReader(big))
	req.Header.Set("content-type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Result().StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 on oversized body, got %d", w.Result().StatusCode)
	}
}
