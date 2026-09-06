// Package aigateway is the HTTP transport for the OpenAI-compatible gateway's non-chat
// endpoints: one request, one JSON answer, read whole.
//
// It is deliberately not part of internal/platform/llm. That package is built on
// langchaingo, which models chat completions and knows nothing about a multipart audio
// upload or a client-secret mint; bolting either onto it would reach around the
// abstraction it exists to provide. What every one of them DOES share is the endpoint —
// the same base URL and the same key serve /chat/completions, /audio/transcriptions and
// /realtime/client_secrets — so a deployment configures one credential and names one
// model per surface.
//
// It is deliberately not routed through internal/platform/safehttp either: that client
// refuses loopback and RFC1918 targets because it fetches URLs a stranger supplied, and
// this gateway is host-local and named by our own configuration.
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
	// Timeout bounds one request. A surface sets its own: minting a client secret is a
	// small JSON round-trip, transcribing minutes of audio is not.
	Timeout time.Duration
	// MaxResponse bounds what is read back, for the same reason — and because a gateway
	// answering with something enormous is misbehaving rather than verbose.
	MaxResponse int64
	// ErrUpstream is the sentinel every failure Post returns wraps. It stays the caller's
	// own so its documented error keeps matching, and so a log line still says which
	// surface was talking to the gateway.
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
// Nil is the "this deployment does not have this surface" answer: the handler asks
// whether it has a client and renders 501 when it does not, which the SPA reads as a
// feature that does not exist here rather than as a fault. Whoever wires it must put an
// untyped nil into the handler's interface — a nil *Client inside an interface is not a
// nil interface, and that mistake turns an absent feature into a panic on first use.
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

	// Read before switching on the status: the gateway explains its refusals in the
	// body, and that sentence is the only thing that distinguishes a bad key from a
	// rate limit in a log.
	raw, err := io.ReadAll(io.LimitReader(resp.Body, c.maxResponse))
	if err != nil {
		return nil, fmt.Errorf("%w: read response: %w", c.errUpstream, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: status %d: %s", c.errUpstream, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return raw, nil
}
