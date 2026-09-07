// Package browseruse is a thin HTTP client for the browser-use.com cloud API (v4):
// create a run, poll its status, fetch its result, and manage the workspace/file-upload
// endpoints a run's own attached files come from. It knows nothing about ATS forms,
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

// RunOptions carries CreateRun's optional per-run settings beyond the task text and cost
// cap. The zero value (no workspace, no attached files) is every caller's need before
// file-upload support existed, so it stays a valid, common argument.
type RunOptions struct {
	// WorkspaceID and AttachedFileIDs both come from CreateWorkspace/RequestFileUpload —
	// a run can only see files already uploaded into the workspace it names here.
	WorkspaceID     string
	AttachedFileIDs []string
}

// CreateRun starts one run with the given task instruction and returns its id.
// maxCostUSD is the v4 API's own per-run spend cap (0 omits it, leaving the account's
// default in effect).
func (c *Client) CreateRun(ctx context.Context, task string, maxCostUSD float64, opts RunOptions) (runID string, err error) {
	body := struct {
		Task            string   `json:"task"`
		MaxCostUSD      float64  `json:"maxCostUsd,omitempty"`
		WorkspaceID     string   `json:"workspaceId,omitempty"`
		AttachedFileIDs []string `json:"attachedFileIds,omitempty"`
	}{Task: task, MaxCostUSD: maxCostUSD, WorkspaceID: opts.WorkspaceID, AttachedFileIDs: opts.AttachedFileIDs}

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

// CreateWorkspace creates a new, empty workspace and returns its id — the container a
// run's attached files live in. A fresh workspace per attempt keeps one candidate's résumé
// from ever being reachable from another's run.
func (c *Client) CreateWorkspace(ctx context.Context) (workspaceID string, err error) {
	var resp struct {
		ID string `json:"id"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/workspaces", struct{}{}, &resp); err != nil {
		return "", err
	}
	if resp.ID == "" {
		return "", fmt.Errorf("browseruse: create workspace: response carried no id")
	}
	return resp.ID, nil
}

// DeleteWorkspace permanently removes a workspace and everything uploaded into it. Callers
// that create a workspace to attach a candidate's résumé should delete it once the run is
// done — nothing here needs that file to outlive the one attempt it was rendered for.
func (c *Client) DeleteWorkspace(ctx context.Context, workspaceID string) error {
	return c.doJSON(ctx, http.MethodDelete, "/workspaces/"+workspaceID, nil, nil)
}

// FileUpload is one file's presigned upload target, returned by RequestFileUpload.
type FileUpload struct {
	ID        string // pass in RunOptions.AttachedFileIDs to attach this file to a run
	UploadURL string // PUT the file's bytes here directly (5 min expiry) — see UploadFile
}

// RequestFileUpload reserves storage for one file in workspaceID and returns a presigned
// PUT url for it — the v4 API's own two-step upload: reserve here, then PUT the bytes
// straight to storage (UploadFile) rather than through this API. size must be the file's
// exact byte count; the presigned URL is pinned to it.
func (c *Client) RequestFileUpload(ctx context.Context, workspaceID, name, contentType string, size int64) (FileUpload, error) {
	type item struct {
		Name        string `json:"name"`
		ContentType string `json:"contentType,omitempty"`
		Size        int64  `json:"size"`
	}
	body := struct {
		Files []item `json:"files"`
	}{Files: []item{{Name: name, ContentType: contentType, Size: size}}}

	var resp struct {
		Files []struct {
			ID        string `json:"id"`
			UploadURL string `json:"uploadUrl"`
		} `json:"files"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/workspaces/"+workspaceID+"/files/upload", body, &resp); err != nil {
		return FileUpload{}, err
	}
	if len(resp.Files) != 1 || resp.Files[0].ID == "" || resp.Files[0].UploadURL == "" {
		return FileUpload{}, fmt.Errorf("browseruse: request file upload: unexpected response shape")
	}
	return FileUpload{ID: resp.Files[0].ID, UploadURL: resp.Files[0].UploadURL}, nil
}

// UploadFile PUTs data to a presigned upload URL from RequestFileUpload. This bypasses
// doJSON deliberately: a presigned URL points at object storage, not this API's own base
// URL, needs no X-Browser-Use-API-Key (the URL itself is the credential, valid 5 minutes),
// and answers with no JSON body to decode.
func (c *Client) UploadFile(ctx context.Context, uploadURL, contentType string, data []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("browseruse: build upload request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	req.ContentLength = int64(len(data))

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("browseruse: upload file: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("browseruse: upload file: status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
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
