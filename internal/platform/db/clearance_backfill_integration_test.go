//go:build integration

// Integration tests for the cmd/backfill-clearance queries. The pass is designed to be
// stopped and re-run at will, and that property rests entirely on the UPDATE's IS
// DISTINCT FROM guard — a SQL behaviour, so only a real Postgres can confirm it.
// Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func trueFlag() pgtype.Bool { return pgtype.Bool{Bool: true, Valid: true} }

// The guard is what makes a re-run free: the second write of the same value reports
// zero rows affected, so no dead tuples and nothing to vacuum. Without it a repeated
// pass would rewrite every row it touches.
func TestSetJobRequiresClearanceIsIdempotent(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	job, err := ingestUpsert(ctx, q, ingestParams("acme:1", "Cleared Systems Engineer"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	n, err := q.SetJobRequiresClearance(ctx, SetJobRequiresClearanceParams{
		ID: job.ID, RequiresClearance: trueFlag(),
	})
	if err != nil {
		t.Fatalf("first write: %v", err)
	}
	if n != 1 {
		t.Fatalf("first write affected %d rows, want 1", n)
	}

	n, err = q.SetJobRequiresClearance(ctx, SetJobRequiresClearanceParams{
		ID: job.ID, RequiresClearance: trueFlag(),
	})
	if err != nil {
		t.Fatalf("second write: %v", err)
	}
	if n != 0 {
		t.Fatalf("second write affected %d rows, want 0 — the guard did not hold", n)
	}

	stored, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !stored.RequiresClearance.Valid || !stored.RequiresClearance.Bool {
		t.Fatalf("requires_clearance = %+v, want true", stored.RequiresClearance)
	}
}

// A fresh posting whose description says nothing carries NULL, not false. The
// distinction is the whole reason the column is nullable: false would claim the
// posting promises no clearance is needed.
func TestRequiresClearanceDefaultsToNull(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	job, err := ingestUpsert(ctx, q, ingestParams("acme:2", "Backend Engineer"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	stored, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if stored.RequiresClearance.Valid {
		t.Fatalf("requires_clearance = %v, want NULL", stored.RequiresClearance.Bool)
	}
}

// The backfill names its candidates through Meilisearch and reads only those rows.
// This is the query that does it: given a set of ids it returns exactly those bodies
// and touches nothing else.
func TestJobDescriptionsByIDsReadsOnlyTheNamedRows(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	wanted, err := ingestUpsert(ctx, q, ingestParams("acme:3", "Cleared Engineer"))
	if err != nil {
		t.Fatalf("upsert wanted: %v", err)
	}
	if _, err := ingestUpsert(ctx, q, ingestParams("acme:4", "Unrelated Engineer")); err != nil {
		t.Fatalf("upsert other: %v", err)
	}

	rows, err := q.JobDescriptionsByIDs(ctx, []int64{wanted.ID})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != wanted.ID {
		t.Fatalf("rows = %+v, want exactly the one named id %d", rows, wanted.ID)
	}
}
