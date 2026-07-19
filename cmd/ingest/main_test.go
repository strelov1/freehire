package main

import (
	"reflect"
	"testing"

	"github.com/strelov1/freehire/internal/pipeline"
	"github.com/strelov1/freehire/internal/sources"
)

// The shared custom.yml must load and pass validation against the real adapter registry,
// so a bad provider name or a missing board there fails the build, not a 2am cron run.
// Validate never fetches, so a nil HTTP client is fine for building the registry.
func TestCustomYAMLValidates(t *testing.T) {
	cfg, err := sources.LoadConfig("../../sources/custom.yml")
	if err != nil {
		t.Fatalf("load custom.yml: %v", err)
	}
	if err := cfg.Validate(sources.All(nil)); err != nil {
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
