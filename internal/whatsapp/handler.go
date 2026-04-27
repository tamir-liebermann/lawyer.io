package whatsapp

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/lawyer-io/lawyer/internal/anthropic"
	"github.com/lawyer-io/lawyer/internal/chat"
)

const maxWAChars = 1600

// Handler handles inbound Twilio WhatsApp webhooks.
// Twilio POSTs application/x-www-form-urlencoded with Body and From fields.
type Handler struct {
	LLM        chat.ChatClient
	RealEstat  chat.RealEstateFetcher
	OfficeName string
	Logger     *log.Logger
	sessions   sync.Map // phone → []anthropic.Message
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	from := r.FormValue("From") // e.g. "whatsapp:+972501234567"
	body := strings.TrimSpace(r.FormValue("Body"))
	if from == "" || body == "" {
		writeTwiML(w, "")
		return
	}

	var reContext string
	if h.RealEstat != nil {
		if city := detectCity(body); city != "" {
			type result struct {
				s   string
				err error
			}
			ch := make(chan result, 1)
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
				defer cancel()
				s, err := h.RealEstat.SummaryForCity(ctx, city)
				ch <- result{s, err}
			}()
			select {
			case res := <-ch:
				if res.err == nil {
					reContext = res.s
				}
			case <-time.After(2500 * time.Millisecond):
			}
		}
	}

	system := anthropic.BuildSystemPrompt("client", h.OfficeName, reContext)
	history := h.loadHistory(from)

	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()

	reply, err := h.LLM.Chat(ctx, system, history, body)
	if err != nil {
		h.logf("whatsapp llm: %v", err)
		writeTwiML(w, "מצטערים, אירעה שגיאה. נסה שוב בעוד רגע.")
		return
	}

	h.appendHistory(from, body, reply)

	if len([]rune(reply)) > maxWAChars {
		runes := []rune(reply)
		reply = string(runes[:maxWAChars-3]) + "..."
	}
	writeTwiML(w, reply)
}

func (h *Handler) loadHistory(phone string) []anthropic.Message {
	if v, ok := h.sessions.Load(phone); ok {
		return v.([]anthropic.Message)
	}
	return nil
}

func (h *Handler) appendHistory(phone, userMsg, aiReply string) {
	history := h.loadHistory(phone)
	history = append(history,
		anthropic.Message{Role: "user", Content: userMsg},
		anthropic.Message{Role: "assistant", Content: aiReply},
	)
	// Cap at 20 turns (40 messages) to avoid unbounded growth.
	if len(history) > 40 {
		history = history[len(history)-40:]
	}
	h.sessions.Store(phone, history)
}

func (h *Handler) logf(f string, args ...any) {
	if h.Logger != nil {
		h.Logger.Printf(f, args...)
	} else {
		log.Printf(f, args...)
	}
}

func writeTwiML(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><Response><Message>%s</Message></Response>`, msg)
}

// detectCity is duplicated from chat/handler.go to avoid an import cycle.
var realEstateHints = []string{"עסקאות", "מחיר", "שוק", "דירות", "מחירים"}
var knownCities = []string{
	"תל אביב", "ירושלים", "חיפה", "ראשון לציון", "פתח תקווה",
	"אשדוד", "נתניה", "באר שבע", "חולון", "רמת גן",
}

func detectCity(msg string) string {
	for _, hint := range realEstateHints {
		if strings.Contains(msg, hint) {
			for _, city := range knownCities {
				if strings.Contains(msg, city) {
					return city
				}
			}
		}
	}
	return ""
}
