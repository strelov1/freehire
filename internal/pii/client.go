package pii

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// Detector returns PII spans for a text. The production implementation is the local
// openai/privacy-filter span-detection endpoint (HTTPDetector); tests use a fake.
type Detector interface {
	Detect(ctx context.Context, text string) ([]Span, error)
}

// HTTPDetector calls the co-located privacy-filter detection endpoint. The endpoint owns
// the model and returns spans already mapped to our Kind vocabulary.
type HTTPDetector struct {
	url    string
	client *http.Client
}

// NewHTTPDetector builds a detector posting to url; client defaults to http.DefaultClient.
func NewHTTPDetector(url string, client *http.Client) *HTTPDetector {
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPDetector{url: url, client: client}
}

type detectRequest struct {
	Text string `json:"text"`
}

type detectResponse struct {
	Spans []Span `json:"spans"`
}

// Detect posts the text and returns the endpoint's spans. A transport error or a non-2xx
// response is returned as an error so the caller can fail closed.
func (d *HTTPDetector) Detect(ctx context.Context, text string) ([]Span, error) {
	body, err := json.Marshal(detectRequest{Text: text})
	if err != nil {
		return nil, fmt.Errorf("pii: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("pii: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pii: detect request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("pii: detector returned status %d", resp.StatusCode)
	}
	var out detectResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("pii: decode response: %w", err)
	}
	return out.Spans, nil
}
