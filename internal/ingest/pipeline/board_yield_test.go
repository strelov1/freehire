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

// A board whose every posting was turned away by the catalogue filter still REACHED those
// postings — the feed carried them, we declined them — so it must record a yield.
//
// This is the reading the implementation is most likely to get wrong, because Stats.Rejected is
// filled in near the end of the board's crawl and a `reached` computed one line too early sees
// zero. Getting it wrong is not a cosmetic slip: a non-tech-heavy national feed is all-rejected
// as its ordinary steady state, so it would stamp no yield on any run, age past the empty-feed
// window, and have its live postings closed under a diagnosis that was never true of it.
func TestRecordSuccessReportsReachedWhenEveryPostingWasRejected(t *testing.T) {
	src := fakeSource{provider: "greenhouse", jobs: []sources.Job{
		{ExternalID: "1", Title: "Line Cook", Company: "Acme"},
	}}
	health := &fakeHealth{}
	r := Runner{Registry: registry(src), Store: &fakeStore{}, BoardHealth: health}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "greenhouse", Board: "acme"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := stats["greenhouse"]; got.Rejected != 1 || got.Ingested != 0 {
		t.Fatalf("stats = %+v, want Rejected=1 Ingested=0 (fixture assumption)", got)
	}
	if !health.reached["greenhouse/acme/"] {
		t.Error("reached = false for an all-rejected board, want true: the crawl listed the posting, " +
			"so the feed is not empty — only the catalogue declined what was in it")
	}
}

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
	if got := stats["whatjobs"]; got.Ingested != 0 || got.Rejected != 0 {
		t.Fatalf("stats = %+v, want Ingested=0 Rejected=0 (fixture assumption)", got)
	}
	if len(health.successes) == 0 {
		t.Fatal("an empty listing must still record SUCCESS — the crawl worked, the feed was empty")
	}
	if health.reached["whatjobs/java developer/"] {
		t.Error("reached = true for an empty listing, want false: nothing was there to reach")
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
