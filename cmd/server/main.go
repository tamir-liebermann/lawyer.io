// Command server boots the lawyer.io MVP HTTP server.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/lawyer-io/lawyer/internal/anthropic"
	"github.com/lawyer-io/lawyer/internal/booking"
	"github.com/lawyer-io/lawyer/internal/chat"
	"github.com/lawyer-io/lawyer/internal/forms"
	"github.com/lawyer-io/lawyer/internal/realestatedata"
	"github.com/lawyer-io/lawyer/internal/whatsapp"
)

// staticDir is the directory the Vite build writes to. Go serves it as-is.
const staticDir = "web/static"

// officeConfig holds the configurable details of the law office.
type officeConfig struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Phone   string `json:"phone"`
	Email   string `json:"email"`
	Hours   string `json:"hours"`
}

func loadOfficeConfig() officeConfig {
	return officeConfig{
		Name:    getenv("OFFICE_NAME", "Lawyer.io"),
		Address: getenv("OFFICE_ADDRESS", ""),
		Phone:   getenv("OFFICE_PHONE", ""),
		Email:   getenv("OFFICE_EMAIL", ""),
		Hours:   getenv("OFFICE_HOURS", ""),
	}
}

func main() {
	if err := run(); err != nil {
		log.Fatalf("fatal: %v", err)
	}
}

func run() error {
	logger := log.New(os.Stdout, "lawyer ", log.LstdFlags|log.Lmsgprefix)

	port := getenv("PORT", "8080")
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	secret := os.Getenv("SESSION_SECRET")
	if secret == "" {
		// Dev fallback: warn loudly. In production this should be a long random string.
		logger.Println("WARN: SESSION_SECRET not set, using dev default (do NOT use in production)")
		secret = "dev-only-session-secret-please-change-me"
	}

	office := loadOfficeConfig()
	bookingSvc := booking.New(booking.Config{
		ServiceAccountJSON: getenv("GOOGLE_SERVICE_ACCOUNT_JSON", ""),
		CalendarID:         getenv("GOOGLE_CALENDAR_ID", ""),
		SMTPHost:           getenv("SMTP_HOST", ""),
		SMTPPort:           getenv("SMTP_PORT", "587"),
		SMTPUser:           getenv("SMTP_USER", ""),
		SMTPPass:           getenv("SMTP_PASS", ""),
		OfficeEmail:        office.Email,
		OfficeName:         office.Name,
	})
	sessStore := chat.NewSessionStore([]byte(secret))
	llm := anthropic.NewClient(apiKey)
	reFetcher := realestatedata.New()

	handler := &chat.Handler{
		Sessions:    sessStore,
		LLM:         llm,
		RealEstat:   reFetcher,
		OfficeName:  office.Name,
		Logger:      logger,
		ChatTimeout: 45 * time.Second,
	}

	mux := http.NewServeMux()
	mux.Handle("/api/chat", handler)
	mux.HandleFunc("/api/reset", handler.ResetHandler)
	mux.HandleFunc("/api/mode", handler.ModeHandler)
	mux.HandleFunc("/api/forms", formsHandler)
	mux.HandleFunc("/api/forms/extract", formExtractHandler(llm))
	mux.HandleFunc("/api/forms/pdf", formPDFHandler)
	mux.HandleFunc("/api/realestate", realEstateHandler(reFetcher))
	mux.HandleFunc("/api/office", officeHandler(office))
	mux.HandleFunc("/api/book", bookHandler(bookingSvc))
	mux.Handle("/api/whatsapp", &whatsapp.Handler{
		LLM:        llm,
		RealEstat:  reFetcher,
		OfficeName: office.Name,
		Logger:     logger,
	})
	mux.HandleFunc("/healthz", healthz)

	// Static assets + SPA fallback for the React client.
	mux.Handle("/", spaHandler(staticDir))

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           securityHeaders(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	idle := make(chan struct{})
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		<-sigCh
		logger.Println("shutdown signal received")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		close(idle)
	}()

	logger.Printf("listening on :%s", port)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	<-idle
	logger.Println("server stopped")
	return nil
}

func bookHandler(svc *booking.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req booking.Request
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&req); err != nil {
			http.Error(w, "בקשה לא תקינה", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Date) == "" || strings.TrimSpace(req.Time) == "" {
			http.Error(w, "שם, תאריך ושעה הם שדות חובה", http.StatusBadRequest)
			return
		}
		if err := svc.Book(r.Context(), req); err != nil {
			log.Printf("book: %v", err)
			http.Error(w, "שגיאה בקביעת הפגישה, נסה שוב", http.StatusInternalServerError)
			return
		}
		w.Header().Set("content-type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}
}

func formExtractHandler(llm *anthropic.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			FormID   string `json:"form_id"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
			http.Error(w, "בקשה לא תקינה", http.StatusBadRequest)
			return
		}
		form, ok := forms.FindForm(body.FormID)
		if !ok {
			http.Error(w, "טופס לא נמצא", http.StatusBadRequest)
			return
		}

		var sb strings.Builder
		for _, m := range body.Messages {
			role := "משתמש"
			if m.Role == "assistant" {
				role = "עוזר"
			}
			sb.WriteString(role + ": " + m.Content + "\n\n")
		}

		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()

		extracted, err := forms.Extract(ctx, llm, form, sb.String())
		if err != nil {
			log.Printf("forms extract: %v", err)
			http.Error(w, "שגיאה בחילוץ הנתונים", http.StatusInternalServerError)
			return
		}

		coll, _ := forms.NewCollector(form.ID)
		for k, v := range extracted {
			_ = coll.Set(k, v)
		}

		resp := map[string]interface{}{
			"form_id":   form.ID,
			"form_name": form.NameHE,
			"values":    extracted,
			"missing":   coll.MissingFields(),
			"summary":   coll.SummaryHebrew(),
		}
		w.Header().Set("content-type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func formPDFHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		FormID string            `json:"form_id"`
		Values map[string]string `json:"values"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		http.Error(w, "בקשה לא תקינה", http.StatusBadRequest)
		return
	}
	if body.FormID == "" {
		http.Error(w, "form_id חסר", http.StatusBadRequest)
		return
	}
	debug := r.URL.Query().Get("debug") == "1"
	pdf, err := forms.FillPDF(body.FormID, body.Values, debug)
	if err != nil {
		log.Printf("formPDF: %v", err)
		http.Error(w, "שגיאה ביצירת ה-PDF", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="`+body.FormID+`-filled.pdf"`)
	_, _ = w.Write(pdf)
}

func officeHandler(cfg officeConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("content-type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(cfg)
	}
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("content-type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// formsHandler returns the list of supported forms as JSON (for the frontend).
func formsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("content-type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"forms": forms.SupportedForms})
}

// realEstateHandler exposes the data.gov.il summary as its own endpoint so the
// frontend can call it directly when the user clicks a "חיפוש עסקאות" chip.
func realEstateHandler(f *realestatedata.Fetcher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		city := r.URL.Query().Get("city")
		if city == "" {
			http.Error(w, "missing city", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		summary, err := f.SummaryForCity(ctx, city)
		if err != nil {
			log.Printf("realestate: %v", err)
			http.Error(w, "fetch failed", http.StatusBadGateway)
			return
		}
		w.Header().Set("content-type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]string{"city": city, "summary": summary})
	}
}

// spaHandler serves files from dir when they exist and otherwise falls back
// to index.html so client-side routes resolve against the React app. API
// paths (/api/...) are never handled here — they are registered on the mux
// before the catch-all "/".
func spaHandler(dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Defensive: if /api slipped through, let it 404 rather than return HTML.
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		// Normalise the path and check whether it points to an existing file.
		clean := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/"))
		if clean == "." || clean == "/" {
			serveIndex(w, r, dir)
			return
		}
		full := filepath.Join(dir, clean)
		info, err := os.Stat(full)
		if err != nil || info.IsDir() {
			serveIndex(w, r, dir)
			return
		}
		fs.ServeHTTP(w, r)
	})
}

func serveIndex(w http.ResponseWriter, r *http.Request, dir string) {
	index := filepath.Join(dir, "index.html")
	if _, err := os.Stat(index); err != nil {
		http.Error(w, "client bundle missing; run: cd web/client && npm run build", http.StatusInternalServerError)
		return
	}
	// Disable caching for the shell so users pick up new bundles promptly.
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, index)
}

func securityHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.ServeHTTP(w, r)
	})
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
