package pipeline

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// A run reports which boards it PROVED it covered, so the post-run sweep can retire those
// boards' stale postings without waiting for their company to appear in the crawled-slug set.
// Every test here is about a board that must NOT earn that right: the one case where the
// mechanism is dangerous is closing within a board the run did not really read.

func TestRunReportsABoardItCovered(t *testing.T) {
	src := fakeSource{provider: "greenhouse", jobs: []sources.Job{
		{ExternalID: "1", Title: "Backend Engineer", Company: "Acme"},
	}}
	r := Runner{Registry: registry(src), Store: &fakeStore{}}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "greenhouse", Board: "acme"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := stats["greenhouse"].SweepableBoards; !slices.Equal(got, []string{"acme"}) {
		t.Errorf("SweepableBoards = %v, want [acme]", got)
	}
}

func TestRunReportsNoBoardWhenTheCrawlYieldedNothing(t *testing.T) {
	// A board that lists zero postings is indistinguishable from a board whose crawl broke —
	// which is not hypothetical: a Workday board reporting total:0 on its second page once had
	// its live tail closed by the sweep (freehire#725). Refusing costs a real but small share
	// of the benefit and removes the whole class of failure.
	src := fakeSource{provider: "greenhouse", jobs: nil}
	r := Runner{Registry: registry(src), Store: &fakeStore{}}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "greenhouse", Board: "acme"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := stats["greenhouse"].SweepableBoards; len(got) != 0 {
		t.Errorf("SweepableBoards = %v, want none — a zero-yield crawl proves nothing", got)
	}
}

func TestRunReportsNoBoardWhenTheCrawlFailed(t *testing.T) {
	src := fakeSource{provider: "greenhouse", err: errors.New("502 bad gateway")}
	r := Runner{Registry: registry(src), Store: &fakeStore{}}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "greenhouse", Board: "acme"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := stats["greenhouse"].SweepableBoards; len(got) != 0 {
		t.Errorf("SweepableBoards = %v, want none — a failed crawl proves nothing", got)
	}
}

func TestRunReportsNoBoardForABoardlessEntry(t *testing.T) {
	// The one failure here is catastrophic rather than merely wrong: a boardless entry
	// namespaces its postings with an empty board, so a board-scoped match would select the
	// provider's ENTIRE catalogue and the sweep would close all of it.
	src := fakeSource{provider: "greenhouse", jobs: []sources.Job{
		{ExternalID: "1", Title: "Backend Engineer", Company: "Acme"},
	}}
	r := Runner{Registry: registry(src), Store: &fakeStore{}}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "greenhouse", Board: ""},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := stats["greenhouse"].SweepableBoards; len(got) != 0 {
		t.Errorf("SweepableBoards = %v, want none — a boardless entry has no board scope", got)
	}
}

func TestRunReportsABoardWhoseEveryPostingWasRejected(t *testing.T) {
	// The crawl reached these postings; the catalogue filter turned them away afterwards. If
	// a rejected posting did not count, the sweep would spare exactly the non-technical boards
	// where stale rows accumulate — and it is the same disjunction the pipeline already uses to
	// decide a mid-crawl error was still progress.
	src := fakeSource{provider: "greenhouse", jobs: []sources.Job{
		{ExternalID: "1", Title: "Registered Nurse", Company: "Acme"},
	}}
	store := &fakeStore{}
	r := Runner{Registry: registry(src), Store: store}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "greenhouse", Board: "acme"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats["greenhouse"].Rejected == 0 {
		t.Fatalf("fixture no longer rejects — stats=%+v; pick a title the catalogue filter refuses",
			stats["greenhouse"])
	}
	if got := stats["greenhouse"].SweepableBoards; !slices.Equal(got, []string{"acme"}) {
		t.Errorf("SweepableBoards = %v, want [acme] — a rejected posting is still a posting the crawl reached", got)
	}
}

func TestRunReportsEachCoveredBoardOfAProviderOnce(t *testing.T) {
	src := fakeSource{provider: "greenhouse", jobs: []sources.Job{
		{ExternalID: "1", Title: "Backend Engineer", Company: "Acme"},
	}}
	r := Runner{Registry: registry(src), Store: &fakeStore{}}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "greenhouse", Board: "acme"},
		{Company: "Globex", Provider: "greenhouse", Board: "globex"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := slices.Clone(stats["greenhouse"].SweepableBoards)
	slices.Sort(got)
	if !slices.Equal(got, []string{"acme", "globex"}) {
		t.Errorf("SweepableBoards = %v, want [acme globex]", got)
	}
}
