package search

import (
	"io"
	"net/http"
	"time"
)

// maxRetries bounds re-sends of a body-less request after a 502/503/504.
const maxRetries = 3

// errorBodyDrainLimit bounds the error body read before a retry so the connection is reused.
const errorBodyDrainLimit = 4 << 10

// engineMaxIdleConnsPerHost matches the SDK's default transport.
const engineMaxIdleConnsPerHost = 100

// idempotentRetryTransport re-sends body-less GET, HEAD and DELETE requests after a
// transient 502/503/504. The SDK's own retry re-sends an already consumed body, so it
// cannot stay on for POSTs (see NewClient). This covers the task polls behind Promote and
// the index drops (dropIndex tolerates the index_not_found a repeated delete causes).
type idempotentRetryTransport struct {
	base    http.RoundTripper
	backoff func(attempt int) time.Duration
}

// newEngineHTTPClient is the client NewClient hands the SDK. It replaces the SDK's
// transport, so the idle pool is restated: Go's default of two idle connections per host
// would redial under concurrent search.
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
		if err != nil || attempt > maxRetries || !transientStatus(resp.StatusCode) {
			return resp, err
		}
		_, _ = io.CopyN(io.Discard, resp.Body, errorBodyDrainLimit)
		_ = resp.Body.Close()
		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(t.backoff(attempt)):
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
