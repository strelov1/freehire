// Package aigateway is the HTTP transport for the OpenAI-compatible gateway's non-chat
// endpoints: one request, one JSON answer, read whole.
//
// It is not part of internal/platform/llm: that package is built on langchaingo, which
// models chat completions only, not multipart audio uploads or client-secret mints. All
// surfaces share one base URL and key (/chat/completions, /audio/transcriptions,
// /realtime/client_secrets); each names its own model.
//
// It does not use internal/platform/safehttp: that client refuses loopback and RFC1918
// targets, and this gateway is host-local and named by our own configuration.
package aigateway

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Config describes one gateway surface: where it is, which credential and model it uses,
// and how long and how much of an answer it will wait for.
type Config struct {
	BaseURL string
	APIKey  string
	Model   string
	// Timeout bounds one request; each surface sets its own.
	Timeout time.Duration
	// MaxResponse bounds what is read back.
	MaxResponse int64
	// ErrUpstream is the caller's own sentinel, wrapped by every failure Post returns.
	ErrUpstream error
}

// Client talks to one gateway surface.
type Client struct {
	baseURL     string
	apiKey      string
	model       string
	maxResponse int64
	errUpstream error
	http        *http.Client
}

// New builds a client, or returns nil when the gateway is not configured — no base URL,
// no key, or no model named.
//
// Nil means this deployment has no such surface; the handler renders 501. Wire it as an
// untyped nil into the handler's interface: a nil *Client inside an interface is not a
// nil interface and panics on first use.
func New(cfg Config) *Client {
	if cfg.BaseURL == "" || cfg.APIKey == "" || cfg.Model == "" {
		return nil
	}
	return &Client{
		baseURL:     strings.TrimSuffix(cfg.BaseURL, "/"),
		apiKey:      cfg.APIKey,
		model:       cfg.Model,
		maxResponse: cfg.MaxResponse,
		errUpstream: cfg.ErrUpstream,
		http:        &http.Client{Timeout: cfg.Timeout},
	}
}

// Model is the model this surface was configured with.
func (c *Client) Model() string { return c.model }

// URL joins path onto the gateway's base URL without doubling the separating slash.
func (c *Client) URL(path string) string { return c.baseURL + path }

// Post sends body to path on the gateway and returns the whole answer.
func (c *Client) Post(ctx context.Context, path, contentType string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL(path), body)
	if err != nil {
		return nil, fmt.Errorf("%w: build request: %w", c.errUpstream, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", contentType)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", c.errUpstream, err)
	}
	defer resp.Body.Close()

	// Read before checking status: the body is the only thing that tells a bad key from a
	// rate limit.
	raw, err := io.ReadAll(io.LimitReader(resp.Body, c.maxResponse))
	if err != nil {
		return nil, fmt.Errorf("%w: read response: %w", c.errUpstream, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: status %d: %s", c.errUpstream, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return raw, nil
}
