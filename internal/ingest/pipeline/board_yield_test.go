package pipeline

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// The `reached` flag RecordSuccess carries is what board_health.last_yield_at is stamped from
// (migration 0158), and therefore what the empty-feed safety net eventually closes a board's
// jobs on. These tests pin the two readings that decide whether a live board is ever mistaken
// for an empty one.

// A board whose listing came back with nothing reached no posting, and must record no yield.
// This is the whole point of the column: the crawl SUCCEEDED, so last_success_at moves and
// consecutive_failures stays zero, and without a separate signal nothing in board_health can
// tell this apart from a healthy board.
func TestRecordSuccessReportsNotReachedForAnEmptyListing(t *testing.T) {
	src := fakeSource{provider: "whatjobs", jobs: nil}
	health := &fakeHealth{}
	r := Runner{Registry: registry(src), Store: &fakeStore{}, BoardHealth: health}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "WhatJobs HU", Provider: "whatjobs", Board: "java developer"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := stats["whatjobs"]; got.Ingested != 0 {
		t.Fatalf("stats = %+v, want Ingested=0 (fixture assumption)", got)
	}
	if len(health.successes) == 0 {
		t.Fatal("an empty listing must still record SUCCESS — the crawl worked, the feed was empty")
	}
	if health.reached["whatjobs/java developer/"] {
		t.Error("reached = true for an empty listing, want false: nothing was there to reach")
	}
}

// A board whose listing works but whose every detail request fails must still record a yield.
// This is the mass-close path, and it is the reason `reached` is boardListedAnyPosting rather
// than the boardReachedPostings permission gate it started as.
//
// The shape: a link-only adapter (smartrecruiters, icims, breezy, jazzhr and the rest, whose
// detail request is a posting's only content) meets a refusing origin or changed markup. Every
// posting comes back Unreadable. Unreadable is deliberately NOT Failed — the board answered, and
// failing it would cool a working board over a per-posting hole — so the crawl records SUCCESS
// with consecutive_failures at 0 and a fresh last_success_at. If that also recorded "no yield",
// the board would age past the empty-feed window with every one of its live postings still on
// it, and the safety net would close them all as feed_empty. The listing named those postings:
// the feed is not empty, our reading of it is broken, and those are different diagnoses.
func TestRecordSuccessReportsReachedWhenEveryPostingWasUnreadable(t *testing.T) {
	src := fakeSource{provider: "smartrecruiters", jobs: unreadableBoard("Acme", 3, 3)}
	health := &fakeHealth{}
	r := Runner{Registry: registry(src), Store: &fakeStore{}, BoardHealth: health}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "smartrecruiters", Board: "acme"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := stats["smartrecruiters"]
	if got.Unreadable != 3 || got.Ingested != 0 || got.Failed != 0 {
		t.Fatalf("stats = %+v, want Unreadable=3 Ingested=0 Failed=0 (fixture assumption)", got)
	}
	if len(health.successes) == 0 {
		t.Fatal("fixture assumption: an all-unreadable board is still recorded as a SUCCESS, " +
			"which is exactly what makes it dangerous here")
	}
	if !health.reached["smartrecruiters/acme/"] {
		t.Error("reached = false for a board whose listing named 3 postings it could not read, " +
			"want true: this board would otherwise be closed as an empty feed while every " +
			"posting on it is live")
	}
}

// An ingested posting is the unambiguous case, pinned so a refactor of the reached expression
// cannot quietly narrow it to something that excludes the ordinary path.
func TestRecordSuccessReportsReachedWhenAPostingWasIngested(t *testing.T) {
	src := fakeSource{provider: "greenhouse", jobs: []sources.Job{
		{ExternalID: "1", Title: "Backend Engineer", Company: "Acme"},
	}}
	health := &fakeHealth{}
	r := Runner{Registry: registry(src), Store: &fakeStore{}, BoardHealth: health}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "greenhouse", Board: "acme"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := stats["greenhouse"]; got.Ingested != 1 {
		t.Fatalf("stats = %+v, want Ingested=1 (fixture assumption)", got)
	}
	if !health.reached["greenhouse/acme/"] {
		t.Error("reached = false for a board that ingested a posting, want true")
	}
}
