//go:build integration

// Integration test for the sweep's row-by-row fallback (see main.go, sweepRowByRow):
// the 2026-08-11 incident where one heap/index-corrupted jobs_pkey value aborted the
// whole bulk CloseUnseenJobs UPDATE, silently leaving every other closeable greenhouse
// job open. A trigger stands in for that corruption — it blocks the UPDATE for one
// specific row, the same shape of failure the bulk statement hits from a bad row.
// Run with: go test -tags=integration ./cmd/ingest/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/job"
	"github.com/strelov1/freehire/internal/jobderive"
	"github.com/strelov1/freehire/internal/pipeline"
	"github.com/strelov1/freehire/internal/testdb"
)

func sweepTestPosting(source, externalID, title string) job.Job {
	j, err := job.New(job.Draft{
		Input: jobderive.Input{
			Source:      source,
			ExternalID:  externalID,
			Title:       title,
			Company:     "Acme",
			Location:    "Remote",
			Description: "<p>A test posting for the sweep row-by-row fallback.</p>",
		},
		URL: "https://example.com/" + externalID,
	})
	if err != nil {
		panic(err)
	}
	return j
}

func ageJobForSweepTest(t *testing.T, pool *pgxpool.Pool, id int64, seenAgo time.Duration) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		"UPDATE jobs SET last_seen_at = now() - $2::interval WHERE id = $1",
		id, seenAgo.String()); err != nil {
		t.Fatalf("backdate last_seen_at for %d: %v", id, err)
	}
}

// blockCloseFor makes any UPDATE that would set jobs.closed_at on this one id fail,
// standing in for a row that a bulk UPDATE can't write (corruption, a stray constraint,
// whatever) without needing to reproduce actual heap corruption in a test. The id is
// baked into the function body directly (not a bind parameter — a trigger function
// takes no arguments of its own, so $1 there would mean something else entirely).
func blockCloseFor(t *testing.T, pool *pgxpool.Pool, id int64) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, fmt.Sprintf(`
		CREATE OR REPLACE FUNCTION test_block_close() RETURNS trigger AS $$
		BEGIN
			IF NEW.id = %d THEN
				RAISE EXCEPTION 'simulated unclosable row (id=%%)', NEW.id;
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql`, id))
	if err != nil {
		t.Fatalf("create block-close function: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		CREATE TRIGGER test_block_close_trigger BEFORE UPDATE ON jobs
		FOR EACH ROW EXECUTE FUNCTION test_block_close()`); err != nil {
		t.Fatalf("create block-close trigger: %v", err)
	}
}

func TestSweepRowByRow_SkipsOneUnclosableRowWithoutBlockingTheRest(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)
	store := newDBStore(pool, 1, nil, nil, pipeline.HydrationRetryWindow)

	if err := store.Save(ctx, sweepTestPosting("greenhouse", "acme:blocked", "Blocked Role")); err != nil {
		t.Fatalf("save blocked job: %v", err)
	}
	if err := store.Save(ctx, sweepTestPosting("greenhouse", "acme:closeable", "Closeable Role")); err != nil {
		t.Fatalf("save closeable job: %v", err)
	}

	blocked, err := q.GetJobBySourceExternalID(ctx, db.GetJobBySourceExternalIDParams{Source: "greenhouse", ExternalID: "acme:blocked"})
	if err != nil {
		t.Fatalf("load blocked job: %v", err)
	}
	closeable, err := q.GetJobBySourceExternalID(ctx, db.GetJobBySourceExternalIDParams{Source: "greenhouse", ExternalID: "acme:closeable"})
	if err != nil {
		t.Fatalf("load closeable job: %v", err)
	}

	ageJobForSweepTest(t, pool, blocked.ID, 72*time.Hour)
	ageJobForSweepTest(t, pool, closeable.ID, 72*time.Hour)
	blockCloseFor(t, pool, blocked.ID)

	cutoff := pgtype.Timestamptz{Time: time.Now().Add(-48 * time.Hour), Valid: true}
	closed, skipped, err := sweepRowByRow(ctx, q, "greenhouse", cutoff, []string{blocked.CompanySlug}, false)
	if err != nil {
		t.Fatalf("sweepRowByRow returned an error (it must skip bad rows, not fail): %v", err)
	}
	if closed != 1 {
		t.Errorf("closed = %d, want 1 (the closeable job)", closed)
	}
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1 (the blocked job)", skipped)
	}

	closeableAfter, err := q.GetJob(ctx, closeable.ID)
	if err != nil {
		t.Fatalf("get closeable job: %v", err)
	}
	if !closeableAfter.ClosedAt.Valid {
		t.Error("the closeable job must be closed despite the other row being unclosable")
	}
	blockedAfter, err := q.GetJob(ctx, blocked.ID)
	if err != nil {
		t.Fatalf("get blocked job: %v", err)
	}
	if blockedAfter.ClosedAt.Valid {
		t.Error("the blocked job must stay open, not silently marked closed")
	}
}
