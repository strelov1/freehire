//go:build integration

package autoapplyorchestrate

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
)

// recordedCall is one request the fake hire server received.
type recordedCall struct {
	path string
	auth string
	body map[string]any
}

// fakeHire stands in for freehire's own two auto-apply routes
// (POST /me/auto-apply/:queueId/{tailor,review}) in the Inngest function's own integration
// tests: it records every call it receives and lets a test control the tailor call's
// response status per queue id, without a real database, session, or LLM call anywhere in
// the path.
type fakeHire struct {
	mu              sync.Mutex
	calls           []recordedCall
	tailorStatusFor map[string]int // queueId -> status; default 200
}

func newFakeHire() *fakeHire {
	return &fakeHire{tailorStatusFor: map[string]int{}}
}

func (f *fakeHire) server() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(f.handle))
}

func (f *fakeHire) handle(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body) // a tailor call has no body; a review call does
	}

	f.mu.Lock()
	f.calls = append(f.calls, recordedCall{path: r.URL.Path, auth: r.Header.Get("Authorization"), body: body})
	f.mu.Unlock()

	switch {
	case strings.HasSuffix(r.URL.Path, "/tailor"):
		queueID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/me/auto-apply/"), "/tailor")
		f.mu.Lock()
		status, ok := f.tailorStatusFor[queueID]
		f.mu.Unlock()
		if !ok {
			status = http.StatusOK
		}
		w.WriteHeader(status)
	case strings.HasSuffix(r.URL.Path, "/review"):
		w.WriteHeader(http.StatusOK)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// Calls returns a snapshot of every call received so far.
func (f *fakeHire) Calls() []recordedCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]recordedCall, len(f.calls))
	copy(out, f.calls)
	return out
}

// setTailorStatus makes the tailor call for queueID answer with status instead of 200.
func (f *fakeHire) setTailorStatus(queueID string, status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tailorStatusFor[queueID] = status
}
