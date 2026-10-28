//go:build integration

// Integration tests for SyncCompaniesFromJobs' write set. The statement runs for about an
// hour on prod inside one transaction, and every companies row it writes stays locked until
// it commits — while ingest's UpsertJob upserts into companies on every posting. A sync that
// rewrote the unchanged rows therefore stalled the whole crawl fleet behind it (2026-10-28:
// 124 UpsertJob waiting on one sync). Row locks are only observable against a real Postgres.
// Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestSyncCompaniesFromJobsWritesOnlyWhatChanged(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)
	ctx := context.Background()

	steady := ingestParams("s1", "Engineer")
	steady.Company, steady.CompanySlug = "Steady Co", "steady"
	stale := ingestParams("r1", "Engineer")
	stale.Company, stale.CompanySlug = "Renamed Co", "renamed"
	mustUpsert(t, q, steady)
	mustUpsert(t, q, stale)
	// One row whose name drifted from its jobs and one with no row at all: the two
	// writes the sync exists for.
	if _, err := pool.Exec(ctx, `UPDATE companies SET name = 'old name' WHERE slug = 'renamed'`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM companies WHERE slug = 'steady'`); err != nil {
		t.Fatal(err)
	}
	if err := q.SyncCompaniesFromJobs(ctx); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	var name string
	if err := pool.QueryRow(ctx, `SELECT name FROM companies WHERE slug = 'renamed'`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "Renamed Co" {
		t.Errorf("drifted name = %q, want %q", name, "Renamed Co")
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM companies WHERE slug = 'steady'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("missing company not inserted: %d rows", n)
	}

	// Now everything is in step. Hold a second sync's transaction open and try to lock a
	// row it had no reason to touch, the way ingest's company upsert would.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := q.WithTx(tx).SyncCompaniesFromJobs(ctx); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	_, err = pool.Exec(ctx, `SELECT 1 FROM companies WHERE slug = 'steady' FOR UPDATE NOWAIT`)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "55P03" {
		t.Fatal("a sync with nothing to change still locked an unchanged company row")
	}
	if err != nil {
		t.Fatal(err)
	}
}
