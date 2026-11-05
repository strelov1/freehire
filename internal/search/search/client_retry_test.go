package search

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/meilisearch/meilisearch-go"
)

// Regression test for the 2026-11-05 incident (fingerprint e6280e536eebc136):
// meilisearch-go v0.36.3's built-in retry re-sends an ALREADY-CONSUMED request
// body, so a POST that met a retryable 503 was retried with a zero-byte body and
// Meilisearch answered 400 missing_payload — surfacing as a logged fault from
// /api/v1/companies/sitemap (POST /indexes/companies/documents/fetch) instead of
// the transient engine 503 that caused the retry. NewClient therefore disables
// the SDK's retry loop; this test pins both halves of that decision.
//
// The stub engine answers a nonempty POST with 503 and an empty retry with
// the incident's 400 missing_payload.

// TestNewClientDisablesSDKRetry pins that NewClient's SDK has retries DISABLED:
// a request meeting a retryable 503 must surface the 503 itself in ONE attempt,
// not be re-sent (a re-send would go out with an empty body and produce the
// incident's 400 missing_payload).
func TestNewClientDisablesSDKRetry(t *testing.T) {
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("stub got %s, want POST", r.Method)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		bodies = append(bodies, string(body))
		w.Header().Set("Content-Type", "application/json")
		if len(body) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"message":"A json payload is missing.","code":"missing_payload","type":"invalid_request"}`)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"message":"engine overloaded","code":"unavailable","type":"internal"}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-key")
	var resp meilisearch.DocumentsResult
	err := c.manager.Index("companies").GetDocumentsWithContext(t.Context(), &meilisearch.DocumentsQuery{Limit: 1}, &resp)
	if err == nil {
		t.Fatal("a 503 from the engine surfaced no error")
	}
	if len(bodies) != 1 {
		t.Errorf("engine saw %d POSTs, want exactly 1 (SDK retry must be disabled): bodies re-sent after a consumed buffer arrive empty and read as 400 missing_payload", len(bodies))
	}
	var me *meilisearch.Error
	if !errors.As(err, &me) {
		t.Fatalf("error is %T, want *meilisearch.Error: %v", err, err)
	}
	if me.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("surfaced status %d, want the engine's 503 (the failure the caller can actually retry)", me.StatusCode)
	}
}

// TestSDKRetryResendsEmptyBody documents the upstream defect itself: with the
// SDK's default retry ENABLED, a POST that meets a retryable 503 is re-sent with
// a zero-byte body. This is the mechanism behind the incident's
// "Code: missing_payload" 400. If a future SDK release fixes body rewinding and
// this package re-enables retries, this test fails — which is the reminder to
// re-read the DisableRetries comment in NewClient.
func TestSDKRetryResendsEmptyBody(t *testing.T) {
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		w.Header().Set("Content-Type", "application/json")
		if len(bodies) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"message":"engine overloaded"}`)
			return
		}
		_, _ = io.WriteString(w, `{"results":[],"total":0}`)
	}))
	defer srv.Close()

	m := meilisearch.New(srv.URL, meilisearch.WithAPIKey("test-key")) // default retry ON
	var resp meilisearch.DocumentsResult
	if err := m.Index("companies").GetDocumentsWithContext(t.Context(), &meilisearch.DocumentsQuery{Limit: 1}, &resp); err != nil {
		t.Fatalf("default-retry client errored: %v", err)
	}
	if len(bodies) != 2 {
		t.Fatalf("stub saw %d requests, want 2 (503 then retry)", len(bodies))
	}
	if bodies[0] == "" {
		t.Fatal("first request carried no body — stub is wrong")
	}
	if bodies[1] != "" {
		t.Fatalf("retried request carried body %q, want EMPTY — the SDK now rewinds the body and DisableRetries in NewClient can be revisited", bodies[1])
	}
}
