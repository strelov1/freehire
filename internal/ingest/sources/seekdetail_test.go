package sources

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// seekRateLimitedBody is the reply SEEK sent on 2026-10-15 to every jobDetails request once it had
// banned the prod IP, verbatim: HTTP 200, the refusal only in the GraphQL errors.
const seekRateLimitedBody = `{"errors":[{"message":"Too many requests.","path":["jobDetails","job"],"extensions":{"code":"RATE_LIMITED"}}],"data":{"jobDetails":null}}`

// seekHydratedBody is a served jobDetails reply.
const seekHydratedBody = `{"data":{"jobDetails":{"job":{"content":"<p>A real body</p>"}}}}`

// seekLaneFake is one egress lane: it answers every jobDetails with the same raw body, decoded the
// way the real client decodes it, and counts what it was sent.
type seekLaneFake struct {
	body string
	err  error

	mu   sync.Mutex
	hits []string
}

func (f *seekLaneFake) PostJSON(_ context.Context, _ string, body, v any) error {
	f.mu.Lock()
	f.hits = append(f.hits, seekTestJobID(body))
	f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	return json.Unmarshal([]byte(f.body), v)
}

func (f *seekLaneFake) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.hits)
}

func TestSeekDetailResponseRecognisesRateLimited(t *testing.T) {
	var limited, served seekDetailResponse
	if err := json.Unmarshal([]byte(seekRateLimitedBody), &limited); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(seekHydratedBody), &served); err != nil {
		t.Fatal(err)
	}
	if !limited.rateLimited() {
		t.Error("the 2026-10-15 refusal body was not recognised as RATE_LIMITED")
	}
	if served.rateLimited() {
		t.Error("a served body was read as RATE_LIMITED")
	}
	other := seekDetailResponse{}
	other.Errors = append(other.Errors, limited.Errors[0])
	other.Errors[0].Extensions.Code = "GRAPHQL_VALIDATION_FAILED"
	if other.rateLimited() {
		t.Error("a non-refusal GraphQL error was read as RATE_LIMITED")
	}
	if !seekRefused(&StatusError{Code: http.StatusTooManyRequests}, seekDetailResponse{}) {
		t.Error("an HTTP 429 was not read as a refusal")
	}
	if seekRefused(&StatusError{Code: http.StatusInternalServerError}, seekDetailResponse{}) {
		t.Error("an HTTP 500 was read as a refusal")
	}
}

// A refusal on the direct lane moves the run to the proxy, and the refused posting itself is
// retried there rather than lost.
func TestSeekDetailNetworkSwitchesToTheProxyOnTheFirstRefusal(t *testing.T) {
	direct := &seekLaneFake{body: seekRateLimitedBody}
	proxy := &seekLaneFake{body: seekHydratedBody}
	n := newSeekDetailNetwork(direct, proxy)

	for _, id := range []string{"1", "2", "3"} {
		body, outcome := n.content(context.Background(), "jobstreet", "https://sg.jobstreet.com", id)
		if outcome != detailHydrated || !strings.Contains(body, "A real body") {
			t.Fatalf("posting %s: outcome %v body %q, want hydrated through the proxy", id, outcome, body)
		}
	}
	if direct.count() != 1 {
		t.Errorf("direct lane sent %d requests, want 1 — the first refusal must move the run off it", direct.count())
	}
	if proxy.count() != 3 {
		t.Errorf("proxy lane sent %d requests, want 3", proxy.count())
	}
}

func TestSeekDetailNetworkStopsWhenTheProxyIsRefusedToo(t *testing.T) {
	direct := &seekLaneFake{body: seekRateLimitedBody}
	proxy := &seekLaneFake{body: seekRateLimitedBody}
	n := newSeekDetailNetwork(direct, proxy)

	for _, id := range []string{"1", "2", "3", "4"} {
		if _, outcome := n.content(context.Background(), "seek", "https://www.seek.com.au", id); outcome != detailStopped {
			t.Fatalf("posting %s: outcome %v, want stopped", id, outcome)
		}
	}
	if direct.count() != 1 || proxy.count() != 1 {
		t.Errorf("sent direct %d / proxy %d, want 1 / 1 — nothing may be sent after the stop", direct.count(), proxy.count())
	}
}

func TestSeekDetailNetworkWithoutAProxyStopsOnTheFirstRefusal(t *testing.T) {
	direct := &seekLaneFake{body: seekRateLimitedBody}
	n := newSeekDetailNetwork(direct, nil)

	for _, id := range []string{"1", "2", "3"} {
		if _, outcome := n.content(context.Background(), "seek", "https://www.seek.com.au", id); outcome != detailStopped {
			t.Fatalf("posting %s: outcome %v, want stopped", id, outcome)
		}
	}
	if direct.count() != 1 {
		t.Errorf("direct lane sent %d requests, want 1", direct.count())
	}
}

// The budget is charged across both lanes, and spending it stops the run like a refusal does.
func TestSeekDetailNetworkStopsAtTheRunBudget(t *testing.T) {
	direct := &seekLaneFake{body: seekHydratedBody}
	n := newSeekDetailNetwork(direct, nil)
	n.budget = 3

	var hydrated, stopped int
	for _, id := range []string{"1", "2", "3", "4", "5"} {
		switch _, outcome := n.content(context.Background(), "seek", "https://www.seek.com.au", id); outcome {
		case detailHydrated:
			hydrated++
		case detailStopped:
			stopped++
		}
	}
	if hydrated != 3 || stopped != 2 || direct.count() != 3 {
		t.Errorf("hydrated %d, stopped %d, sent %d; want 3, 2, 3", hydrated, stopped, direct.count())
	}
}

// A non-refusal failure is a per-posting failure: it must not move the lane.
func TestSeekDetailNetworkTransportFailureDoesNotSwitchLanes(t *testing.T) {
	direct := &seekLaneFake{err: errSeekTest}
	proxy := &seekLaneFake{body: seekHydratedBody}
	n := newSeekDetailNetwork(direct, proxy)

	if _, outcome := n.content(context.Background(), "seek", "https://www.seek.com.au", "1"); outcome != detailFailed {
		t.Fatalf("outcome %v, want failed", outcome)
	}
	if proxy.count() != 0 {
		t.Errorf("proxy lane sent %d requests after a transport failure, want 0", proxy.count())
	}
}

// A ban jobstreet meets must stop seek too: the two adapters share one network.
func TestSeekNetworkStopIsSharedAcrossProviders(t *testing.T) {
	limited := &seekLaneFake{body: seekRateLimitedBody}
	n := newSeekDetailNetwork(limited, nil)
	posting := func(id string) map[int][]seekPosting {
		return map[int][]seekPosting{1: {seekPost(id, "Role", "Co")}}
	}
	notSeen := func(string) bool { return false }

	jobstreet := NewJobStreet(&seekFake{searchByPage: posting("1")}, n).(HydratingSource)
	if jobs, err := jobstreet.FetchNew(context.Background(), CompanyEntry{Region: "sg", Board: "6290"}, notSeen); err != nil || len(jobs) != 0 {
		t.Fatalf("jobstreet: %d jobs, err %v; want the refused posting deferred", len(jobs), err)
	}
	seekSrc := NewSeek(&seekFake{searchByPage: posting("2")}, n).(HydratingSource)
	if jobs, err := seekSrc.FetchNew(context.Background(), CompanyEntry{Region: "au", Board: "6290"}, notSeen); err != nil || len(jobs) != 0 {
		t.Fatalf("seek: %d jobs, err %v; want the posting deferred", len(jobs), err)
	}
	if limited.count() != 1 {
		t.Errorf("sent %d detail requests, want 1 — seek must not hammer a network jobstreet saw banned", limited.count())
	}
}

// ApplyProxyEgress must give both SEEK-network providers ONE network with a proxy lane, and leave
// them direct-only when no proxy is configured.
func TestApplyProxyEgressGivesTheSeekNetworkOneProxyLane(t *testing.T) {
	t.Setenv("SOURCES_PROXY_URL", "")
	registry := All(NewClient())
	if err := ApplyProxyEgress(registry); err != nil {
		t.Fatalf("ApplyProxyEgress: %v", err)
	}
	if registry["seek"].(seek).details.proxy != nil {
		t.Error("the SEEK network has a proxy lane with no proxy configured")
	}

	t.Setenv("SOURCES_PROXY_URL", "http://user:pass@proxy.example:8080")
	if err := ApplyProxyEgress(registry); err != nil {
		t.Fatalf("ApplyProxyEgress: %v", err)
	}
	s, js := registry["seek"].(seek), registry["jobstreet"].(jobStreet)
	if s.details != js.details {
		t.Error("seek and jobstreet were given separate detail networks; a ban must be shared")
	}
	if s.details.proxy == nil {
		t.Error("the SEEK network has no proxy lane with a proxy configured")
	}
}
