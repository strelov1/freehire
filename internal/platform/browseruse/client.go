// Package browseruse is a thin HTTP client for the browser-use.com cloud API (v4):
// create a run, poll its status, fetch its result, and manage the workspace and file-upload
// endpoints for a run's attached files. It knows nothing about the domain (ATS forms,
// application plans), like internal/platform/llm.
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

// New builds a Client. baseURL defaults to the production API when empty; httpClient
// defaults to one with a generous timeout when nil. The caller's context still governs each call.
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
	// Result is the agent's final report; empty until terminal, and empty if the run failed first.
	Result       string
	Error        string
	TotalCostUSD float64
}

// terminalStatuses are the run states Wait stops polling on: every documented value except
// queued/dispatching/running.
var terminalStatuses = map[string]bool{"completed": true, "failed": true, "cancelled": true}

// RunOptions carries CreateRun's optional per-run settings. The zero value (no workspace,
// no attached files) is valid.
type RunOptions struct {
	// WorkspaceID and AttachedFileIDs come from CreateWorkspace/RequestFileUpload; a run
	// sees only files uploaded into the workspace it names.
	WorkspaceID     string
	AttachedFileIDs []string
}

// CreateRun starts one run with the given task instruction and returns its id.
// maxCostUSD is the per-run spend cap (0 omits it, leaving the account default).
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

// CreateWorkspace creates an empty workspace and returns its id. Use a fresh one per
// attempt so one candidate's résumé is never reachable from another's run.
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

// DeleteWorkspace permanently removes a workspace and everything in it. Delete a résumé
// workspace once the run is done.
func (c *Client) DeleteWorkspace(ctx context.Context, workspaceID string) error {
	return c.doJSON(ctx, http.MethodDelete, "/workspaces/"+workspaceID, nil, nil)
}

// FileUpload is one file's presigned upload target, returned by RequestFileUpload.
type FileUpload struct {
	ID        string // pass in RunOptions.AttachedFileIDs to attach this file to a run
	UploadURL string // PUT the file's bytes here directly (5 min expiry) — see UploadFile
}

// RequestFileUpload reserves storage for one file in workspaceID and returns a presigned
// PUT url for it: reserve here, then PUT the bytes straight to storage (UploadFile). size
// must be the file's exact byte count; the presigned URL is pinned to it.
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

// UploadFile PUTs data to a presigned upload URL from RequestFileUpload. It bypasses doJSON:
// the URL points at object storage, needs no API key (the URL is the credential, valid 5
// minutes) and returns no JSON.
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

// PollStatus reads a run's current status, the cheap target for repeated checks; GetResult
// is the fuller, pricier fetch.
func (c *Client) PollStatus(ctx context.Context, runID string) (status string, err error) {
	var resp struct {
		Status string `json:"status"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/runs/"+runID+"/status", nil, &resp); err != nil {
		return "", err
	}
	return resp.Status, nil
}

// GetResult fetches a run's full summary; before a terminal status, Result/Error read empty.
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
	// Best-effort and informational: an unparseable cost reads as 0 instead of failing.
	if cost, err := strconv.ParseFloat(resp.TotalCostUsd, 64); err == nil {
		out.TotalCostUSD = cost
	}
	return out, nil
}

// Wait polls status every pollInterval until the run reaches a terminal state or timeout
// elapses, then fetches and returns the full result exactly once.
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
