// Package jev is the transport for TypeSafe's Jev API (POST /v1/systemone): one request
// carries a state and named questions, the response carries one answer per question.
// It knows nothing about what is asked; callers own their instructions and how they
// read an answer.
package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	// BaseURL is the live API.
	BaseURL = "https://api.typesafe.ai"
	// Model is required by the API: a request without it is rejected with 422.
	Model   = "jev-latest"
	timeout = 15 * time.Second
)

// Question is one named question. Criteria is the label -> description set of a
// "choice" question and is omitted for the other types (e.g. "noul").
type Question struct {
	Type         string            `json:"type"`
	Instructions string            `json:"instructions"`
	Criteria     map[string]string `json:"criteria,omitempty"`
}

// Answer is the union of what the question types return; a caller reads the field
// its question type fills.
type Answer struct {
	Choice        string             `json:"choice"`
	Probabilities map[string]float64 `json:"probabilities"`
	Noul          *float64           `json:"noul"`
}

// Request is the POST body.
type Request struct {
	State     any                 `json:"state"`
	Model     string              `json:"model"`
	Questions map[string]Question `json:"questions"`
}

type response struct {
	Answers map[string]Answer `json:"answers"`
}

// Client makes one attempt per call, no retries: callers fall back on error themselves.
type Client struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL points the client at another endpoint (a test server).
func WithBaseURL(url string) Option {
	return func(c *Client) { c.baseURL = url }
}

// New builds a client authenticated with apiKey.
func New(apiKey string, opts ...Option) *Client {
	c := &Client{httpClient: &http.Client{
		Timeout: timeout,
		// Never follow: a 307/308 replays the POST body (profiles, emails) to the Location.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}, baseURL: BaseURL, apiKey: apiKey}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Ask sends state and questions and returns the answers by question name. Errors carry
// no package prefix; the caller adds its own.
func (c *Client) Ask(ctx context.Context, state any, questions map[string]Question) (map[string]Answer, error) {
	body, err := json.Marshal(Request{State: state, Model: Model, Questions: questions})
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/systemone", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// No body in the error: a 422 echoes the request input (e.g. an email body) into logs.
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	var parsed response
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return parsed.Answers, nil
}
