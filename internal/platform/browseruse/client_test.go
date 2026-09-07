package browseruse

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCreateRun_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/runs" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("X-Browser-Use-API-Key"); got != "test-key" {
			t.Fatalf("api key header = %q, want test-key", got)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["task"] != "do the thing" {
			t.Fatalf("task = %v, want %q", body["task"], "do the thing")
		}
		if body["maxCostUsd"] != 0.5 {
			t.Fatalf("maxCostUsd = %v, want 0.5", body["maxCostUsd"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"run-123","status":"queued"}`))
	}))
	defer srv.Close()

	c := New("test-key", srv.URL, nil)
	id, err := c.CreateRun(context.Background(), "do the thing", 0.5, RunOptions{})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if id != "run-123" {
		t.Errorf("id = %q, want run-123", id)
	}
}

func TestCreateRun_NonEmptyIDRequired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"","status":"queued"}`))
	}))
	defer srv.Close()

	c := New("test-key", srv.URL, nil)
	if _, err := c.CreateRun(context.Background(), "task", 0, RunOptions{}); err == nil {
		t.Fatal("want an error for an empty run id")
	}
}

func TestPollStatus_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/runs/run-123/status" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"status":"running"}`))
	}))
	defer srv.Close()

	c := New("test-key", srv.URL, nil)
	status, err := c.PollStatus(context.Background(), "run-123")
	if err != nil {
		t.Fatalf("PollStatus: %v", err)
	}
	if status != "running" {
		t.Errorf("status = %q, want running", status)
	}
}

func TestGetResult_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/runs/run-123" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"id":"run-123","status":"completed","result":"CONFIRMED: done","error":null,"totalCostUsd":"0.0261"}`))
	}))
	defer srv.Close()

	c := New("test-key", srv.URL, nil)
	res, err := c.GetResult(context.Background(), "run-123")
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if res.Status != "completed" || res.Result != "CONFIRMED: done" || res.Error != "" {
		t.Errorf("result = %+v", res)
	}
	if res.TotalCostUSD < 0.026 || res.TotalCostUSD > 0.027 {
		t.Errorf("TotalCostUSD = %v, want ~0.0261", res.TotalCostUSD)
	}
}

func TestDoJSON_NonSuccessStatusIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"detail":"Zero Data Retention is enabled"}`))
	}))
	defer srv.Close()

	c := New("test-key", srv.URL, nil)
	if _, err := c.CreateRun(context.Background(), "task", 0, RunOptions{}); err == nil {
		t.Fatal("want an error for a non-2xx response")
	}
}

func TestWait_ReturnsResultOnceTerminal(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/runs/run-123/status":
			calls++
			if calls < 3 {
				_, _ = w.Write([]byte(`{"status":"running"}`))
			} else {
				_, _ = w.Write([]byte(`{"status":"completed"}`))
			}
		case "/runs/run-123":
			_, _ = w.Write([]byte(`{"id":"run-123","status":"completed","result":"CONFIRMED: ok","totalCostUsd":"0.01"}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := New("test-key", srv.URL, nil)
	res, err := c.Wait(context.Background(), "run-123", 5*time.Millisecond, time.Second)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if res.Status != "completed" || res.Result != "CONFIRMED: ok" {
		t.Errorf("result = %+v", res)
	}
	if calls < 3 {
		t.Errorf("status calls = %d, want at least 3 (polled until terminal)", calls)
	}
}

func TestWait_TimesOutOnAStuckRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"running"}`))
	}))
	defer srv.Close()

	c := New("test-key", srv.URL, nil)
	_, err := c.Wait(context.Background(), "run-123", 5*time.Millisecond, 30*time.Millisecond)
	if err == nil {
		t.Fatal("want a timeout error for a run stuck running")
	}
}

func TestCreateRun_PassesWorkspaceAndAttachedFileIDs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["workspaceId"] != "ws-1" {
			t.Errorf("workspaceId = %v, want ws-1", body["workspaceId"])
		}
		ids, _ := body["attachedFileIds"].([]any)
		if len(ids) != 1 || ids[0] != "file-1" {
			t.Errorf("attachedFileIds = %v, want [file-1]", body["attachedFileIds"])
		}
		_, _ = w.Write([]byte(`{"id":"run-1","status":"queued"}`))
	}))
	defer srv.Close()

	c := New("test-key", srv.URL, nil)
	if _, err := c.CreateRun(context.Background(), "task", 0, RunOptions{WorkspaceID: "ws-1", AttachedFileIDs: []string{"file-1"}}); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
}

func TestCreateWorkspace_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/workspaces" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"id":"ws-123","archived":false,"createdAt":"2026-09-07T00:00:00Z","updatedAt":"2026-09-07T00:00:00Z"}`))
	}))
	defer srv.Close()

	c := New("test-key", srv.URL, nil)
	id, err := c.CreateWorkspace(context.Background())
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if id != "ws-123" {
		t.Errorf("id = %q, want ws-123", id)
	}
}

func TestDeleteWorkspace_HappyPath(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/workspaces/ws-123" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := New("test-key", srv.URL, nil)
	if err := c.DeleteWorkspace(context.Background(), "ws-123"); err != nil {
		t.Fatalf("DeleteWorkspace: %v", err)
	}
	if !called {
		t.Error("delete request was never sent")
	}
}

func TestRequestFileUpload_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/workspaces/ws-123/files/upload" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		files, _ := body["files"].([]any)
		if len(files) != 1 {
			t.Fatalf("files = %v, want exactly one entry", body["files"])
		}
		f := files[0].(map[string]any)
		if f["name"] != "resume.pdf" || f["contentType"] != "application/pdf" || f["size"] != float64(1234) {
			t.Errorf("file entry = %+v, want name=resume.pdf contentType=application/pdf size=1234", f)
		}
		_, _ = w.Write([]byte(`{"files":[{"id":"file-1","name":"resume.pdf","storedName":"resume.pdf","path":"uploads/resume.pdf","willOverride":false,"uploadUrl":"https://storage.example.test/upload-1"}]}`))
	}))
	defer srv.Close()

	c := New("test-key", srv.URL, nil)
	upload, err := c.RequestFileUpload(context.Background(), "ws-123", "resume.pdf", "application/pdf", 1234)
	if err != nil {
		t.Fatalf("RequestFileUpload: %v", err)
	}
	if upload.ID != "file-1" || upload.UploadURL != "https://storage.example.test/upload-1" {
		t.Errorf("upload = %+v, want id=file-1 uploadUrl=https://storage.example.test/upload-1", upload)
	}
}

func TestUploadFile_PUTsBytesWithNoAPIKeyHeader(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("method = %s, want PUT", r.Method)
		}
		if got := r.Header.Get("X-Browser-Use-API-Key"); got != "" {
			t.Errorf("api key header = %q, want empty — a presigned URL carries its own credential", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/pdf" {
			t.Errorf("content-type = %q, want application/pdf", got)
		}
		b, _ := io.ReadAll(r.Body)
		gotBody = b
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New("test-key", "https://unused.example.test", nil)
	if err := c.UploadFile(context.Background(), srv.URL, "application/pdf", []byte("%PDF-1.4 fake")); err != nil {
		t.Fatalf("UploadFile: %v", err)
	}
	if string(gotBody) != "%PDF-1.4 fake" {
		t.Errorf("uploaded body = %q, want %q", gotBody, "%PDF-1.4 fake")
	}
}

func TestUploadFile_NonSuccessStatusIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := New("test-key", "https://unused.example.test", nil)
	if err := c.UploadFile(context.Background(), srv.URL, "application/pdf", []byte("data")); err == nil {
		t.Fatal("want an error for a non-2xx response")
	}
}
