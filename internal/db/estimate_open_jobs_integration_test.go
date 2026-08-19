//go:build integration

// Integration test for estimate_open_jobs(), the planner-backed approximate total the
// DB-backed /jobs list reports as meta.total. The function must estimate the same set
// the list paginates — open, not duplicate-suppressed, not private — rather than the
// wider "closed_at IS NULL" set, which counts suppressed reposts the list never shows.
// This is a plpgsql/planner behavior, verifiable only against a real Postgres.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// externalID mints a seeded job's external_id so a whole cohort is addressable by a
// LIKE prefix — the seeding below flips columns per cohort, not per row.
func externalID(cohort string, i int) string {
	return cohort + ":" + strconv.Itoa(i)
}

// countJobs runs an exact count under the given predicate, naming the two candidate
// sets the estimate could be describing.
func countJobs(t *testing.T, pool *pgxpool.Pool, where string) int64 {
	t.Helper()
	var n int64
	if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM jobs WHERE `+where).Scan(&n); err != nil {
		t.Fatalf("count where %s: %v", where, err)
	}
	return n
}

func TestEstimateOpenJobsExcludesSuppressedAndPrivate(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	// Enough rows that the planner has something to estimate from, and enough
	// excluded rows that counting them would blow past the tolerance: 20 listed
	// against 30 that must not be counted.
	const (
		listed    = 20
		closed    = 10
		duplicate = 10
		private   = 10
	)

	var canonicalID int64
	for i := range listed {
		job, err := ingestUpsert(ctx, q, ingestParams(externalID("open", i), "Go Engineer"))
		if err != nil {
			t.Fatalf("seed open job: %v", err)
		}
		if i == 0 {
			canonicalID = job.ID // the row the suppressed copies point at
		}
	}

	for i := range closed {
		if _, err := ingestUpsert(ctx, q, ingestParams(externalID("closed", i), "Go Engineer")); err != nil {
			t.Fatalf("seed closed job: %v", err)
		}
	}
	if _, err := pool.Exec(ctx,
		`UPDATE jobs SET closed_at = now() WHERE external_id LIKE 'closed:%'`); err != nil {
		t.Fatalf("close jobs: %v", err)
	}

	for i := range duplicate {
		if _, err := ingestUpsert(ctx, q, ingestParams(externalID("dup", i), "Go Engineer")); err != nil {
			t.Fatalf("seed duplicate job: %v", err)
		}
	}
	if _, err := pool.Exec(ctx,
		`UPDATE jobs SET duplicate_of_role = $1 WHERE external_id LIKE 'dup:%'`, canonicalID); err != nil {
		t.Fatalf("suppress duplicates: %v", err)
	}

	for i := range private {
		if _, err := ingestUpsert(ctx, q, ingestParams(externalID("priv", i), "Go Engineer")); err != nil {
			t.Fatalf("seed private job: %v", err)
		}
	}
	if _, err := pool.Exec(ctx,
		`UPDATE jobs SET is_private = true WHERE external_id LIKE 'priv:%'`); err != nil {
		t.Fatalf("mark private: %v", err)
	}

	// The estimate reads planner statistics, so it is only meaningful once they
	// reflect the rows just written.
	if _, err := pool.Exec(ctx, `ANALYZE jobs`); err != nil {
		t.Fatalf("analyze: %v", err)
	}

	// The two sets the function could be describing: the one the list paginates, and
	// the wider one it used to estimate.
	paginated := countJobs(t, pool, "closed_at IS NULL AND duplicate_of IS NULL AND NOT is_private")
	notClosed := countJobs(t, pool, "closed_at IS NULL")
	if paginated != listed || notClosed != listed+duplicate+private {
		t.Fatalf("seeding is wrong: paginated = %d (want %d), not-closed = %d (want %d)",
			paginated, listed, notClosed, listed+duplicate+private)
	}

	got, err := q.EstimateOpenJobs(ctx)
	if err != nil {
		t.Fatalf("EstimateOpenJobs: %v", err)
	}

	// Assert which set is being estimated, not how accurate the planner is. The
	// function returns an estimate by design, and on a table this small the planner's
	// selectivity arithmetic carries real error — pinning the value to the exact count
	// would test Postgres's statistics rather than this function's predicate.
	distToPaginated := abs64(got - paginated)
	distToNotClosed := abs64(got - notClosed)
	if distToPaginated >= distToNotClosed {
		t.Errorf("EstimateOpenJobs = %d sits nearer the %d not-closed rows than the %d rows the "+
			"list paginates — it is still estimating the wider set (%d duplicate-suppressed and "+
			"%d private rows must not be counted)",
			got, notClosed, paginated, duplicate, private)
	}
}

func abs64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}
