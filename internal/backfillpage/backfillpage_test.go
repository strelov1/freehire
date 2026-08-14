package backfillpage

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/db"
)

// fakeLister serves rows keyed off AfterID, standing in for the real keyset
// query so Rows' paging can be exercised without a database.
func fakeLister(all []db.Job) Lister {
	return func(_ context.Context, arg db.ListJobsBySourceAfterParams) ([]db.Job, error) {
		var page []db.Job
		for _, j := range all {
			if j.ID > arg.AfterID {
				page = append(page, j)
				if int32(len(page)) == arg.BatchSize {
					break
				}
			}
		}
		return page, nil
	}
}

// TestRows_WalksEveryRowAcrossMultiplePages guards the pagination loop itself:
// a source with more rows than fit in one page must still have every row
// visited, advancing the keyset cursor page over page.
func TestRows_WalksEveryRowAcrossMultiplePages(t *testing.T) {
	all := make([]db.Job, 0, BatchSize*2+3)
	for i := int64(1); i <= int64(BatchSize)*2+3; i++ {
		all = append(all, db.Job{ID: i, Source: "x"})
	}

	var visited []int64
	err := Rows(context.Background(), fakeLister(all), "x", func(j db.Job) error {
		visited = append(visited, j.ID)
		return nil
	})
	if err != nil {
		t.Fatalf("Rows: %v", err)
	}
	if len(visited) != len(all) {
		t.Fatalf("visited %d rows, want %d (every row across %d+ pages)", len(visited), len(all), 3)
	}
	for i, id := range visited {
		if id != all[i].ID {
			t.Fatalf("visited[%d] = %d, want %d (rows out of keyset order)", i, id, all[i].ID)
		}
	}
}

// TestRows_AbortsOnVisitError guards that a visit error (a real write failure
// the worker needs to surface, not a per-row failure it counts and skips)
// stops the walk immediately rather than continuing past it.
func TestRows_AbortsOnVisitError(t *testing.T) {
	all := []db.Job{{ID: 1, Source: "x"}, {ID: 2, Source: "x"}, {ID: 3, Source: "x"}}
	boom := errors.New("boom")

	var visited []int64
	err := Rows(context.Background(), fakeLister(all), "x", func(j db.Job) error {
		visited = append(visited, j.ID)
		if j.ID == 2 {
			return boom
		}
		return nil
	})
	if !errors.Is(err, boom) {
		t.Fatalf("Rows err = %v, want %v", err, boom)
	}
	if len(visited) != 2 {
		t.Fatalf("visited %d rows before abort, want 2 (must stop at the failing row)", len(visited))
	}
}

// TestRows_EmptySourceVisitsNothing guards the zero-row case: no page at all
// must not error and must not call visit.
func TestRows_EmptySourceVisitsNothing(t *testing.T) {
	called := false
	err := Rows(context.Background(), fakeLister(nil), "x", func(db.Job) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("Rows: %v", err)
	}
	if called {
		t.Error("visit was called for an empty source")
	}
}
