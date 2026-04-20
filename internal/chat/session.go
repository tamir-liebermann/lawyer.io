// Package chat owns session state and the HTTP chat handler.
//
// We use gorilla/sessions (cookie store) for the user-type flag, and an
// in-process sync.Map keyed by the session's stable ID for conversation
// history. History stays off the cookie to avoid the 4KB cookie cap.
package chat

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/sessions"

	"github.com/lawyer-io/lawyer/internal/anthropic"
)

const (
	CookieName = "lawyer_session"

	keyUserType = "user_type"
	keySID      = "sid"

	UserTypeClient = "client"
	UserTypeLawyer = "lawyer"

	// MaxHistoryTurns bounds per-session turns to keep token usage sane.
	MaxHistoryTurns = 20
)

// SessionStore wraps gorilla's CookieStore and an in-memory history map.
type SessionStore struct {
	cs      *sessions.CookieStore
	history sync.Map // sid -> *sessionData
}

type sessionData struct {
	mu       sync.Mutex
	messages []anthropic.Message
	updated  time.Time
}

// NewSessionStore builds a SessionStore. The secret should come from
// SESSION_SECRET env var and be long and random in production.
func NewSessionStore(secret []byte) *SessionStore {
	cs := sessions.NewCookieStore(secret)
	cs.Options = &sessions.Options{
		Path:     "/",
		HttpOnly: true,
		MaxAge:   60 * 60 * 24 * 7, // 7 days
		SameSite: http.SameSiteLaxMode,
	}
	return &SessionStore{cs: cs}
}

// Get returns the gorilla session, creating a stable sid if missing.
func (s *SessionStore) Get(r *http.Request) (*sessions.Session, error) {
	sess, err := s.cs.Get(r, CookieName)
	if err != nil {
		// gorilla returns a new session alongside the error on decode failure;
		// we recover by overwriting.
		sess, _ = s.cs.New(r, CookieName)
	}
	if _, ok := sess.Values[keySID].(string); !ok {
		sess.Values[keySID] = newID()
	}
	return sess, nil
}

// UserType reads the session's user type, defaulting to client.
func UserType(sess *sessions.Session) string {
	v, _ := sess.Values[keyUserType].(string)
	if v == UserTypeLawyer {
		return UserTypeLawyer
	}
	return UserTypeClient
}

// SetUserType validates and stores the user type on the session.
func SetUserType(sess *sessions.Session, t string) {
	switch t {
	case UserTypeClient, UserTypeLawyer:
		sess.Values[keyUserType] = t
	}
}

// SID returns the stable session ID (created lazily in Get).
func SID(sess *sessions.Session) string {
	v, _ := sess.Values[keySID].(string)
	return v
}

// History returns a copy of the message history for the given sid.
func (s *SessionStore) History(sid string) []anthropic.Message {
	v, ok := s.history.Load(sid)
	if !ok {
		return nil
	}
	sd := v.(*sessionData)
	sd.mu.Lock()
	defer sd.mu.Unlock()
	out := make([]anthropic.Message, len(sd.messages))
	copy(out, sd.messages)
	return out
}

// Append adds a user/assistant pair to the session history.
func (s *SessionStore) Append(sid, userMsg, assistantMsg string) {
	v, _ := s.history.LoadOrStore(sid, &sessionData{})
	sd := v.(*sessionData)
	sd.mu.Lock()
	defer sd.mu.Unlock()
	sd.messages = append(sd.messages,
		anthropic.Message{Role: "user", Content: userMsg},
		anthropic.Message{Role: "assistant", Content: assistantMsg},
	)
	// Trim oldest turns once we exceed the cap. Each "turn" is a user+assistant pair.
	if max := MaxHistoryTurns * 2; len(sd.messages) > max {
		sd.messages = sd.messages[len(sd.messages)-max:]
	}
	sd.updated = time.Now()
}

// Reset clears the in-memory history for a sid (used by /api/reset).
func (s *SessionStore) Reset(sid string) {
	s.history.Delete(sid)
}

// Save persists the gorilla session to the cookie.
func (s *SessionStore) Save(r *http.Request, w http.ResponseWriter, sess *sessions.Session) error {
	return sess.Save(r, w)
}

func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
