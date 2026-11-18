package main

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
)

// A covered row whose refresh the state fence refuses maxFencedAttempts times is an error, never
// an ok=false no-op: the board must not prove coverage with a listed row unrefreshed.
func TestRefreshCoveredOne_FenceExhaustionIsAnError(t *testing.T) {
	calls := 0
	orig := refreshCoveredJobFn
	refreshCoveredJobFn = func(context.Context, *db.Queries, db.RefreshCoveredJobParams) (db.RefreshCoveredJobRow, error) {
		calls++
		return db.RefreshCoveredJobRow{Refreshed: false}, nil
	}
	t.Cleanup(func() { refreshCoveredJobFn = orig })

	tally := newWriteTally()
	s := &dbStore{tally: tally}
	_, ok, err := s.refreshCoveredOne(context.Background(), "lever", "acme:1")
	if err == nil || ok {
		t.Fatalf("refreshCoveredOne = ok=%v err=%v, want an error", ok, err)
	}
	if calls != maxFencedAttempts {
		t.Errorf("statement ran %d times, want %d", calls, maxFencedAttempts)
	}
	if got, want := tally.summary(), "lever fenced=3"; got != want {
		t.Errorf("tally = %q, want %q", got, want)
	}
}
