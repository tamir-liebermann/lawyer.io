// Package anthropic is a minimal HTTP client for Anthropic's Messages API.
// We deliberately use net/http rather than the SDK to keep the dependency
// surface small and tests hermetic via httptest.
package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	DefaultBaseURL = "https://api.anthropic.com"
	DefaultModel   = "claude-sonnet-4-20250514"
	APIVersion     = "2023-06-01"
	DefaultMaxTok  = 1024
)

// Message is a single turn in the conversation history.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Client talks to the Anthropic Messages API.
type Client struct {
	apiKey    string
	baseURL   string
	model     string
	maxTokens int
	http      *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the default API base URL (used in tests).
func WithBaseURL(u string) Option { return func(c *Client) { c.baseURL = u } }

// WithHTTPClient overrides the default http.Client.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// WithModel overrides the default model.
func WithModel(m string) Option { return func(c *Client) { c.model = m } }

// WithMaxTokens overrides the default max tokens.
func WithMaxTokens(n int) Option { return func(c *Client) { c.maxTokens = n } }

// NewClient creates a Client. apiKey may be empty for tests that use
// WithBaseURL to point at a test server — in that case pass a placeholder.
func NewClient(apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:    apiKey,
		baseURL:   DefaultBaseURL,
		model:     DefaultModel,
		maxTokens: DefaultMaxTok,
		http:      &http.Client{Timeout: 30 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

type messagesRequest struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system,omitempty"`
	Messages  []Message `json:"messages"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type apiError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type messagesResponse struct {
	Content []contentBlock `json:"content"`
	Error   *apiError      `json:"error,omitempty"`
}

// ErrMissingAPIKey is returned when the client was constructed without a key.
var ErrMissingAPIKey = errors.New("anthropic: missing API key")

// Chat sends a single-turn request (history + userMsg) and returns the
// concatenated text of the assistant's response.
func (c *Client) Chat(ctx context.Context, system string, history []Message, userMsg string) (string, error) {
	if c.apiKey == "" {
		return "", ErrMissingAPIKey
	}

	msgs := make([]Message, 0, len(history)+1)
	msgs = append(msgs, history...)
	msgs = append(msgs, Message{Role: "user", Content: userMsg})

	body, err := json.Marshal(messagesRequest{
		Model:     c.model,
		MaxTokens: c.maxTokens,
		System:    system,
		Messages:  msgs,
	})
	if err != nil {
		return "", fmt.Errorf("anthropic: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("anthropic: new request: %w", err)
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", APIVersion)
	req.Header.Set("content-type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("anthropic: do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("anthropic: read body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("anthropic: API error status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var mr messagesResponse
	if err := json.Unmarshal(respBody, &mr); err != nil {
		return "", fmt.Errorf("anthropic: decode response: %w (body=%s)", err, string(respBody))
	}
	if mr.Error != nil {
		return "", fmt.Errorf("anthropic: %s: %s", mr.Error.Type, mr.Error.Message)
	}

	var out bytes.Buffer
	for _, b := range mr.Content {
		if b.Type == "text" {
			out.WriteString(b.Text)
		}
	}
	return out.String(), nil
}
