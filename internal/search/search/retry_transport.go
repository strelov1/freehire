package search

import (
	"net/http"
	"time"
)

// retryAttempts bounds how often a body-less request is re-sent after a 502/503/504
// (the SDK's own default was three retries).
const retryAttempts = 3

// engineMaxIdleConnsPerHost matches the SDK's default transport; the engine is one host.
const engineMaxIdleConnsPerHost = 100

// idempotentRetryTransport re-sends body-less GET, HEAD and DELETE requests that met a
// transient 502/503/504. It exists because the Meilisearch SDK's retry loop re-sends a
// request whose body has already been consumed, so it cannot be left on for POSTs (see
// NewClient). A request with no body is safe to repeat, and two such requests carry a
// rebuild that nothing else retries: the task-status polls behind Promote, and the index
// drops around it (dropIndex tolerates the index_not_found a repeated delete can cause).
type idempotentRetryTransport struct {
	base    http.RoundTripper
	backoff func(attempt int) time.Duration
}

// newEngineHTTPClient is the HTTP client NewClient hands the SDK. Passing a client
// replaces the SDK's own transport, so its idle pool is restated here: Go's default of
// two idle connections per host would close and redial under concurrent search traffic.
func newEngineHTTPClient() *http.Client {
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.MaxIdleConnsPerHost = engineMaxIdleConnsPerHost
	return &http.Client{Transport: &idempotentRetryTransport{
		base:    base,
		backoff: func(attempt int) time.Duration { return time.Duration(attempt) * time.Second },
	}}
}

func (t *idempotentRetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !repeatable(req) {
		return t.base.RoundTrip(req)
	}
	for attempt := 1; ; attempt++ {
		resp, err := t.base.RoundTrip(req)
		if err != nil || attempt > retryAttempts || !transientStatus(resp.StatusCode) {
			return resp, err
		}
		_ = resp.Body.Close()
		timer := time.NewTimer(t.backoff(attempt))
		select {
		case <-req.Context().Done():
			timer.Stop()
			return nil, req.Context().Err()
		case <-timer.C:
		}
	}
}

func repeatable(req *http.Request) bool {
	if req.Body != nil && req.Body != http.NoBody {
		return false
	}
	return req.Method == http.MethodGet || req.Method == http.MethodHead || req.Method == http.MethodDelete
}

func transientStatus(code int) bool {
	return code == http.StatusBadGateway || code == http.StatusServiceUnavailable || code == http.StatusGatewayTimeout
}
