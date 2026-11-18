//go:build integration

// Release B of the job_activity split: an ordinary observation (the cheap crawl refresh, the
// covered-row refresh, a strike reset) writes the job_activity tuple and NOTHING of jobs: not a
// new version, not a row lock. That is the whole point of the split, because a new jobs version
// rewrites a wide tuple and its page, and a row lock stamps xmax and logs WAL.
//
// Asserted on the heap tuple itself (xmin, xmax, ctid) before and after, never "xmax = 0": the
// foreign-key check of the insert that created the row already stamps xmax, so the baseline is
// whatever the row had, and only a CHANGE is a write or a lock.
//
// Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// tupleFootprint is the physical identity and lock state of one heap tuple.
type tupleFootprint struct{ Xmin, Xmax, Ctid string }

func jobsFootprint(t *testing.T, pool *pgxpool.Pool, id int64) tupleFootprint {
	t.Helper()
	var f tupleFootprint
	if err := pool.QueryRow(context.Background(),
		`SELECT xmin::text, xmax::text, ctid::text FROM jobs WHERE id = $1`, id).Scan(&f.Xmin, &f.Xmax, &f.Ctid); err != nil {
		t.Fatalf("footprint of job %d: %v", id, err)
	}
	return f
}

func activityXmin(t *testing.T, pool *pgxpool.Pool, id int64) string {
	t.Helper()
	var x string
	if err := pool.QueryRow(context.Background(), `SELECT xmin::text FROM job_activity WHERE job_id = $1`, id).Scan(&x); err != nil {
		t.Fatalf("activity xmin of job %d: %v", id, err)
	}
	return x
}

func requireJobsTupleUntouched(t *testing.T, pool *pgxpool.Pool, id int64, before tupleFootprint, what string) {
	t.Helper()
	if after := jobsFootprint(t, pool, id); after != before {
		t.Errorf("%s changed the jobs tuple (xmin/xmax/ctid) %+v -> %+v: an observation must write job_activity only", what, before, after)
	}
}

func TestCheapRefreshLeavesTheJobsTupleAlone(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	job := seedForRefresh(t, pool, q, []string{"berlin"})
	jobBefore, actBefore := jobsFootprint(t, pool, job.ID), activityXmin(t, pool, job.ID)

	if _, err := q.RefreshUnchangedJob(ctx, unchangedParams()); err != nil {
		t.Fatalf("RefreshUnchangedJob: %v", err)
	}

	requireJobsTupleUntouched(t, pool, job.ID, jobBefore, "RefreshUnchangedJob")
	if activityXmin(t, pool, job.ID) == actBefore {
		t.Error("RefreshUnchangedJob did not write job_activity: the observation went nowhere")
	}
	if got := readBack(t, q); !got.LastSeenAt.Time.After(job.LastSeenAt.Time) {
		t.Errorf("last_seen_at = %v, want advanced from %v", got.LastSeenAt.Time, job.LastSeenAt.Time)
	}
}

func TestResetLivenessStrikesWritesOnlyActivity(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	job, err := ingestUpsert(ctx, q, ingestParams("acme:strikes", "Struck"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE job_activity SET liveness_strikes = 2 WHERE job_id = $1`, job.ID); err != nil {
		t.Fatalf("set strikes: %v", err)
	}
	before := jobsFootprint(t, pool, job.ID)

	if err := q.ResetLivenessStrikes(ctx, job.ID); err != nil {
		t.Fatalf("ResetLivenessStrikes: %v", err)
	}

	requireJobsTupleUntouched(t, pool, job.ID, before, "ResetLivenessStrikes")
	var strikes int
	if err := pool.QueryRow(ctx, `SELECT liveness_strikes FROM job_activity WHERE job_id = $1`, job.ID).Scan(&strikes); err != nil || strikes != 0 {
		t.Errorf("activity liveness_strikes = %d (err %v), want 0", strikes, err)
	}
}

func TestCoveredRefreshLeavesTheJobsTuplesAlone(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	var ids []int64
	var externalIDs []string
	for _, ext := range []string{"acme:c1", "acme:c2", "acme:c3"} {
		job, err := ingestUpsert(ctx, q, ingestParams(ext, "Covered "+ext))
		if err != nil {
			t.Fatalf("upsert %s: %v", ext, err)
		}
		ids = append(ids, job.ID)
		externalIDs = append(externalIDs, ext)
	}
	if _, err := pool.Exec(ctx, `UPDATE job_activity SET last_seen_at = now() - interval '10 days'`); err != nil {
		t.Fatalf("age activity: %v", err)
	}
	before := make(map[int64]tupleFootprint, len(ids))
	for _, id := range ids {
		before[id] = jobsFootprint(t, pool, id)
	}

	if _, err := q.RefreshCoveredJobs(ctx, RefreshCoveredJobsParams{Source: "greenhouse", ExternalIds: externalIDs}); err != nil {
		t.Fatalf("RefreshCoveredJobs: %v", err)
	}

	for _, id := range ids {
		requireJobsTupleUntouched(t, pool, id, before[id], "RefreshCoveredJobs")
	}
	var fresh int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM job_activity WHERE last_seen_at > now() - interval '1 minute'`).Scan(&fresh); err != nil || fresh != len(ids) {
		t.Errorf("refreshed activity rows = %d (err %v), want %d", fresh, err, len(ids))
	}
}

// touchObserved is the full touch as the ingest store runs it in release B: the lifecycle
// statement, then the observation in job_activity (job.Touch). A touch the statement deferred
// wrote nothing and observes nothing.
func touchObserved(ctx context.Context, q *Queries, p TouchJobParams) (TouchJobRow, error) {
	row, err := q.TouchJob(ctx, p)
	if err != nil || row.Deferred {
		return row, err
	}
	n, err := q.ObserveJobActivity(ctx, ObserveJobActivityParams{JobID: row.ID, ResetStrikes: row.ResetStrikes})
	if err != nil {
		return row, err
	}
	if n != 1 {
		return row, fmt.Errorf("observe job %d: %d activity rows, want 1", row.ID, n)
	}
	return row, nil
}
