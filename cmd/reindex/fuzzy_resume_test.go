package main

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/platform/db"
)

// resumeFake is a store whose companies have nothing to judge, so a test sees only which
// companies the pass visited and what it recorded as its resume point.
type resumeFake struct {
	companies []string
	cursor    *string // nil: no row yet
	cancelAt  string
	cancel    context.CancelFunc

	mu      sync.Mutex
	visited []string
	writes  []string
}

func (f *resumeFake) CompaniesWithFuzzyDedupCandidates(context.Context) ([]string, error) {
	return f.companies, nil
}

func (f *resumeFake) FuzzyDedupCandidateTitlesForCompany(ctx context.Context, company string) ([]db.FuzzyDedupCandidateTitlesForCompanyRow, error) {
	if company == f.cancelAt {
		f.cancel()
		return nil, ctx.Err()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.visited = append(f.visited, company)
	return nil, nil
}

func (f *resumeFake) GetJobDescriptionsByIDs(context.Context, []int64) ([]db.GetJobDescriptionsByIDsRow, error) {
	return nil, nil
}

func (f *resumeFake) MarkFuzzyDuplicatesForCompany(context.Context, db.MarkFuzzyDuplicatesForCompanyParams) (int64, error) {
	return 0, nil
}

func (f *resumeFake) GetMarkerPassCursor(_ context.Context, pass string) (string, error) {
	if pass != fuzzyPassName {
		return "", errors.New("unexpected pass " + pass)
	}
	if f.cursor == nil {
		return "", pgx.ErrNoRows
	}
	return *f.cursor, nil
}

func (f *resumeFake) SetMarkerPassCursor(_ context.Context, arg db.SetMarkerPassCursorParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes = append(f.writes, arg.AfterCompany)
	c := arg.AfterCompany
	f.cursor = &c
	return nil
}

// A run resumes after the company the previous one finished last, and the walk sorts the
// list itself (it arrives unordered), so "after" means the same thing on every run. A run
// that reaches the end clears the resume point for the next cycle.
func TestCollapseFuzzyResumesAfterTheSavedCompanyAndResetsAtTheEnd(t *testing.T) {
	saved := "b"
	f := &resumeFake{companies: []string{"d", "a", "c", "b"}, cursor: &saved}

	if _, err := collapseFuzzyDuplicates(context.Background(), f, 1); err != nil {
		t.Fatalf("pass: %v", err)
	}
	if !slices.Equal(f.visited, []string{"c", "d"}) {
		t.Errorf("visited = %v, want only the companies after %q, in order", f.visited, saved)
	}
	if f.cursor == nil || *f.cursor != "" {
		t.Errorf("resume point after a complete run: writes %q, want it cleared to start from the top", f.writes)
	}
}

// No saved row is a first run: every company is visited.
func TestCollapseFuzzyWithNoSavedCursorWalksEverything(t *testing.T) {
	f := &resumeFake{companies: []string{"b", "a"}}

	if _, err := collapseFuzzyDuplicates(context.Background(), f, 2); err != nil {
		t.Fatalf("pass: %v", err)
	}
	got := slices.Clone(f.visited)
	slices.Sort(got)
	if !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("visited = %v, want every company", f.visited)
	}
}

// A run killed part-way records how far it got, so the next one starts there instead of
// walking the same head of the list into the same timeout — which is how the tail of the
// list went undeduplicated for good.
func TestCollapseFuzzySavesItsProgressWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := &resumeFake{companies: []string{"a", "b", "c", "d"}, cancelAt: "c", cancel: cancel}

	_, err := collapseFuzzyDuplicates(ctx, f, 1)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want the cancellation", err)
	}
	if f.cursor == nil || *f.cursor != "b" {
		t.Errorf("resume point writes %q, want the last to be %q — the last company finished before the cut", f.writes, "b")
	}
}
