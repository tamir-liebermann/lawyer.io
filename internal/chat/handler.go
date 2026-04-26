package chat

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/lawyer-io/lawyer/internal/anthropic"
	"github.com/lawyer-io/lawyer/internal/realestatedata"
)

// ChatClient is the minimal surface the handler needs from an Anthropic client.
// Having an interface makes handler tests hermetic (no network).
type ChatClient interface {
	Chat(ctx context.Context, system string, history []anthropic.Message, userMsg string) (string, error)
}

// RealEstateFetcher is the minimal surface for fetching market data.
type RealEstateFetcher interface {
	SummaryForCity(ctx context.Context, city string) (string, error)
}

// Handler wires session storage, the LLM client, and the real-estate fetcher
// into the HTTP /api/chat endpoint.
type Handler struct {
	Sessions    *SessionStore
	LLM         ChatClient
	RealEstat   RealEstateFetcher // optional; may be nil
	OfficeName  string            // injected from OFFICE_NAME env var
	Logger      *log.Logger
	// ChatTimeout bounds each LLM round-trip. Default 45s if zero.
	ChatTimeout time.Duration
}

type chatRequest struct {
	Message string `json:"message"`
	// Mode is optional; if present it overrides the session user type for this
	// request AND is persisted. Only "client" or "lawyer" are accepted.
	Mode string `json:"mode,omitempty"`
}

type chatResponse struct {
	Reply            string   `json:"reply"`
	SessionID        string   `json:"session_id"`
	UserType         string   `json:"user_type"`
	SuggestedActions []string `json:"suggested_actions,omitempty"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// ServeHTTP implements http.Handler for POST /api/chat.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req chatRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "בקשה לא תקינה") // "invalid request"
		return
	}
	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" {
		writeErr(w, http.StatusBadRequest, "ההודעה ריקה") // "message empty"
		return
	}

	sess, err := h.Sessions.Get(r)
	if err != nil {
		h.logf("session get: %v", err)
		writeErr(w, http.StatusInternalServerError, "שגיאה בטעינת הסשן")
		return
	}

	// URL param ?mode=lawyer or JSON body mode overrides/persists.
	if m := r.URL.Query().Get("mode"); m != "" {
		SetUserType(sess, m)
	}
	if req.Mode != "" {
		SetUserType(sess, req.Mode)
	}

	userType := UserType(sess)
	sid := SID(sess)

	// Start real-estate fetch in background; we wait at most 2.5s so that
	// even a cache miss can complete before we build the system prompt.
	// Using context.Background() (not r.Context()) so a cancelled request
	// doesn't abort a fetch that's about to populate the cache.
	var reContext string
	if h.RealEstat != nil {
		if city := detectCityQuery(req.Message); city != "" {
			type reResult struct {
				summary string
				err     error
			}
			reCh := make(chan reResult, 1)
			go func() {
				ctx2, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
				defer cancel()
				s, err := h.RealEstat.SummaryForCity(ctx2, city)
				reCh <- reResult{s, err}
			}()
			select {
			case res := <-reCh:
				if res.err != nil {
					h.logf("real-estate summary for %q: %v", city, res.err)
				} else {
					reContext = res.summary
				}
			case <-time.After(2500 * time.Millisecond):
				h.logf("real-estate fetch for %q timed out; proceeding without context", city)
			}
		}
	}

	system := anthropic.BuildSystemPrompt(userType, h.OfficeName, reContext)
	history := h.Sessions.History(sid)

	ctx := r.Context()
	if h.ChatTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, h.ChatTimeout)
		defer cancel()
	} else {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
	}

	reply, err := h.LLM.Chat(ctx, system, history, req.Message)
	if err != nil {
		h.logf("llm chat: %v", err)
		if errors.Is(err, anthropic.ErrMissingAPIKey) {
			writeErr(w, http.StatusServiceUnavailable, "השירות אינו מוגדר (חסר מפתח API)")
			return
		}
		writeErr(w, http.StatusBadGateway, "שגיאה בשירות ה-AI, נסו שוב בעוד רגע")
		return
	}

	h.Sessions.Append(sid, req.Message, reply)

	if err := h.Sessions.Save(r, w, sess); err != nil {
		h.logf("session save: %v", err)
	}

	resp := chatResponse{
		Reply:            reply,
		SessionID:        sid,
		UserType:         userType,
		SuggestedActions: SuggestedActions(userType),
	}
	writeJSON(w, http.StatusOK, resp)
}

// ResetHandler clears the history for the current session. POST only.
func (h *Handler) ResetHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	sess, err := h.Sessions.Get(r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "שגיאה בטעינת הסשן")
		return
	}
	h.Sessions.Reset(SID(sess))
	_ = h.Sessions.Save(r, w, sess)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ModeHandler explicitly sets the user type for the current session.
// Body: {"mode": "client" | "lawyer"}.
func (h *Handler) ModeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "בקשה לא תקינה")
		return
	}
	if body.Mode != UserTypeClient && body.Mode != UserTypeLawyer {
		writeErr(w, http.StatusBadRequest, "סוג משתמש לא חוקי")
		return
	}
	sess, err := h.Sessions.Get(r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "שגיאה בטעינת הסשן")
		return
	}
	SetUserType(sess, body.Mode)
	_ = h.Sessions.Save(r, w, sess)
	writeJSON(w, http.StatusOK, map[string]string{"user_type": body.Mode})
}

// SuggestedActions returns Hebrew quick-reply chips based on user type.
func SuggestedActions(userType string) []string {
	if userType == UserTypeLawyer {
		return []string{
			"איסוף נתונים לטופס 7002",
			"איסוף נתונים לטופס 7000",
			"חיפוש עסקאות בתל אביב",
			"חוק המקרקעין סעיף 9",
		}
	}
	return []string{
		"מה להביא לפגישה ראשונה",
		"כמה זמן לוקח רישום בטאבו?",
		"מה זה מס שבח?",
		"שלבי עסקת נדל\"ן",
	}
}

func (h *Handler) logf(format string, args ...interface{}) {
	if h.Logger != nil {
		h.Logger.Printf(format, args...)
		return
	}
	log.Printf(format, args...)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("content-type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}

// detectCityQuery is a lightweight Hebrew keyword matcher. Returns "" when
// the message does not look like a market-data question. Extend freely.
var realEstateHints = []string{"עסקאות", "מחיר", "שוק", "דירות", "מחירים"}

// knownCities is a short curated list of large cities whose Hebrew name
// we scan for. The data.gov.il dataset expects the Hebrew city name.
var knownCities = []string{
	"תל אביב", "ירושלים", "חיפה", "ראשון לציון", "פתח תקווה",
	"אשדוד", "נתניה", "באר שבע", "חולון", "רמת גן", "גבעתיים",
	"הרצליה", "רעננה", "כפר סבא", "אילת", "מודיעין", "בני ברק",
}

func detectCityQuery(msg string) string {
	hasHint := false
	for _, h := range realEstateHints {
		if strings.Contains(msg, h) {
			hasHint = true
			break
		}
	}
	if !hasHint {
		return ""
	}
	for _, c := range knownCities {
		if strings.Contains(msg, c) {
			return c
		}
	}
	return ""
}

// Compile-time check that the concrete fetcher satisfies our interface.
var _ RealEstateFetcher = (*realestatedata.Fetcher)(nil)
