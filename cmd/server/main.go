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
	"syscall"
	"time"

	"github.com/lawyer-io/lawyer/internal/anthropic"
	"github.com/lawyer-io/lawyer/internal/chat"
	"github.com/lawyer-io/lawyer/internal/forms"
	"github.com/lawyer-io/lawyer/internal/realestatedata"
)

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

	sessStore := chat.NewSessionStore([]byte(secret))
	llm := anthropic.NewClient(apiKey)
	reFetcher := realestatedata.New()

	handler := &chat.Handler{
		Sessions:    sessStore,
		LLM:         llm,
		RealEstat:   reFetcher,
		Logger:      logger,
		ChatTimeout: 45 * time.Second,
	}

	mux := http.NewServeMux()
	mux.Handle("/api/chat", handler)
	mux.HandleFunc("/api/reset", handler.ResetHandler)
	mux.HandleFunc("/api/mode", handler.ModeHandler)
	mux.HandleFunc("/api/forms", formsHandler)
	mux.HandleFunc("/api/realestate", realEstateHandler(reFetcher))
	mux.HandleFunc("/healthz", healthz)

	// Static assets + default root.
	mux.Handle("/", http.FileServer(http.Dir("web/static")))

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
