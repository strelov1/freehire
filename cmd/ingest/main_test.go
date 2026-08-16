package main

import (
	"reflect"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/pipeline"
	"github.com/strelov1/freehire/internal/sources"
)

// The shared custom.yml must load and pass validation against the real adapter registry,
// so a bad provider name or a missing board there fails the build, not a 2am cron run.
// Validate never fetches, so the taxonomy registry is the right one — it also spares the
// test the crawl credentials a keyed provider's board file would otherwise need.
func TestCustomYAMLValidates(t *testing.T) {
	cfg, err := sources.LoadConfig("../../sources/custom.yml")
	if err != nil {
		t.Fatalf("load custom.yml: %v", err)
	}
	if err := cfg.Validate(sources.Taxonomy()); err != nil {
		t.Fatalf("custom.yml failed validation against the real registry: %v", err)
	}
	if len(cfg.Sources) < 13 {
		t.Errorf("custom.yml has %d entries, want >= 13 single-source providers", len(cfg.Sources))
	}
}

// In a multi-provider run only the providers that ingested at least one job are swept,
// so a provider whose crawl failed (ingested 0) never has its catalogue mass-closed. The
// result is sorted for a deterministic sweep order.
func TestSweepableProviders(t *testing.T) {
	rs := pipeline.RunStats{
		"vk":   {Ingested: 5},
		"ozon": {Ingested: 0, Failed: 3}, // crawl failed → excluded
		"sber": {Ingested: 2},
	}
	got := sweepableProviders(rs)
	want := []string{"sber", "vk"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sweepableProviders = %v, want %v", got, want)
	}
}

// The sweep guard: closing stale jobs is only safe after a run that actually saw
// postings — a zero-ingest run (total crawl outage) must not trigger the sweep.
func TestShouldSweep(t *testing.T) {
	cases := []struct {
		name  string
		stats pipeline.Stats
		want  bool
	}{
		{"normal run", pipeline.Stats{Ingested: 100, Failed: 3}, true},
		{"zero ingested", pipeline.Stats{Ingested: 0, Failed: 550}, false},
		{"all boards ok but empty", pipeline.Stats{Ingested: 0, Failed: 0}, false},
	}
	for _, tc := range cases {
		if got := shouldSweep(tc.stats); got != tc.want {
			t.Errorf("%s: shouldSweep = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// A slice-crawled source declares a window wider than the default so a posting that merely
// drifted past the crawl's page depth is not closed and then reopened; every other provider
// keeps the default untouched.
func TestSweepWindowFor(t *testing.T) {
	grace := map[string]time.Duration{"whatjobs": 14 * 24 * time.Hour}

	cases := []struct {
		name     string
		provider string
		want     time.Duration
	}{
		{"declaring provider gets its wider window", "whatjobs", 14 * 24 * time.Hour},
		{"ordinary provider keeps the default", "greenhouse", staleAfter},
	}
	for _, tc := range cases {
		if got := sweepWindowFor(grace, tc.provider); got != tc.want {
			t.Errorf("%s: sweepWindowFor(%q) = %v, want %v", tc.name, tc.provider, got, tc.want)
		}
	}
}

// A fullCatalog source may close by source only after a clean (zero-Failed) run: a truncated
// crawl looks like a shrunken catalogue and a source-scoped close would mass-close the postings
// it never reached, so a partial run must fall back to the company-scoped close.
func TestSweepBySource(t *testing.T) {
	cases := []struct {
		name        string
		stats       pipeline.Stats
		fullCatalog bool
		want        bool
	}{
		{"full-catalog, clean run", pipeline.Stats{Ingested: 1000, Failed: 0}, true, true},
		{"full-catalog, a board failed", pipeline.Stats{Ingested: 1000, Failed: 1}, true, false},
		{"not full-catalog, clean run", pipeline.Stats{Ingested: 1000, Failed: 0}, false, false},
	}
	for _, tc := range cases {
		if got := sweepBySource(tc.stats, tc.fullCatalog); got != tc.want {
			t.Errorf("%s: sweepBySource = %v, want %v", tc.name, got, tc.want)
		}
	}
}

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
