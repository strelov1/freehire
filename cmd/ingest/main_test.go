package main

import (
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/ingest/pipeline"
	sweeppolicy "github.com/strelov1/freehire/internal/ingest/sweep"
)

// HYDRATION_RETRY_DAYS widens the window during which a body-less row is re-offered for
// detail hydration, so an operator can repair a backlog the ordinary two-week window has
// already aged past (freehire#1866). Unset means the default; a value that is not a positive
// number is a config error, not a silent fallback — a typo on a repair run would otherwise
// look like a run that repaired nothing.
func TestHydrationRetryWindowFor(t *testing.T) {
	cases := []struct {
		name    string
		env     string
		want    time.Duration
		wantErr bool
	}{
		{"unset keeps the default", "", pipeline.HydrationRetryWindow, false},
		{"widened for a repair run", "365", 365 * 24 * time.Hour, false},
		{"narrowed", "1", 24 * time.Hour, false},
		{"not a number", "two weeks", 0, true},
		{"zero", "0", 0, true},
		{"negative", "-5", 0, true},
	}
	for _, tc := range cases {
		got, err := hydrationRetryWindowFor(tc.env)
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: hydrationRetryWindowFor(%q) err = %v, wantErr %v", tc.name, tc.env, err, tc.wantErr)
			continue
		}
		if err == nil && got != tc.want {
			t.Errorf("%s: hydrationRetryWindowFor(%q) = %v, want %v", tc.name, tc.env, got, tc.want)
		}
	}
}

// INGEST_REFETCH_ALL empties the seen-set so a crawl re-writes the provider's stored rows —
// the only way an adapter fix reaches postings ingested before it, since a re-listed posting
// otherwise takes the content-less refresh path. Anything but the two accepted spellings is a
// config error rather than a silent false, for the same reason HYDRATION_RETRY_DAYS is.
func TestRefetchAllFor(t *testing.T) {
	cases := []struct {
		name    string
		env     string
		want    bool
		wantErr bool
	}{
		{"unset is an ordinary crawl", "", false, false},
		{"one", "1", true, false},
		{"true", "true", true, false},
		{"zero is not a spelling of off", "0", false, true},
		{"typo", "yes", false, true},
	}
	for _, tc := range cases {
		got, err := refetchAllFor(tc.env)
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: refetchAllFor(%q) err = %v, wantErr %v", tc.name, tc.env, err, tc.wantErr)
			continue
		}
		if err == nil && got != tc.want {
			t.Errorf("%s: refetchAllFor(%q) = %v, want %v", tc.name, tc.env, got, tc.want)
		}
	}
}

// defaultSeenPolicy is the seen-set policy a deployment with no knobs set gets: retry a
// body-less row inside the default window, re-fetch nothing else. It is what the store
// integration tests construct, so a test asserting a WRITE never accidentally asserts a
// re-fetch policy as well.
func defaultSeenPolicy() seenPolicy {
	return seenPolicy{
		hydrationWindow: pipeline.HydrationRetryWindow,
		bodies:          sweeppolicy.BodyRefresh{Slices: 1, Slot: sweeppolicy.BodyRefreshDisabledSlot},
	}
}

// A red ingest unit must mean the run is broken. A provider of thousands of boards always has
// a few failing ones, and those belong to board_health, not to the exit code.
func TestCrawlReachedNothing(t *testing.T) {
	cases := []struct {
		name  string
		total pipeline.Stats
		want  bool
	}{
		{"a few dead boards beside a working crawl", pipeline.Stats{Ingested: 26362, Failed: 5}, false},
		{"every posting covered, so only refreshed", pipeline.Stats{Refreshed: 40, Failed: 1}, false},
		{"every board failed", pipeline.Stats{Failed: 12}, true},
		{"every save failed", pipeline.Stats{Skipped: 4149}, true},
		{"nothing to do: every board quiet", pipeline.Stats{Quiet: 300}, false},
	}
	for _, tc := range cases {
		if got := crawlReachedNothing(tc.total); got != tc.want {
			t.Errorf("%s: crawlReachedNothing(%+v) = %t, want %t", tc.name, tc.total, got, tc.want)
		}
	}
}

func TestRecentWindowFor(t *testing.T) {
	for _, tc := range []struct {
		env     string
		want    time.Duration
		wantErr bool
	}{
		{"", 0, false},
		{"1", 24 * time.Hour, false},
		{"3", 72 * time.Hour, false},
		{"30", 30 * 24 * time.Hour, false},
		{"0", 0, true},
		{"-2", 0, true},
		{"31", 0, true},
		{"abc", 0, true},
		{"1.5", 0, true},
		{" 3", 0, true},
	} {
		got, err := recentWindowFor(tc.env)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("recentWindowFor(%q) = %v, %v; want %v, err=%t", tc.env, got, err, tc.want, tc.wantErr)
		}
	}
}

// A recent run must not reach the database at all: nil queries would panic if any close ran.
func TestSweepDoesNothingInRecentMode(t *testing.T) {
	stats := pipeline.RunStats{"adzuna": {Ingested: 5, QualifyingBoards: []pipeline.BoardRef{{Board: "it-jobs", Region: "gb"}}}}
	if failed := sweep(t.Context(), nil, nil, stats, newCrawledSet(), nil, false, true); failed != 0 {
		t.Fatalf("failed = %d, want 0", failed)
	}
}
