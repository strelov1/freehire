package search

import (
	"net/http"
	"time"
)

// getRetryAttempts bounds how often a body-less read is re-sent after a 502/503/504
// (the SDK's own default was three retries).
const getRetryAttempts = 3

// getRetryTransport re-sends GET and HEAD requests that met a transient 502/503/504.
// It exists because the Meilisearch SDK's retry loop re-sends a request whose body
// has already been consumed, so it cannot be left on for POSTs (see NewClient); a
// request with no body is safe to repeat, and the task-status polls behind a
// rebuild's Promote are exactly such requests that nothing else retries.
type getRetryTransport struct {
	base    http.RoundTripper
	backoff func(attempt int) time.Duration
}

func (t *getRetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		return t.base.RoundTrip(req)
	}
	for attempt := 1; ; attempt++ {
		resp, err := t.base.RoundTrip(req)
		if err != nil || attempt > getRetryAttempts || !transientStatus(resp.StatusCode) {
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

func transientStatus(code int) bool {
	return code == http.StatusBadGateway || code == http.StatusServiceUnavailable || code == http.StatusGatewayTimeout
}
