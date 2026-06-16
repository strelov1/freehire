//go:build integration

// Integration test for the incremental reindex feed (ListJobsUpdatedAfter): it
// must return only rows changed at or after the cutoff, and must include
// freshly-closed rows (so the reindex --since pass can delete them from the
// index). Verifiable only against real Postgres.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestListJobsUpdatedAfter(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	old, err := q.UpsertJob(ctx, ingestParams("acme:old", "Old"))
	if err != nil {
		t.Fatalf("upsert old: %v", err)
	}
	recent, err := q.UpsertJob(ctx, ingestParams("acme:recent", "Recent"))
	if err != nil {
		t.Fatalf("upsert recent: %v", err)
	}

	// Pin updated_at to controlled instants so the cutoff boundary is deterministic
	// (UpsertJob stamps now(), which would put both rows on the same side).
	before := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	after := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	cutoff := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `UPDATE jobs SET updated_at = $1 WHERE id = $2`, before, old.ID); err != nil {
		t.Fatalf("pin old updated_at: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET updated_at = $1 WHERE id = $2`, after, recent.ID); err != nil {
		t.Fatalf("pin recent updated_at: %v", err)
	}

	params := func(since time.Time) ListJobsUpdatedAfterParams {
		return ListJobsUpdatedAfterParams{
			AfterID:   0,
			Since:     pgtype.Timestamptz{Time: since, Valid: true},
			BatchSize: 100,
		}
	}

	t.Run("returns only rows at or after the cutoff", func(t *testing.T) {
		got, err := q.ListJobsUpdatedAfter(ctx, params(cutoff))
		if err != nil {
			t.Fatalf("ListJobsUpdatedAfter: %v", err)
		}
		if len(got) != 1 || got[0].ID != recent.ID {
			t.Fatalf("got %v, want only the recent job %d", jobIDs(got), recent.ID)
		}
	})

	t.Run("includes a freshly-closed row so it can leave the index", func(t *testing.T) {
		// Closing stamps updated_at = now(); simulate that landing after the cutoff.
		if _, err := pool.Exec(ctx, `UPDATE jobs SET closed_at = $1, updated_at = $1 WHERE id = $2`, after, old.ID); err != nil {
			t.Fatalf("close old: %v", err)
		}
		got, err := q.ListJobsUpdatedAfter(ctx, params(cutoff))
		if err != nil {
			t.Fatalf("ListJobsUpdatedAfter: %v", err)
		}
		var sawClosedOld bool
		for _, j := range got {
			if j.ID == old.ID {
				sawClosedOld = j.ClosedAt.Valid
			}
		}
		if !sawClosedOld {
			t.Fatalf("recently-closed job %d missing from incremental feed (ids %v)", old.ID, jobIDs(got))
		}
	})
}

func jobIDs(jobs []Job) []int64 {
	out := make([]int64, len(jobs))
	for i, j := range jobs {
		out[i] = j.ID
	}
	return out
}
