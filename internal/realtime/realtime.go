// Package realtime mints short-lived OpenAI Realtime API client secrets through the
// same OpenAI-compatible gateway internal/llm and internal/speech already use.
//
// It is deliberately not part of internal/llm: that package is built on langchaingo,
// which models chat completions and has no speech-to-speech surface. What it shares
// with internal/speech is the gateway — the same base URL and key serve
// /chat/completions, /audio/transcriptions, and /realtime/client_secrets alike — so a
// deployment configures one credential and names three models, not three credentials.
package realtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrUpstream is what every failure on the gateway's side wraps: a refusal, a fault,
// an answer we cannot read. Callers render it as a 502 — the caller of the API did
// nothing wrong, and the remedy is not theirs.
var ErrUpstream = errors.New("realtime gateway")

// requestTimeout bounds minting one client secret. It is a single small JSON
// round-trip, not the call itself, so it stays short.
const requestTimeout = 15 * time.Second

// maxResponse bounds what is read back. A client secret is a short token plus a
// little session metadata; a gateway that answers with something enormous is
// misbehaving rather than verbose.
const maxResponse = 1 << 16

// Client mints Realtime API client secrets through one gateway.
type Client struct {
	baseURL string
	apiKey  string
	model   string
	http    *http.Client
}

// New builds a client, or returns nil when the gateway is not configured.
//
// Nil is the "this deployment has no voice mode" answer, following internal/speech:
// the handler asks whether it has a client and renders 501 when it does not, which the
// SPA reads as a surface that does not exist here rather than as a fault.
func New(baseURL, apiKey, model string) *Client {
	if baseURL == "" || apiKey == "" || model == "" {
		return nil
	}
	return &Client{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		apiKey:  apiKey,
		model:   model,
		http:    &http.Client{Timeout: requestTimeout},
	}
}

// mintRequest is the client_secrets request body: a realtime session naming its
// model and carrying instructions the way a system prompt carries them for a text
// turn.
//
// audio.input is not optional in practice, even though the gateway accepts a
// request without it: input transcription is off by default, so a caller who omits
// it gets a working call whose OWN side never appears as text anywhere — a spike
// that shipped this omission produced a call the candidate could hear but never see
// themselves in. turnDetection matters for the same reason from the other
// direction: an unset value falls back to whatever default the model picks, not
// necessarily the responsiveness this deployment wants.
type mintRequest struct {
	Session struct {
		Type         string `json:"type"`
		Model        string `json:"model"`
		Instructions string `json:"instructions"`
		Audio        struct {
			Input struct {
				TurnDetection struct {
					Type string `json:"type"`
				} `json:"turn_detection"`
				Transcription struct {
					Model string `json:"model"`
				} `json:"transcription"`
			} `json:"input"`
		} `json:"audio"`
	} `json:"session"`
}

// turnDetectionType and transcriptionModel are fixed rather than configurable.
// server_vad (its own default timing, not overridden here) is chosen over the
// semantic_vad the spike used because semantic_vad is explicitly variable-latency
// by design — it waits LONGER when uncertain a turn ended — the opposite of what a
// snappy rehearsal call wants. gpt-realtime-whisper is this deployment's model
// family's own transcription model, not a second credential to configure.
const (
	turnDetectionType  = "server_vad"
	transcriptionModel = "gpt-realtime-whisper"
)

// Model names which Realtime model the client secret is minted for. The WebRTC SDP
// exchange names the model on its URL, and the caller has no other way to learn
// which one this deployment is configured with — hardcoding it in the browser would
// drift silently from REALTIME_MODEL the moment ops changes it.
func (c *Client) Model() string {
	return c.model
}

// MintClientSecret returns a short-lived credential scoped to one Realtime session.
func (c *Client) MintClientSecret(ctx context.Context, instructions string) (string, error) {
	var payload mintRequest
	payload.Session.Type = "realtime"
	payload.Session.Model = c.model
	payload.Session.Instructions = instructions
	payload.Session.Audio.Input.TurnDetection.Type = turnDetectionType
	payload.Session.Audio.Input.Transcription.Model = transcriptionModel

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("%w: build request: %w", ErrUpstream, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/realtime/client_secrets", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("%w: build request: %w", ErrUpstream, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrUpstream, err)
	}
	defer resp.Body.Close()

	// Read before switching on the status: the gateway explains its refusals in the
	// body, and that sentence is the only thing that distinguishes a bad key from a
	// rate limit in a log.
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse))
	if err != nil {
		return "", fmt.Errorf("%w: read response: %w", ErrUpstream, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%w: status %d: %s", ErrUpstream, resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var parsed struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("%w: decode response: %w", ErrUpstream, err)
	}
	if parsed.Value == "" {
		return "", fmt.Errorf("%w: response carried no client secret value", ErrUpstream)
	}
	return parsed.Value, nil
}
