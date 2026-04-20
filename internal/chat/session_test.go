package chat

import (
	"net/http/httptest"
	"testing"

	"github.com/lawyer-io/lawyer/internal/anthropic"
)

func TestSessionStore_Get_AssignsSID(t *testing.T) {
	s := NewSessionStore([]byte("secret"))
	r := httptest.NewRequest("GET", "/", nil)
	sess, err := s.Get(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if SID(sess) == "" {
		t.Fatal("SID should be assigned on first Get")
	}
}

func TestUserType_DefaultsToClient(t *testing.T) {
	s := NewSessionStore([]byte("secret"))
	r := httptest.NewRequest("GET", "/", nil)
	sess, _ := s.Get(r)
	if got := UserType(sess); got != UserTypeClient {
		t.Errorf("default user type should be client, got %q", got)
	}
}

func TestSetUserType_ValidatesValues(t *testing.T) {
	s := NewSessionStore([]byte("secret"))
	r := httptest.NewRequest("GET", "/", nil)
	sess, _ := s.Get(r)

	SetUserType(sess, "lawyer")
	if UserType(sess) != UserTypeLawyer {
		t.Error("lawyer should stick")
	}
	SetUserType(sess, "hacker") // invalid, should be ignored
	if UserType(sess) != UserTypeLawyer {
		t.Error("invalid value should not overwrite")
	}
	SetUserType(sess, "client")
	if UserType(sess) != UserTypeClient {
		t.Error("client should stick")
	}
}

func TestSessionStore_AppendAndHistory(t *testing.T) {
	s := NewSessionStore([]byte("secret"))
	sid := "sid-123"
	s.Append(sid, "שלום", "שלום לך")
	s.Append(sid, "מה שלומך", "טוב")

	hist := s.History(sid)
	if len(hist) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(hist))
	}
	if hist[0].Role != "user" || hist[0].Content != "שלום" {
		t.Errorf("first message wrong: %+v", hist[0])
	}
	if hist[1].Role != "assistant" {
		t.Errorf("second message should be assistant, got %s", hist[1].Role)
	}
}

func TestSessionStore_HistoryCap(t *testing.T) {
	s := NewSessionStore([]byte("secret"))
	sid := "sid-cap"
	// Append more than MaxHistoryTurns pairs.
	total := MaxHistoryTurns + 5
	for i := 0; i < total; i++ {
		s.Append(sid, "u", "a")
	}
	hist := s.History(sid)
	if len(hist) != MaxHistoryTurns*2 {
		t.Fatalf("expected trimmed history of %d, got %d", MaxHistoryTurns*2, len(hist))
	}
}

func TestSessionStore_Reset(t *testing.T) {
	s := NewSessionStore([]byte("secret"))
	sid := "sid-reset"
	s.Append(sid, "u", "a")
	if len(s.History(sid)) == 0 {
		t.Fatal("precondition failed: history empty after append")
	}
	s.Reset(sid)
	if len(s.History(sid)) != 0 {
		t.Fatal("Reset should clear history")
	}
}

func TestSessionStore_HistoryCopyIsIndependent(t *testing.T) {
	s := NewSessionStore([]byte("secret"))
	sid := "sid-copy"
	s.Append(sid, "u1", "a1")
	hist := s.History(sid)
	hist[0].Content = "mutated"

	hist2 := s.History(sid)
	if hist2[0].Content != "u1" {
		t.Error("History returned a shared slice; callers can corrupt internal state")
	}
	_ = anthropic.Message{} // keep import
}

func TestSessionStore_PersistsAcrossRequests(t *testing.T) {
	s := NewSessionStore([]byte("secret"))

	// 1st request: set user type, record cookie from response.
	r1 := httptest.NewRequest("GET", "/", nil)
	w1 := httptest.NewRecorder()
	sess1, _ := s.Get(r1)
	SetUserType(sess1, "lawyer")
	if err := s.Save(r1, w1, sess1); err != nil {
		t.Fatalf("save: %v", err)
	}
	sid1 := SID(sess1)
	cookies := w1.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected a session cookie on response")
	}

	// 2nd request: replay cookie, expect same sid and user type.
	r2 := httptest.NewRequest("GET", "/", nil)
	for _, c := range cookies {
		r2.AddCookie(c)
	}
	sess2, _ := s.Get(r2)
	if SID(sess2) != sid1 {
		t.Errorf("sid not preserved: %q vs %q", SID(sess2), sid1)
	}
	if UserType(sess2) != UserTypeLawyer {
		t.Errorf("user type not preserved: %s", UserType(sess2))
	}
}
