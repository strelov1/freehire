// Package realtime mints short-lived OpenAI Realtime API client secrets through the
// same OpenAI-compatible gateway internal/platform/llm and internal/ai/speech already use.
//
// It is not part of internal/platform/llm, which is built on langchaingo and has no
// speech-to-speech surface. It shares the gateway with internal/ai/speech: one base URL and
// key serve chat, transcription and realtime, via internal/platform/aigateway.
package realtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/strelov1/freehire/internal/platform/aigateway"
)

// ErrUpstream is what every failure on the gateway's side wraps: a refusal, a fault, an
// unreadable answer. Callers render it as a 502.
var ErrUpstream = errors.New("realtime gateway")

// requestTimeout bounds minting one client secret (a single small round-trip).
const requestTimeout = 15 * time.Second

// maxResponse bounds what is read back; a client secret is a short token.
const maxResponse = 1 << 16

// Client mints Realtime API client secrets through one gateway.
type Client struct {
	gw *aigateway.Client
}

// New builds a client, or returns nil when the gateway is not configured.
//
// Nil means this deployment has no voice mode (as in internal/ai/speech): the handler
// renders 501, which the SPA reads as an absent surface rather than a fault.
func New(baseURL, apiKey, model string) *Client {
	gw := aigateway.New(aigateway.Config{
		BaseURL:     baseURL,
		APIKey:      apiKey,
		Model:       model,
		Timeout:     requestTimeout,
		MaxResponse: maxResponse,
		ErrUpstream: ErrUpstream,
	})
	if gw == nil {
		return nil
	}
	return &Client{gw: gw}
}

// mintRequest is the client_secrets request body: a realtime session naming its model and
// carrying instructions like a system prompt.
//
// audio.input is required in practice though the gateway accepts its absence: input
// transcription is off by default, so the caller's own side would never appear as text.
// turnDetection is set because an unset value falls back to the model's default.
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
// server_vad is chosen over semantic_vad, which waits longer when unsure a turn ended: the
// opposite of what a snappy rehearsal call wants.
const (
	turnDetectionType  = "server_vad"
	transcriptionModel = "gpt-realtime-whisper"
)

// Model names which Realtime model the client secret is minted for. The WebRTC SDP exchange
// names it on its URL, so the browser must learn it here rather than hardcode it.
func (c *Client) Model() string {
	return c.gw.Model()
}

// CallsURL is where the browser POSTs its WebRTC SDP offer: this gateway, never
// api.openai.com. The MintClientSecret value is the gateway's own wrapped token (an
// encrypted upstream key plus routing metadata), redeemable only at its /realtime/calls.
func (c *Client) CallsURL() string {
	return c.gw.URL("/realtime/calls")
}

// MintClientSecret returns a short-lived credential scoped to one Realtime session.
func (c *Client) MintClientSecret(ctx context.Context, instructions string) (string, error) {
	var payload mintRequest
	payload.Session.Type = "realtime"
	payload.Session.Model = c.gw.Model()
	payload.Session.Instructions = instructions
	payload.Session.Audio.Input.TurnDetection.Type = turnDetectionType
	payload.Session.Audio.Input.Transcription.Model = transcriptionModel

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("%w: build request: %w", ErrUpstream, err)
	}
	raw, err := c.gw.Post(ctx, "/realtime/client_secrets", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
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
