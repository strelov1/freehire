package pipeline

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// deadlineSource answers its "fast" board at once and holds every other board until the run's
// context ends, then returns what it had listed WITHOUT an error — the adapter that swallows a
// cancellation and hands back a partial listing, which is the hardest shape for the sweep: no
// Failed, a healthy yield, and the writes after the deadline refused or never attempted.
type deadlineSource struct{ provider string }

func (s deadlineSource) Fetch(ctx context.Context, e sources.CompanyEntry) ([]sources.Job, error) {
	jobs := []sources.Job{{ExternalID: "1", Title: "Backend Engineer", Company: e.Company}}
	if e.Board != "fast" {
		<-ctx.Done()
	}
	return jobs, nil
}

// TestRunReportsOnlyTheBoardsItFinishedBeforeItsDeadline is the crawl deadline's safety
// contract: cmd/ingest stops the crawl short of its unit's timeout so the sweep still runs, and
// that sweep may close only within boards the run read to completion. A board finished before
// the deadline qualifies exactly as it would in an unbounded run; a board still in flight when
// the deadline hit does not, even though its adapter reported no failure and it yielded
// postings — it stopped at an arbitrary point, and its unread tail would be read as removed.
func TestRunReportsOnlyTheBoardsItFinishedBeforeItsDeadline(t *testing.T) {
	r := Runner{Registry: registry(deadlineSource{provider: "workday"}), Store: &fakeStore{}}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	stats, err := r.Run(ctx, []sources.CompanyEntry{
		{Company: "Fast Co", Provider: "workday", Board: "fast"},
		{Company: "Slow Co", Provider: "workday", Board: "slow"},
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run error = %v, want the deadline", err)
	}
	got := stats["workday"]
	if !slices.Contains(boardNames(got.QualifyingBoards), "fast") {
		t.Errorf("QualifyingBoards = %v, want the board finished before the deadline", got.QualifyingBoards)
	}
	if slices.Contains(boardNames(got.QualifyingBoards), "slow") {
		t.Errorf("QualifyingBoards = %v, must not contain a board the deadline cut short", got.QualifyingBoards)
	}
	if got.Interrupted != 1 {
		t.Errorf("Interrupted = %d, want 1", got.Interrupted)
	}
	if got.Failed != 0 {
		t.Errorf("Failed = %d, want 0 — running out of time is not the board's failure", got.Failed)
	}
}

// A fetch that ERRORS because the run's context ended is interrupted too, not failed: counting
// it failed is what reported 160 DigitalRecruiters sites as broken on its first crawl
// (2026-10-28).
func TestRunCountsAFetchTheDeadlineEndedAsInterrupted(t *testing.T) {
	src := deadlineErrSource{provider: "workday", started: make(chan struct{})}
	r := Runner{Registry: registry(src), Store: &fakeStore{}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-src.started
		cancel()
	}()

	stats, _ := r.Run(ctx, []sources.CompanyEntry{{Company: "Slow Co", Provider: "workday", Board: "slow"}})
	got := stats["workday"]
	if got.Interrupted != 1 || got.Failed != 0 {
		t.Errorf("Interrupted = %d, Failed = %d; want 1 and 0", got.Interrupted, got.Failed)
	}
}

// A rate limiter refuses AHEAD of the deadline — "rate: Wait(n=1) would exceed context
// deadline" arrives while ctx.Err() is still nil — so the context check alone read it as the
// board's failure. That cooled 160 of phenom's 383 boards on 2026-10-30: every board still
// queued when its shared limiter ran out of run failed in the last seconds of every run, and
// the third such run in a row put it in cooldown.
func TestRunCountsALimiterDeadlineRefusalAsInterrupted(t *testing.T) {
	src := limiterRefusedSource{provider: "phenom"}
	health := &fakeHealth{}
	r := Runner{Registry: registry(src), Store: &fakeStore{}, BoardHealth: health}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	stats, err := r.Run(ctx, []sources.CompanyEntry{{Company: "DHL", Provider: "phenom", Board: "careers.dhl.com"}})
	// The run did not crawl what it was asked to, so it must say so even though its context
	// never ended: cmd/ingest reads a non-nil error as a truncated run, which keeps the
	// company and source scopes from closing the unreached board's postings.
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Run error = %v, want one wrapping context.DeadlineExceeded", err)
	}
	got := stats["phenom"]
	if got.Interrupted != 1 || got.Failed != 0 {
		t.Errorf("Interrupted = %d, Failed = %d; want 1 and 0", got.Interrupted, got.Failed)
	}
	if len(health.failures) != 0 {
		t.Errorf("recorded failures %v; running out of run is not the board's failure", health.failures)
	}
}

// The streaming path persists as it goes, so a deadline mid-stream leaves a board with real
// progress and, here, no error at all. It must not qualify either.
func TestRunReportsNoBoardForAStreamTheDeadlineCutShort(t *testing.T) {
	r := Runner{Registry: registry(deadlineStreamingSource{provider: "eightfold"}), Store: &fakeStore{}}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	stats, _ := r.Run(ctx, []sources.CompanyEntry{{Company: "Acme", Provider: "eightfold", Board: "acme"}})
	got := stats["eightfold"]
	if got.Ingested == 0 {
		t.Fatalf("stats = %+v, want the posting emitted before the deadline saved (fixture assumption)", got)
	}
	if len(got.QualifyingBoards) != 0 {
		t.Errorf("QualifyingBoards = %v, want none for a stream the deadline cut short", got.QualifyingBoards)
	}
	if got.Interrupted != 1 {
		t.Errorf("Interrupted = %d, want 1", got.Interrupted)
	}
}
