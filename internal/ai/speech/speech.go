// Package speech turns recorded audio into text through an OpenAI-compatible
// gateway.
//
// It is not part of internal/platform/llm: langchaingo has no audio surface. The same
// gateway and key serve /chat/completions and /audio/transcriptions; the HTTP half is
// internal/platform/aigateway, shared with internal/api/realtime.
package speech

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"time"

	"github.com/strelov1/freehire/internal/platform/aigateway"
)

// ErrUpstream wraps every failure on the gateway's side (a refusal, a fault, an
// unreadable answer). Callers render it as a 502.
var ErrUpstream = errors.New("speech gateway")

// requestTimeout bounds one transcription: long audio is transcribed in one pass, but
// it must be finite so a silent gateway cannot hold a connection and goroutine forever.
const requestTimeout = 2 * time.Minute

// Client transcribes audio through one gateway.
type Client struct {
	gw *aigateway.Client
}

// New builds a client, or returns nil when the gateway is not configured. Nil means
// this deployment has no speech: the handler renders 501 (as with internal/candidate/headshot).
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

// Transcribe returns what was said in audio.
//
// The filename goes to the gateway unchanged: its extension (.webm vs .mp4) selects the
// decoder, and only the browser knows the container it produced.
//
// An empty result is a success: it is what silence transcribes to.
func (c *Client) Transcribe(ctx context.Context, audio []byte, filename string) (string, error) {
	body, contentType, err := transcriptionBody(audio, filename, c.gw.Model())
	if err != nil {
		return "", err
	}
	raw, err := c.gw.Post(ctx, "/audio/transcriptions", contentType, body)
	if err != nil {
		return "", err
	}

	var parsed struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("%w: decode response: %w", ErrUpstream, err)
	}
	return parsed.Text, nil
}

// maxResponse bounds what is read back; an enormous answer means a misbehaving gateway.
const maxResponse = 1 << 20

// transcriptionBody builds the multipart form: the audio under "file", plus the model
// and response_format fields.
func transcriptionBody(audio []byte, filename, model string) (io.Reader, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		return nil, "", fmt.Errorf("%w: build body: %w", ErrUpstream, err)
	}
	if _, err := part.Write(audio); err != nil {
		return nil, "", fmt.Errorf("%w: build body: %w", ErrUpstream, err)
	}
	for field, value := range map[string]string{"model": model, "response_format": "json"} {
		if err := w.WriteField(field, value); err != nil {
			return nil, "", fmt.Errorf("%w: build body: %w", ErrUpstream, err)
		}
	}
	if err := w.Close(); err != nil {
		return nil, "", fmt.Errorf("%w: build body: %w", ErrUpstream, err)
	}
	return &buf, w.FormDataContentType(), nil
}
