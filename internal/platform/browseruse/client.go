// Package browseruse is a thin HTTP client for the browser-use.com cloud API (v4):
// create a run, poll its status, fetch its result. It knows nothing about ATS forms,
// resolved application plans, or any other domain — the same "transport, not domain"
// split internal/platform/llm keeps for its own provider-agnostic wrapper.
package browseruse

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const defaultBaseURL = "https://api.browser-use.com/api/v4"

// Client calls the browser-use cloud API.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// New builds a Client. baseURL defaults to the production API when empty — tests pass an
// httptest.Server's URL instead. httpClient defaults to a client with a generous timeout
// when nil; the caller's own context still governs any individual call.
func New(apiKey, baseURL string, httpClient *http.Client) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{baseURL: baseURL, apiKey: apiKey, http: httpClient}
}

// RunResult is one run's terminal outcome.
type RunResult struct {
	ID     string
	Status string // "completed", "failed", or "cancelled" once terminal
	// Result is the agent's final natural-language report — empty until the run is
	// terminal, and still empty on a run that failed before producing one.
	Result       string
	Error        string
	TotalCostUSD float64
}

// terminalStatuses are the run states Wait stops polling on — every value RunStatusResponse
// documents besides queued/dispatching/running.
var terminalStatuses = map[string]bool{"completed": true, "failed": true, "cancelled": true}

// CreateRun starts one run with the given task instruction and returns its id.
// maxCostUSD is the v4 API's own per-run spend cap (0 omits it, leaving the account's
// default in effect).
func (c *Client) CreateRun(ctx context.Context, task string, maxCostUSD float64) (runID string, err error) {
	body := struct {
		Task       string  `json:"task"`
		MaxCostUSD float64 `json:"maxCostUsd,omitempty"`
	}{Task: task, MaxCostUSD: maxCostUSD}

	var resp struct {
		ID string `json:"id"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/runs", body, &resp); err != nil {
		return "", err
	}
	if resp.ID == "" {
		return "", fmt.Errorf("browseruse: create run: response carried no id")
	}
	return resp.ID, nil
}

// PollStatus reads a run's current status — the cheap, indexed poll target the v4 API
// documents for repeated checks; GetResult is the fuller (and pricier) fetch.
func (c *Client) PollStatus(ctx context.Context, runID string) (status string, err error) {
	var resp struct {
		Status string `json:"status"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/runs/"+runID+"/status", nil, &resp); err != nil {
		return "", err
	}
	return resp.Status, nil
}

// GetResult fetches a run's full summary. Meaningful once the run has reached a terminal
// status; called before that, Result/Error simply read empty.
func (c *Client) GetResult(ctx context.Context, runID string) (RunResult, error) {
	var resp struct {
		ID           string  `json:"id"`
		Status       string  `json:"status"`
		Result       *string `json:"result"`
		Error        *string `json:"error"`
		TotalCostUsd string  `json:"totalCostUsd"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/runs/"+runID, nil, &resp); err != nil {
		return RunResult{}, err
	}
	out := RunResult{ID: resp.ID, Status: resp.Status}
	if resp.Result != nil {
		out.Result = *resp.Result
	}
	if resp.Error != nil {
		out.Error = *resp.Error
	}
	// Best-effort: the API's own cost figure is informational here, not a value this
	// client acts on — an unparseable string simply reads as 0 rather than failing the
	// whole result.
	if cost, err := strconv.ParseFloat(resp.TotalCostUsd, 64); err == nil {
		out.TotalCostUSD = cost
	}
	return out, nil
}

// Wait polls status every pollInterval until the run reaches a terminal state or timeout
// elapses, then fetches and returns the full result exactly once — mirroring the v4 API's
// own documented pattern (poll the cheap status target, fetch the full summary once).
func (c *Client) Wait(ctx context.Context, runID string, pollInterval, timeout time.Duration) (RunResult, error) {
	deadline := time.Now().Add(timeout)
	for {
		status, err := c.PollStatus(ctx, runID)
		if err != nil {
			return RunResult{}, err
		}
		if terminalStatuses[status] {
			return c.GetResult(ctx, runID)
		}
		if time.Now().After(deadline) {
			return RunResult{}, fmt.Errorf("browseruse: run %s did not reach a terminal state within %s", runID, timeout)
		}
		select {
		case <-ctx.Done():
			return RunResult{}, ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

func (c *Client) doJSON(ctx context.Context, method, path string, body, out any) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("browseruse: marshal request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("browseruse: build request: %w", err)
	}
	req.Header.Set("X-Browser-Use-API-Key", c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("browseruse: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("browseruse: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("browseruse: %s %s: status %d: %s", method, path, resp.StatusCode, string(data))
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("browseruse: decode response: %w", err)
		}
	}
	return nil
}
