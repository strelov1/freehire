package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/strelov1/freehire/internal/job/liveness"
)

func TestDetailsURLTakesThePostingIDAfterTheBoard(t *testing.T) {
	got, ok := detailsURL("it-jobs:5922558051")
	if !ok || got != "https://www.adzuna.com/details/5922558051" {
		t.Fatalf("detailsURL() = %q, %v", got, ok)
	}
}

// A malformed id must not become a URL: probing "details/abc" or "details/" reads some other
// page, and a 404 from it would close a posting nobody asked about.
func TestDetailsURLRefusesAnIDThatIsNotDigits(t *testing.T) {
	for _, id := range []string{"it-jobs:", "it-jobs:59x2", "", "it-jobs:59/../1"} {
		if got, ok := detailsURL(id); ok {
			t.Errorf("detailsURL(%q) = %q, want refused", id, got)
		}
	}
}

func TestWellfoundPageURLIsTheStoredPostingPage(t *testing.T) {
	const url = "https://wellfound.com/jobs/4465590-senior-or-staff-full-stack-engineer"
	got, ok := wellfoundPageURL("software-engineer:4465590", url)
	if !ok || got != url {
		t.Fatalf("wellfoundPageURL() = %q, %v", got, ok)
	}
}

// Wellfound answers 404 for a LIVE posting asked for without its title slug
// (/jobs/4689896, measured 2026-10-30), so a URL that does not carry the slug, or names a
// different posting than the row, must not be probed: its 404 would close a live posting.
func TestWellfoundPageURLRefusesAURLThatCannotAnswerForTheRow(t *testing.T) {
	cases := []struct{ externalID, url string }{
		{"software-engineer:4465590", "https://wellfound.com/jobs/4465590"},
		{"software-engineer:4465590", "https://wellfound.com/jobs/4465591-other-posting"},
		{"software-engineer:4465590", "https://wellfound.com/jobs/44655900-longer-id"},
		{"software-engineer:4465590", "https://example.com/jobs/4465590-senior"},
		{"software-engineer:4465590", "http://wellfound.com/jobs/4465590-senior"},
		{"software-engineer:", "https://wellfound.com/jobs/-senior"},
		{"software-engineer:44x5", "https://wellfound.com/jobs/44x5-senior"},
	}
	for _, c := range cases {
		if got, ok := wellfoundPageURL(c.externalID, c.url); ok {
			t.Errorf("wellfoundPageURL(%q, %q) = %q, want refused", c.externalID, c.url, got)
		}
	}
}

func TestClassifyReadsTheThreeShapesAdzunaAnswers(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     string
		want     liveness.Verdict
		answered bool
	}{
		{"removed: page not found", 404, "<title>Page not found - adzuna.com.au</title>", liveness.Expired, true},
		{"removed: gone", 410, "<title>Oracle PL/SQL Developer</title> no longer available", liveness.Expired, true},
		{"live posting", 200, `<script type="application/ld+json">{"@type":"JobPosting"}</script>`, liveness.Live, true},
		{"challenge", 405, "<title>Human Verification</title>", liveness.Uncertain, false},
		// A 200 that is not a posting page is not evidence the posting is live.
		{"200 without a posting", 200, "<title>Human Verification</title>", liveness.Uncertain, false},
		{"bot wall", 403, "ERROR: The request could not be satisfied", liveness.Uncertain, false},
		{"server error", 503, "", liveness.Uncertain, false},
	}
	for _, c := range cases {
		got, answered := classify(c.status, []byte(c.body))
		if got != c.want || answered != c.answered {
			t.Errorf("%s: classify() = %v, %v; want %v, %v", c.name, got, answered, c.want, c.answered)
		}
	}
}

// The challenge answers most requests, so a probe that gave up on the first one would answer
// almost nothing. It must keep asking until Adzuna answers, and stop the moment it does.
func TestProbeRetriesPastTheChallengeAndStopsAtTheAnswer(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) < 4 {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusGone)
	}))
	defer srv.Close()

	verdict, tries, err := probe(context.Background(), srv.Client(), srv.URL, 25)
	if err != nil || verdict != liveness.Expired || tries != 4 {
		t.Fatalf("probe() = %v, %d, %v; want Expired after 4 tries", verdict, tries, err)
	}
	if calls.Load() != 4 {
		t.Fatalf("server saw %d requests, want 4 — the probe must stop at the first answer", calls.Load())
	}
}

func TestProbeLeavesAPostingUncertainWhenNoAttemptGetsThrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	defer srv.Close()

	verdict, tries, _ := probe(context.Background(), srv.Client(), srv.URL, 5)
	if verdict != liveness.Uncertain || tries != 5 {
		t.Fatalf("probe() = %v after %d tries, want Uncertain after 5", verdict, tries)
	}
}
