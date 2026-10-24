package main

import (
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

func TestOrderOldestRunFirst(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	entries := []sources.CompanyEntry{
		{Board: "fresh.example.com"},
		{Board: "stale.example.com"},
		{Board: "new-a.example.com"},
		{Board: "Stalest.Example.com"}, // board_health folds case
		{Board: "new-b.example.com"},
		{Board: "it-jobs", Region: "de"},
		{Board: "it-jobs", Region: "fr"},
	}
	lastRun := map[lastRunKey]time.Time{
		newLastRunKey("fresh.example.com", ""):   now,
		newLastRunKey("stale.example.com", ""):   now.Add(-48 * time.Hour),
		newLastRunKey("stalest.example.com", ""): now.Add(-96 * time.Hour),
		newLastRunKey("it-jobs", "de"):           now.Add(-time.Hour),
		newLastRunKey("it-jobs", "fr"):           now.Add(-72 * time.Hour),
	}

	orderOldestRunFirst(entries, lastRun)

	want := []string{
		"new-a.example.com/", // never crawled, catalog order kept
		"new-b.example.com/",
		"Stalest.Example.com/",
		"it-jobs/fr",
		"stale.example.com/",
		"it-jobs/de",
		"fresh.example.com/",
	}
	for i, e := range entries {
		if got := e.Board + "/" + e.Region; got != want[i] {
			t.Errorf("position %d = %s, want %s", i, got, want[i])
		}
	}
}
