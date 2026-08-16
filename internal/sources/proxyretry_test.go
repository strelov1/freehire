package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

// rewriteTransport sends every request to base whatever URL was asked for, which is how these
// tests stand in for "this transport egresses somewhere else" without running a real proxy.
type rewriteTransport struct{ base *url.URL }

func (t rewriteTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.URL.Scheme, r.URL.Host = t.base.Scheme, t.base.Host
	return http.DefaultTransport.RoundTrip(r)
}

// twoWayClient builds a Client whose direct transport goes to direct and whose refusal
// fallback goes to fallback, with no backoff so the tests stay fast.
func twoWayClient(t *testing.T, direct, fallback *httptest.Server) *Client {
	t.Helper()
	du, err := url.Parse(direct.URL)
	if err != nil {
		t.Fatalf("parse direct: %v", err)
	}
	c := &Client{
		httpClient: &http.Client{Transport: rewriteTransport{du}, Timeout: 5 * time.Second},
		userAgent:  "test",
		maxRetries: 2,
		retryDelay: 0,
	}
	if fallback != nil {
		fu, err := url.Parse(fallback.URL)
		if err != nil {
			t.Fatalf("parse fallback: %v", err)
		}
		c.refusalClient = &http.Client{Transport: rewriteTransport{fu}, Timeout: 5 * time.Second}
	}
	return c
}

func countingServer(t *testing.T, status int, body string, hits *atomic.Int64) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(s.Close)
	return s
}

// A 429 from the direct IP is the case this exists for: the platform is there and is declining
// this IP's volume, so the retry goes out through the proxy instead of hammering the same IP.
func TestRefusalRetryUsesFallbackOn429(t *testing.T) {
	var directHits, fallbackHits atomic.Int64
	direct := countingServer(t, http.StatusTooManyRequests, "", &directHits)
	fallback := countingServer(t, http.StatusOK, `{"ok":true}`, &fallbackHits)

	var out struct {
		OK bool `json:"ok"`
	}
	if err := twoWayClient(t, direct, fallback).GetJSON(context.Background(), "https://board.example/x", &out); err != nil {
		t.Fatalf("GetJSON: %v", err)
	}
	if !out.OK {
		t.Error("decoded body did not come from the fallback")
	}
	if got := fallbackHits.Load(); got != 1 {
		t.Errorf("fallback hits = %d, want 1", got)
	}
	if got := directHits.Load(); got != 1 {
		t.Errorf("direct hits = %d, want 1 — the direct IP must not be retried once it is refusing", got)
	}
}

// A 403 is normally final and returns at once. With a fallback it is worth exactly one more
// try from a different IP, because that is precisely the shape of an IP-reputation refusal.
func TestRefusalRetryUsesFallbackOn403(t *testing.T) {
	var directHits, fallbackHits atomic.Int64
	direct := countingServer(t, http.StatusForbidden, "", &directHits)
	fallback := countingServer(t, http.StatusOK, `{"ok":true}`, &fallbackHits)

	var out struct {
		OK bool `json:"ok"`
	}
	if err := twoWayClient(t, direct, fallback).GetJSON(context.Background(), "https://board.example/x", &out); err != nil {
		t.Fatalf("GetJSON: %v", err)
	}
	if got := fallbackHits.Load(); got != 1 {
		t.Errorf("fallback hits = %d, want 1", got)
	}
}

// Without a fallback a 403 must behave exactly as before — returned immediately, not retried.
// This is what keeps the change inert for every provider that is not opted in.
func TestNoFallbackLeaves403Final(t *testing.T) {
	var directHits atomic.Int64
	direct := countingServer(t, http.StatusForbidden, "", &directHits)

	var out struct{}
	err := twoWayClient(t, direct, nil).GetJSON(context.Background(), "https://board.example/x", &out)
	if err == nil {
		t.Fatal("GetJSON succeeded, want a 403 error")
	}
	if got := directHits.Load(); got != 1 {
		t.Errorf("direct hits = %d, want 1 — a 403 is final without a fallback", got)
	}
}

// The proxy is the expensive path (one shared IP), so it must stay untouched while the direct
// IP is being served.
func TestFallbackUnusedWhenDirectSucceeds(t *testing.T) {
	var directHits, fallbackHits atomic.Int64
	direct := countingServer(t, http.StatusOK, `{"ok":true}`, &directHits)
	fallback := countingServer(t, http.StatusOK, `{"ok":false}`, &fallbackHits)

	var out struct {
		OK bool `json:"ok"`
	}
	if err := twoWayClient(t, direct, fallback).GetJSON(context.Background(), "https://board.example/x", &out); err != nil {
		t.Fatalf("GetJSON: %v", err)
	}
	if got := fallbackHits.Load(); got != 0 {
		t.Errorf("fallback hits = %d, want 0", got)
	}
}

// A fallback that is refused too must not loop: the whole point of the budget is that the
// proxy IP is a shared, limited resource.
func TestFallbackTriedOnlyOnce(t *testing.T) {
	var directHits, fallbackHits atomic.Int64
	direct := countingServer(t, http.StatusTooManyRequests, "", &directHits)
	fallback := countingServer(t, http.StatusTooManyRequests, "", &fallbackHits)

	var out struct{}
	if err := twoWayClient(t, direct, fallback).GetJSON(context.Background(), "https://board.example/x", &out); err == nil {
		t.Fatal("GetJSON succeeded, want a rate-limit error")
	}
	if got := fallbackHits.Load(); got != 1 {
		t.Errorf("fallback hits = %d, want 1 — the shared proxy IP gets one try, not every retry", got)
	}
}
