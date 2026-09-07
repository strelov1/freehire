package browseruse

import (
	"context"
	"encoding/json"
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
	id, err := c.CreateRun(context.Background(), "do the thing", 0.5)
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
	if _, err := c.CreateRun(context.Background(), "task", 0); err == nil {
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
	if _, err := c.CreateRun(context.Background(), "task", 0); err == nil {
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
