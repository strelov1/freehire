package search

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/meilisearch/meilisearch-go"
)

// TestNewClientDisablesSDKRetry is the regression test for the 2026-11-05 incident
// (fingerprint e6280e536eebc136): meilisearch-go v0.36.3's built-in retry re-sends an
// ALREADY-CONSUMED request body, so a POST that met a retryable 503 was retried with a
// zero-byte body and Meilisearch answered 400 missing_payload, surfacing as a fault from
// /api/v1/companies/sitemap (POST /indexes/companies/documents/fetch) instead of the 503.
// The stub answers a nonempty POST with 503 and an empty one with that 400; the 503 must
// surface in ONE attempt.
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
// "Code: missing_payload" 400. If a future SDK release fixes body rewinding, this
// test fails — which is the reminder to re-read the DisableRetries comment in NewClient.
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

// TestNewClientRetriesTransientBodylessRequests pins the requests that keep a retry: a
// task-status poll or an index drop that meets one 503 is re-sent and succeeds, while the
// SDK's own retry loop stays off. Each case waits out the production 1s backoff.
func TestNewClientRetriesTransientBodylessRequests(t *testing.T) {
	cases := []struct {
		name, method, success string
		successStatus         int
		call                  func(*Client) error
	}{
		{
			name: "task poll", method: http.MethodGet, successStatus: http.StatusOK,
			success: `{"uid":7,"indexUid":"companies","status":"succeeded","type":"documentAdditionOrUpdate"}`,
			call: func(c *Client) error {
				_, err := c.manager.GetTaskWithContext(t.Context(), 7)
				return err
			},
		},
		{
			name: "index drop", method: http.MethodDelete, successStatus: http.StatusAccepted,
			success: `{"taskUid":9,"indexUid":"jobs_rebuild","status":"enqueued","type":"indexDeletion"}`,
			call: func(c *Client) error {
				_, err := c.manager.DeleteIndexWithContext(t.Context(), "jobs_rebuild")
				return err
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var reqs atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != tc.method {
					t.Errorf("stub got %s, want %s", r.Method, tc.method)
				}
				w.Header().Set("Content-Type", "application/json")
				if reqs.Add(1) == 1 {
					w.WriteHeader(http.StatusServiceUnavailable)
					_, _ = io.WriteString(w, `{"message":"engine overloaded","code":"unavailable","type":"internal"}`)
					return
				}
				w.WriteHeader(tc.successStatus)
				_, _ = io.WriteString(w, tc.success)
			}))
			defer srv.Close()

			if err := tc.call(NewClient(srv.URL, "test-key")); err != nil {
				t.Fatalf("request after one transient 503 failed: %v", err)
			}
			if reqs.Load() != 2 {
				t.Errorf("engine saw %d requests, want exactly 2", reqs.Load())
			}
		})
	}
}

// TestRetryTransportBounds pins that a persistent 503 is retried a bounded number
// of times and then surfaced as the 503 itself.
func TestRetryTransportBounds(t *testing.T) {
	var reqs atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	rt := &idempotentRetryTransport{base: http.DefaultTransport, backoff: func(int) time.Duration { return time.Millisecond }}

	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable || reqs.Load() != maxRetries+1 {
		t.Errorf("GET: status %d after %d requests, want 503 after %d", resp.StatusCode, reqs.Load(), maxRetries+1)
	}
}

// TestRetryTransportStopsWhenContextEnds pins that a cancelled caller is not held through
// the backoff: the wait ends with the context's own error and nothing is re-sent.
func TestRetryTransportStopsWhenContextEnds(t *testing.T) {
	var reqs atomic.Int32
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs.Add(1)
		cancel()
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	rt := &idempotentRetryTransport{base: http.DefaultTransport, backoff: func(int) time.Duration { return time.Hour }}

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, err := rt.RoundTrip(req)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if reqs.Load() != 1 {
		t.Errorf("engine saw %d requests, want 1", reqs.Load())
	}
}

// TestEngineHTTPClientKeepsIdlePool pins the idle pool the SDK's own transport had:
// the Go default of two per host would redial under concurrent search traffic.
func TestEngineHTTPClientKeepsIdlePool(t *testing.T) {
	rt, ok := newEngineHTTPClient().Transport.(*idempotentRetryTransport)
	if !ok {
		t.Fatal("engine client does not use idempotentRetryTransport")
	}
	base, ok := rt.base.(*http.Transport)
	if !ok {
		t.Fatalf("base transport is %T, want *http.Transport", rt.base)
	}
	if base.MaxIdleConnsPerHost != engineMaxIdleConnsPerHost {
		t.Errorf("MaxIdleConnsPerHost = %d, want %d", base.MaxIdleConnsPerHost, engineMaxIdleConnsPerHost)
	}
}

// TestRepeatable pins which requests may be re-sent: only a body-less read or delete.
func TestRepeatable(t *testing.T) {
	cases := []struct {
		method string
		body   io.Reader
		want   bool
	}{
		{http.MethodGet, nil, true},
		{http.MethodHead, nil, true},
		{http.MethodDelete, nil, true},
		{http.MethodDelete, http.NoBody, true},
		{http.MethodDelete, strings.NewReader("x"), false},
		{http.MethodPost, nil, false},
		{http.MethodPost, strings.NewReader("x"), false},
		{http.MethodPatch, nil, false},
	}
	for _, tc := range cases {
		req, err := http.NewRequestWithContext(t.Context(), tc.method, "http://engine.test", tc.body)
		if err != nil {
			t.Fatal(err)
		}
		if got := repeatable(req); got != tc.want {
			t.Errorf("repeatable(%s, body=%v) = %v, want %v", tc.method, tc.body != nil, got, tc.want)
		}
	}
}
