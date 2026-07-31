// Package speech turns recorded audio into text through an OpenAI-compatible
// gateway.
//
// It is deliberately not part of internal/llm. That package is built on langchaingo,
// which models chat completions and has no audio surface at all; a multipart audio
// POST bolted onto it would be reaching around the abstraction it exists to provide.
// What it does share is the endpoint: the same gateway and the same key serve
// /chat/completions and /audio/transcriptions, so the two are configured together.
package speech

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// ErrUpstream is what every failure on the gateway's side wraps: a refusal, a fault,
// an answer we cannot read. Callers render it as a 502 — the caller of the API did
// nothing wrong, and the remedy is not theirs.
var ErrUpstream = errors.New("speech gateway")

// requestTimeout bounds one transcription. It is generous because the audio may be
// minutes long and the gateway transcribes it in one pass, and it is finite because
// a client that waits forever holds a connection and a goroutine for as long as the
// gateway is willing to be silent.
const requestTimeout = 2 * time.Minute

// Client transcribes audio through one gateway.
type Client struct {
	baseURL string
	apiKey  string
	model   string
	http    *http.Client
}

// New builds a client, or returns nil when the gateway is not configured.
//
// Nil is the "this deployment has no speech" answer, following internal/headshot:
// the handler asks whether it has a client and renders 501 when it does not, which
// the SPA reads as a surface that does not exist here rather than as a fault.
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

// Transcribe returns what was said in audio.
//
// The filename travels to the gateway unchanged because that is how the container is
// identified upstream — a recording named .webm and a recording named .mp4 are read
// by different decoders, and the browser is the only party that knows which one it
// produced.
//
// An empty result is a success, not a failure: it is what silence transcribes to.
func (c *Client) Transcribe(ctx context.Context, audio []byte, filename string) (string, error) {
	body, contentType, err := transcriptionBody(audio, filename, c.model)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/audio/transcriptions", body)
	if err != nil {
		return "", fmt.Errorf("%w: build request: %w", ErrUpstream, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", contentType)

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
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("%w: decode response: %w", ErrUpstream, err)
	}
	return parsed.Text, nil
}

// maxResponse bounds what is read back. A transcription of an upload we already
// capped cannot be large, and a gateway that answers with something enormous is
// misbehaving rather than verbose.
const maxResponse = 1 << 20

// transcriptionBody builds the multipart form the endpoint expects: the audio under
// "file", and the two fields that say which model to run and how to answer.
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
