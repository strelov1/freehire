package qualityaudit

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// site is a fake job host: the paths it knows answer 200, and what an unknown id gets is up to
// unknown. It records every path asked for.
type site struct {
	mu      sync.Mutex
	hits    []string
	live    map[string]bool
	gone    map[string]bool
	unknown int
	robots  string
	refuse  map[string]bool
}

func (s *site) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.hits = append(s.hits, r.URL.Path)
	s.mu.Unlock()
	switch {
	case r.URL.Path == "/robots.txt":
		if s.robots == "" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(s.robots))
	case s.refuse[r.URL.Path]:
		w.WriteHeader(http.StatusTooManyRequests)
	case s.live[r.URL.Path]:
		_, _ = w.Write([]byte("posting"))
	case s.gone[r.URL.Path]:
		w.WriteHeader(http.StatusGone)
	default:
		w.WriteHeader(s.unknown)
	}
}

func (s *site) asked(path string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, h := range s.hits {
		if h == path {
			return true
		}
	}
	return false
}

func (s *site) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.hits)
}

func newAuditor(max int64) *Auditor {
	return &Auditor{
		Client:        http.DefaultClient,
		MaxRequests:   max,
		HostInterval:  time.Millisecond,
		MaxCrawlDelay: time.Second,
		HostWorkers:   4,
	}
}

func posting(id int64, source, base, extID, path string) Posting {
	return Posting{ID: id, Source: source, ExternalID: extID, URL: base + path}
}

func byID(results []Result) map[int64]Result {
	out := make(map[int64]Result, len(results))
	for _, r := range results {
		out[r.Posting.ID] = r
	}
	return out
}

func TestAuditCalibratedHost(t *testing.T) {
	s := &site{
		live:    map[string]bool{"/jobs/100001": true, "/jobs/100002": true},
		gone:    map[string]bool{"/jobs/100003": true},
		unknown: http.StatusNotFound,
	}
	srv := httptest.NewServer(s)
	defer srv.Close()

	results := newAuditor(100).Run(context.Background(), []Posting{
		posting(1, "greenhouse", srv.URL, "acme:100001", "/jobs/100001"),
		posting(2, "greenhouse", srv.URL, "acme:100002", "/jobs/100002"),
		posting(3, "greenhouse", srv.URL, "acme:100003", "/jobs/100003"),
	})
	got := byID(results)
	if got[1].Outcome != Live || got[2].Outcome != Live || got[3].Outcome != Dead {
		t.Fatalf("results = %+v", results)
	}
	counts := TallyLiveness(results)["greenhouse"]
	if counts != (LivenessCounts{Sampled: 3, Live: 2, Dead: 1}) {
		t.Errorf("tally = %+v", counts)
	}
}

func TestAuditHostAnsweringAnyID(t *testing.T) {
	s := &site{gone: map[string]bool{"/ad/200002": true}, unknown: http.StatusOK}
	srv := httptest.NewServer(s)
	defer srv.Close()

	results := newAuditor(100).Run(context.Background(), []Posting{
		posting(1, "jobtech", srv.URL, "200001", "/ad/200001"),
		posting(2, "jobtech", srv.URL, "200002", "/ad/200002"),
	})
	for _, r := range results {
		if r.Outcome != Unmeasured || r.Reason != ReasonUncalibrated {
			t.Errorf("%d: %v %q, want uncalibrated", r.Posting.ID, r.Outcome, r.Reason)
		}
	}
	// The canary already proved the host's answers mean nothing, so no posting is fetched —
	// not even the one that would have answered 410.
	if s.asked("/ad/200001") || s.asked("/ad/200002") {
		t.Errorf("an uncalibrated host's postings were fetched: %v", s.hits)
	}
	if c := TallyLiveness(results)["jobtech"]; c != (LivenessCounts{Sampled: 2}) {
		t.Errorf("tally = %+v, want sampled only", c)
	}
}

func TestAuditRefusalEndsTheHost(t *testing.T) {
	s := &site{
		live:    map[string]bool{"/j/300002": true, "/j/300003": true},
		refuse:  map[string]bool{"/j/300001": true},
		unknown: http.StatusNotFound,
	}
	srv := httptest.NewServer(s)
	defer srv.Close()

	// One host worker, so the postings go in input order after the shuffle of HOSTS (one).
	a := newAuditor(100)
	results := a.Run(context.Background(), []Posting{
		posting(1, "x", srv.URL, "300001", "/j/300001"),
		posting(2, "x", srv.URL, "300002", "/j/300002"),
		posting(3, "x", srv.URL, "300003", "/j/300003"),
	})
	got := byID(results)
	for id := int64(1); id <= 3; id++ {
		if got[id].Reason != ReasonRefused {
			t.Errorf("%d: %v %q, want refused", id, got[id].Outcome, got[id].Reason)
		}
	}
	if s.asked("/j/300002") {
		t.Error("the host was asked again after it refused")
	}
}

func TestAuditBudget(t *testing.T) {
	s := &site{live: map[string]bool{"/j/400001": true, "/j/400002": true}, unknown: http.StatusNotFound}
	srv := httptest.NewServer(s)
	defer srv.Close()

	// robots.txt + canary + one posting.
	results := newAuditor(3).Run(context.Background(), []Posting{
		posting(1, "x", srv.URL, "400001", "/j/400001"),
		posting(2, "x", srv.URL, "400002", "/j/400002"),
	})
	got := byID(results)
	if got[1].Outcome != Live || got[2].Reason != ReasonBudget {
		t.Errorf("results = %+v", results)
	}
	if n := s.count(); n != 3 {
		t.Errorf("host was asked %d times, want 3", n)
	}
}

func TestAuditRobots(t *testing.T) {
	s := &site{
		live:    map[string]bool{"/open/500001": true, "/closed/500002": true},
		unknown: http.StatusNotFound,
		robots:  "User-agent: *\nDisallow: /closed\n",
	}
	srv := httptest.NewServer(s)
	defer srv.Close()

	got := byID(newAuditor(100).Run(context.Background(), []Posting{
		posting(1, "x", srv.URL, "500001", "/open/500001"),
		posting(2, "x", srv.URL, "500002", "/closed/500002"),
	}))
	if got[1].Outcome != Live || got[2].Reason != ReasonRobots {
		t.Errorf("results = %+v", got)
	}
	if s.asked("/closed/500002") {
		t.Error("a disallowed path was fetched")
	}
}

func TestAuditCrawlDelayTooLongSkipsTheHost(t *testing.T) {
	s := &site{live: map[string]bool{"/j/600001": true}, unknown: http.StatusNotFound, robots: "User-agent: *\nCrawl-delay: 120\n"}
	srv := httptest.NewServer(s)
	defer srv.Close()

	got := byID(newAuditor(100).Run(context.Background(), []Posting{posting(1, "x", srv.URL, "600001", "/j/600001")}))
	if got[1].Reason != ReasonRobots {
		t.Errorf("result = %+v, want robots", got[1])
	}
}

func TestAuditStatusBlindAndBadURL(t *testing.T) {
	s := &site{live: map[string]bool{"/vacancy/700001": true}, unknown: http.StatusNotFound}
	srv := httptest.NewServer(s)
	defer srv.Close()

	got := byID(newAuditor(100).Run(context.Background(), []Posting{
		posting(1, "hh", srv.URL, "700001", "/vacancy/700001"),
		{ID: 2, Source: "x", ExternalID: "700002", URL: "not a url"},
	}))
	if got[1].Reason != ReasonStatusBlind || got[2].Reason != ReasonBadURL {
		t.Errorf("results = %+v", got)
	}
	if s.count() != 0 {
		t.Errorf("a status-blind source was fetched: %v", s.hits)
	}
}

func TestAuditNoCanaryPossible(t *testing.T) {
	s := &site{live: map[string]bool{"/apply": true}, unknown: http.StatusNotFound}
	srv := httptest.NewServer(s)
	defer srv.Close()

	got := byID(newAuditor(100).Run(context.Background(), []Posting{posting(1, "x", srv.URL, "800001", "/apply")}))
	if got[1].Reason != ReasonUncalibrated {
		t.Errorf("result = %+v, want uncalibrated", got[1])
	}
}

func TestAuditTwoHostsCalibrateApart(t *testing.T) {
	good := &site{gone: map[string]bool{"/j/900001": true}, unknown: http.StatusNotFound}
	liar := &site{unknown: http.StatusOK}
	gs, ls := httptest.NewServer(good), httptest.NewServer(liar)
	defer gs.Close()
	defer ls.Close()

	results := newAuditor(100).Run(context.Background(), []Posting{
		posting(1, "greenhouse", gs.URL, "900001", "/j/900001"),
		posting(2, "greenhouse", ls.URL, "900002", "/j/900002"),
	})
	got := byID(results)
	if got[1].Outcome != Dead || got[2].Reason != ReasonUncalibrated {
		t.Errorf("results = %+v", got)
	}
	if !strings.HasPrefix(got[2].Posting.URL, ls.URL) {
		t.Fatal("results lost their postings")
	}
}

func TestAuditDoesNotFollowAPostingOffItsHost(t *testing.T) {
	elsewhere := &site{live: map[string]bool{"/landing": true}, unknown: http.StatusNotFound}
	es := httptest.NewServer(elsewhere)
	defer es.Close()
	mux := http.NewServeMux()
	mux.HandleFunc("/j/110001", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, es.URL+"/landing", http.StatusFound)
	})
	mux.HandleFunc("/j/110002", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/j/110002/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/j/110002/", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("/", http.NotFound)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	got := byID(newAuditor(100).Run(context.Background(), []Posting{
		posting(1, "x", srv.URL, "110001", "/j/110001"),
		posting(2, "x", srv.URL, "110002", "/j/110002"),
	}))
	if got[1].Reason != ReasonMoved {
		t.Errorf("off-host redirect = %+v, want moved", got[1])
	}
	if got[2].Outcome != Live {
		t.Errorf("same-host redirect = %+v, want followed and live", got[2])
	}
	if elsewhere.count() != 0 {
		t.Errorf("the other host was probed: %v", elsewhere.hits)
	}
}

func TestAuditRobotsRefusalEndsTheHost(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) })
	var asked atomic.Int64
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { asked.Add(1); http.NotFound(w, r) })
	srv := httptest.NewServer(mux)
	defer srv.Close()

	got := byID(newAuditor(100).Run(context.Background(), []Posting{posting(1, "x", srv.URL, "120001", "/j/120001")}))
	if got[1].Reason != ReasonRefused || asked.Load() != 0 {
		t.Errorf("result = %+v after %d requests, want refused and nothing asked", got[1], asked.Load())
	}
}

func TestAuditRedirectHopsArePacedAndBudgeted(t *testing.T) {
	var hits atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("/j/130001", func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Redirect(w, r, "/j/130001/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/j/130001/", func(w http.ResponseWriter, _ *http.Request) { hits.Add(1); _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { hits.Add(1); http.NotFound(w, r) })
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// robots.txt + canary + the posting's first hop: the budget has nothing left for the second.
	got := byID(newAuditor(3).Run(context.Background(), []Posting{posting(1, "x", srv.URL, "130001", "/j/130001")}))
	if got[1].Reason != ReasonBudget {
		t.Errorf("result = %+v, want budget", got[1])
	}
	if n := hits.Load(); n != 3 {
		t.Errorf("host was asked %d times, want 3", n)
	}
}

func TestAuditFollowsFiveRedirects(t *testing.T) {
	mux := http.NewServeMux()
	for i := range 5 {
		mux.HandleFunc(fmt.Sprintf("/j/140001/%d", i), func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, fmt.Sprintf("/j/140001/%d", i+1), http.StatusFound)
		})
	}
	var reached atomic.Int64
	mux.HandleFunc("/j/140001/5", func(w http.ResponseWriter, _ *http.Request) { reached.Add(1); _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("/", http.NotFound)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	newAuditor(100).Run(context.Background(), []Posting{posting(1, "x", srv.URL, "140001", "/j/140001/0")})
	if reached.Load() != 1 {
		t.Error("the fifth redirect was not followed")
	}
}
