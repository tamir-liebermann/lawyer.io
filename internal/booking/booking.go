// Package booking handles client meeting requests: creates a Google Calendar
// event via a service-account JWT (no external SDK) and sends an email
// notification to the office via SMTP.
//
// Both integrations are optional — if the env vars are absent the booking is
// accepted and logged without a calendar or email action.
package booking

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/smtp"
	"net/url"
	"strings"
	"time"
)

// Request holds the client's meeting details submitted from the frontend.
type Request struct {
	Name     string `json:"name"`
	Phone    string `json:"phone"`
	Email    string `json:"email"`
	Date     string `json:"date"`  // YYYY-MM-DD
	Time     string `json:"time"`  // HH:MM (24h)
	Duration int    `json:"duration"` // minutes; defaults to 60
	Topic    string `json:"topic"`
}

// Config is populated from environment variables by the caller (main.go).
type Config struct {
	// Google Calendar — service account
	ServiceAccountJSON string // full JSON string of the service account key file
	CalendarID         string // Google Calendar ID (usually the office Gmail address)

	// SMTP email notification
	SMTPHost string // e.g. smtp.gmail.com
	SMTPPort string // e.g. 587
	SMTPUser string
	SMTPPass string

	// Office details for the notification email body
	OfficeEmail string
	OfficeName  string
}

// Service executes meeting bookings.
type Service struct {
	cfg    Config
	client *http.Client
}

// New returns a Service ready to use.
func New(cfg Config) *Service {
	return &Service{
		cfg:    cfg,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

// Book creates a calendar event and/or sends an email notification.
// Both steps are attempted independently; a failure in one does not prevent the other.
func (s *Service) Book(ctx context.Context, req Request) error {
	if req.Duration <= 0 {
		req.Duration = 60
	}

	var calErr, mailErr error

	if s.cfg.CalendarID != "" && s.cfg.ServiceAccountJSON != "" {
		calErr = s.createCalendarEvent(ctx, req)
		if calErr != nil {
			log.Printf("booking: calendar event failed: %v", calErr)
		}
	}

	if s.cfg.OfficeEmail != "" && s.cfg.SMTPHost != "" {
		mailErr = s.sendNotificationEmail(req)
		if mailErr != nil {
			log.Printf("booking: email notification failed: %v", mailErr)
		}
	}

	// Return an error only when both configured actions failed.
	if calErr != nil && mailErr != nil {
		return fmt.Errorf("calendar: %v; email: %v", calErr, mailErr)
	}
	if calErr != nil && s.cfg.SMTPHost == "" {
		return calErr
	}
	if mailErr != nil && s.cfg.CalendarID == "" {
		return mailErr
	}
	return nil
}

// ───────────────────────────── Google Calendar ──────────────────────────────

type serviceAccountKey struct {
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

func (s *Service) createCalendarEvent(ctx context.Context, req Request) error {
	token, err := s.fetchAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("access token: %w", err)
	}

	start, end, err := eventTimes(req.Date, req.Time, req.Duration)
	if err != nil {
		return fmt.Errorf("parse datetime: %w", err)
	}

	topic := req.Topic
	if topic == "" {
		topic = "פגישה ייעוץ"
	}

	type dateTimeObj struct {
		DateTime string `json:"dateTime"`
		TimeZone string `json:"timeZone"`
	}
	type attendee struct {
		Email string `json:"email"`
	}
	type event struct {
		Summary     string      `json:"summary"`
		Description string      `json:"description"`
		Start       dateTimeObj `json:"start"`
		End         dateTimeObj `json:"end"`
		Attendees   []attendee  `json:"attendees,omitempty"`
	}

	ev := event{
		Summary: fmt.Sprintf("פגישה עם %s — %s", req.Name, topic),
		Description: fmt.Sprintf(
			"לקוח: %s\nטלפון: %s\nאימייל: %s\nנושא: %s",
			req.Name, req.Phone, req.Email, topic,
		),
		Start: dateTimeObj{DateTime: start, TimeZone: "Asia/Jerusalem"},
		End:   dateTimeObj{DateTime: end, TimeZone: "Asia/Jerusalem"},
	}
	if req.Email != "" {
		ev.Attendees = []attendee{{Email: req.Email}}
	}

	body, err := json.Marshal(ev)
	if err != nil {
		return err
	}

	apiURL := fmt.Sprintf(
		"https://www.googleapis.com/calendar/v3/calendars/%s/events",
		url.PathEscape(s.cfg.CalendarID),
	)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("calendar API status=%d body=%s", resp.StatusCode, string(respBody))
	}
	return nil
}

// fetchAccessToken exchanges a JWT assertion for a Google OAuth2 access token.
func (s *Service) fetchAccessToken(ctx context.Context) (string, error) {
	var sa serviceAccountKey
	if err := json.Unmarshal([]byte(s.cfg.ServiceAccountJSON), &sa); err != nil {
		return "", fmt.Errorf("parse service account JSON: %w", err)
	}
	if sa.TokenURI == "" {
		sa.TokenURI = "https://oauth2.googleapis.com/token"
	}

	jwt, err := buildJWT(sa, "https://www.googleapis.com/auth/calendar")
	if err != nil {
		return "", fmt.Errorf("build JWT: %w", err)
	}

	vals := url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {jwt},
	}
	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost, sa.TokenURI,
		strings.NewReader(vals.Encode()),
	)
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("token endpoint status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(respBody, &tok); err != nil {
		return "", err
	}
	return tok.AccessToken, nil
}

// buildJWT creates a signed RS256 JWT for Google service-account auth.
func buildJWT(sa serviceAccountKey, scope string) (string, error) {
	now := time.Now().Unix()

	headerJSON, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	claimsJSON, _ := json.Marshal(map[string]interface{}{
		"iss":   sa.ClientEmail,
		"scope": scope,
		"aud":   "https://oauth2.googleapis.com/token",
		"exp":   now + 3600,
		"iat":   now,
	})

	unsigned := base64.RawURLEncoding.EncodeToString(headerJSON) + "." +
		base64.RawURLEncoding.EncodeToString(claimsJSON)

	sig, err := rsaSign(sa.PrivateKey, unsigned)
	if err != nil {
		return "", err
	}
	return unsigned + "." + sig, nil
}

// rsaSign signs data with the PEM-encoded PKCS8 RSA private key.
func rsaSign(privateKeyPEM, data string) (string, error) {
	// Google service-account keys embed literal \n; normalise them.
	privateKeyPEM = strings.ReplaceAll(privateKeyPEM, `\n`, "\n")

	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return "", fmt.Errorf("booking: failed to decode PEM block from private key")
	}
	rawKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("booking: parse private key: %w", err)
	}
	rsaKey, ok := rawKey.(*rsa.PrivateKey)
	if !ok {
		return "", fmt.Errorf("booking: private key is not RSA")
	}
	h := sha256.Sum256([]byte(data))
	sig, err := rsa.SignPKCS1v15(rand.Reader, rsaKey, crypto.SHA256, h[:])
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(sig), nil
}

// eventTimes parses date (YYYY-MM-DD) and time (HH:MM) and returns RFC3339
// start and end strings in Asia/Jerusalem timezone.
func eventTimes(date, timeStr string, durationMin int) (start, end string, err error) {
	loc, err := time.LoadLocation("Asia/Jerusalem")
	if err != nil {
		loc = time.UTC
	}
	t, err := time.ParseInLocation("2006-01-02 15:04", date+" "+timeStr, loc)
	if err != nil {
		return "", "", fmt.Errorf("invalid date/time %q %q: %w", date, timeStr, err)
	}
	endT := t.Add(time.Duration(durationMin) * time.Minute)
	return t.Format(time.RFC3339), endT.Format(time.RFC3339), nil
}

// ──────────────────────────────── Email ─────────────────────────────────────

func (s *Service) sendNotificationEmail(req Request) error {
	subject := fmt.Sprintf("בקשת פגישה חדשה — %s", req.Name)
	body := fmt.Sprintf(
		"התקבלה בקשת פגישה חדשה במשרד %s:\n\n"+
			"שם: %s\n"+
			"טלפון: %s\n"+
			"אימייל: %s\n"+
			"תאריך: %s\n"+
			"שעה: %s\n"+
			"נושא: %s\n",
		s.cfg.OfficeName,
		req.Name, req.Phone, req.Email,
		req.Date, req.Time, req.Topic,
	)

	port := s.cfg.SMTPPort
	if port == "" {
		port = "587"
	}

	addr := s.cfg.SMTPHost + ":" + port
	auth := smtp.PlainAuth("", s.cfg.SMTPUser, s.cfg.SMTPPass, s.cfg.SMTPHost)

	mime := "MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n"
	msg := []byte(
		"From: " + s.cfg.SMTPUser + "\r\n" +
			"To: " + s.cfg.OfficeEmail + "\r\n" +
			"Subject: " + subject + "\r\n" +
			mime + "\r\n" +
			body,
	)

	// Try STARTTLS first (port 587); fall back to plain if TLS unavailable.
	err := sendWithSTARTTLS(addr, auth, s.cfg.SMTPUser, s.cfg.OfficeEmail, msg, s.cfg.SMTPHost)
	if err != nil {
		// Fallback: plain smtp.SendMail (handles TLS negotiation internally).
		return smtp.SendMail(addr, auth, s.cfg.SMTPUser, []string{s.cfg.OfficeEmail}, msg)
	}
	return nil
}

// sendWithSTARTTLS performs explicit STARTTLS negotiation.
func sendWithSTARTTLS(addr string, auth smtp.Auth, from, to string, msg []byte, host string) error {
	c, err := smtp.Dial(addr)
	if err != nil {
		return err
	}
	defer c.Close()

	tlsCfg := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
	if err := c.StartTLS(tlsCfg); err != nil {
		return err
	}
	if err := c.Auth(auth); err != nil {
		return err
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	return w.Close()
}
