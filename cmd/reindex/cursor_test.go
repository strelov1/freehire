package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/platform/db"
)

// cursorFake is a marker_pass_cursors table of one pass.
type cursorFake struct {
	point  *string // nil: no row yet
	writes []string
}

func (f *cursorFake) GetMarkerPassCursor(context.Context, string) (string, error) {
	if f.point == nil {
		return "", pgx.ErrNoRows
	}
	return *f.point, nil
}

func (f *cursorFake) SetMarkerPassCursor(_ context.Context, arg db.SetMarkerPassCursorParams) error {
	f.writes = append(f.writes, arg.AfterCompany)
	f.point = &arg.AfterCompany
	return nil
}

// The batched passes used to start from the top every run. Once a walk outgrew the unit's 4h
// (2026-11-21) every run redid the same head and was killed, and the passes after it never ran.
func TestBatchedPassResumesAfterTheSavedCompanyAndClearsAtTheEnd(t *testing.T) {
	companies := chunkable([]string{"c", "a", "b"}) // unsorted, as the query returns them
	slices.Reverse(companies)
	saved := lastOfBatches(chunkable([]string{"a"}))[0]
	store := &cursorFake{point: &saved}

	var visited []string
	_, err := forCompanyBatchesResuming(context.Background(), &passCursor{store: store, pass: rolePassName}, companies,
		func(_ context.Context, batch []string) (int64, error) {
			visited = append(visited, batch[0][:1])
			return 0, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"b", "c"}; !slices.Equal(visited, want) {
		t.Fatalf("visited batches %v, want %v: everything after the saved point, in sorted order", visited, want)
	}
	if store.point == nil || *store.point != "" {
		t.Fatalf("resume point = %v, want it cleared once the walk reached the end", store.point)
	}
}

// A run its deadline stops keeps its place instead of clearing it.
func TestBatchedPassKeepsItsPlaceWhenTheRunEnds(t *testing.T) {
	companies := chunkable([]string{"a", "b", "c"})
	store := &cursorFake{}
	ctx, cancel := context.WithCancel(context.Background())
	_, err := forCompanyBatchesResuming(ctx, &passCursor{store: store, pass: rolePassName}, companies,
		func(ctx context.Context, batch []string) (int64, error) {
			if batch[0][:1] == "b" {
				cancel()
				return 0, ctx.Err()
			}
			return 0, nil
		})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want the cancellation", err)
	}
	if want := lastOfBatches(companies)[0]; store.point == nil || *store.point != want {
		t.Fatalf("resume point = %v, want %q, the last company of the one finished batch", store.point, want)
	}
}

// A pass the run's own budget stopped saved its place, so it is not a failure; a SIGTERM or a
// statement timeout still is.
func TestBudgetSpentIsOnlyTheRunsOwnDeadline(t *testing.T) {
	expired, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	<-expired.Done()
	terminated, stop := context.WithCancel(context.Background())
	stop()

	cases := []struct {
		name string
		ctx  context.Context
		err  error
		want bool
	}{
		{"budget", expired, fmt.Errorf("cancelled after 3 completed batches: %w", context.DeadlineExceeded), true},
		{"sigterm", terminated, fmt.Errorf("cancelled after 3 completed batches: %w", context.Canceled), false},
		{"statement timeout in a live run", context.Background(), errors.New("canceling statement due to statement timeout"), false},
		{"other error after the deadline", expired, errors.New("save resume point: connection refused"), false},
		{"budget joined with nothing else", expired, errors.Join(fmt.Errorf("walk: %w", context.DeadlineExceeded), nil), true},
		{"budget joined with a failed save", expired, errors.Join(fmt.Errorf("walk: %w", context.DeadlineExceeded), errors.New("save role resume point")), false},
	}
	for _, c := range cases {
		if got := budgetSpent(c.ctx, c.err); got != c.want {
			t.Errorf("%s: budgetSpent = %t, want %t", c.name, got, c.want)
		}
	}
}
