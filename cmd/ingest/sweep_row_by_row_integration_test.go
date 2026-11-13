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
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/job/jobderive"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
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
	// The sweep reads job_activity. Backdate the legacy column first: the one-way bridge only ever
	// moves job_activity forward, so a legacy write made after ageing it would undo the ageing.
	for _, stmt := range []string{
		"UPDATE jobs SET last_seen_at = now() - $2::interval WHERE id = $1",
		"UPDATE job_activity SET last_seen_at = now() - $2::interval WHERE job_id = $1",
	} {
		if _, err := pool.Exec(context.Background(), stmt, id, seenAgo.String()); err != nil {
			t.Fatalf("backdate last_seen_at for %d: %v", id, err)
		}
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
	store := newDBStore(pool, 1, nil, nil, defaultSeenPolicy())

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
	closed, skipped, err := sweepRowByRow(ctx, q, "greenhouse", cutoff, []string{blocked.CompanySlug}, nil, false)
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

// refreshBeforeClose runs a crawl's observation of one job the instant before the individual close
// statement reaches the database: the window between the fallback's candidate list and its
// per-row closes, where a refresh used to be lost.
type refreshBeforeClose struct {
	db.DBTX
	t    *testing.T
	pool *pgxpool.Pool
	id   int64
}

func (r refreshBeforeClose) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if strings.Contains(sql, "-- name: CloseUnseenJobByID") {
		// The legacy update reaches activity through the same bridge as a release A writer.
		if _, err := r.pool.Exec(ctx, `UPDATE jobs SET last_seen_at = now() WHERE id = $1`, r.id); err != nil {
			r.t.Fatalf("refresh selected row: %v", err)
		}
	}
	return r.DBTX.QueryRow(ctx, sql, args...)
}

// The fallback lists its candidates, then closes them one by one. A crawl that lists a candidate
// again in between has proved it live, so its close must write nothing and queue no removal.
func TestSweepRowByRow_DoesNotCloseARowRefreshedAfterSelection(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)
	store := newDBStore(pool, 1, nil, nil, defaultSeenPolicy())

	if err := store.Save(ctx, sweepTestPosting("greenhouse", "acme:refreshed", "Refreshed Role")); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(ctx, sweepTestPosting("greenhouse", "acme:gone", "Gone Role")); err != nil {
		t.Fatal(err)
	}
	refreshed, err := q.GetJobBySourceExternalID(ctx, db.GetJobBySourceExternalIDParams{Source: "greenhouse", ExternalID: "acme:refreshed"})
	if err != nil {
		t.Fatal(err)
	}
	gone, err := q.GetJobBySourceExternalID(ctx, db.GetJobBySourceExternalIDParams{Source: "greenhouse", ExternalID: "acme:gone"})
	if err != nil {
		t.Fatal(err)
	}
	ageJobForSweepTest(t, pool, refreshed.ID, 72*time.Hour)
	ageJobForSweepTest(t, pool, gone.ID, 72*time.Hour)

	cutoff := pgtype.Timestamptz{Time: time.Now().Add(-48 * time.Hour), Valid: true}
	closed, skipped, err := sweepRowByRow(ctx, db.New(refreshBeforeClose{DBTX: pool, t: t, pool: pool, id: refreshed.ID}),
		"greenhouse", cutoff, []string{refreshed.CompanySlug}, nil, false)
	if err != nil || skipped != 0 {
		t.Fatalf("sweepRowByRow = %d closed, %d skipped, %v", closed, skipped, err)
	}
	if closed != 1 {
		t.Errorf("closed = %d, want 1 (only the row nobody listed again)", closed)
	}
	after, err := q.GetJob(ctx, refreshed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.ClosedAt.Valid {
		t.Error("the row a crawl refreshed after selection was closed anyway")
	}
	var queued int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM search_delete_outbox WHERE job_id = $1`, refreshed.ID).Scan(&queued); err != nil || queued != 0 {
		t.Errorf("removals queued for the refreshed row = %d, %v; want 0", queued, err)
	}
}
