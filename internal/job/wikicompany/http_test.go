package wikicompany

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDoWithRetry_RetriesOn429ThenSucceeds(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	resp, err := doWithRetry(srv.Client(), req, 3, 0)
	if err != nil {
		t.Fatalf("doWithRetry returned error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected final status 200, got %d", resp.StatusCode)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestDoWithRetry_GivesUpAfterMaxAttempts(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	resp, err := doWithRetry(srv.Client(), req, 2, 0)
	if err == nil {
		t.Fatal("expected an error after exhausting retries, got nil")
	}
	// No caller of doWithRetry ever reads the body on the error path, so an
	// exhausted retry must close it itself and return nil — leaving it open on a
	// returned-but-unusable response is exactly the leak a caller forgetting a
	// conditional Close() would reintroduce.
	if resp != nil {
		t.Fatalf("expected a nil response on the error path, got %+v (body would leak)", resp)
	}
	if attempts != 2 {
		t.Fatalf("expected exactly 2 attempts, got %d", attempts)
	}
}
