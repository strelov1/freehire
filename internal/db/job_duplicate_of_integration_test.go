//go:build integration

// Integration tests for the role-duplicate recompute: driven per company
// (CompaniesWithRoleClusters + RecomputeRoleDuplicatesForCompany, as the reindex does),
// it collapses each role cluster (company_slug + role_fingerprint) to one canonical open
// job (min(id)), pointing the other open reposts at it via duplicate_of, while leaving
// singletons and unfingerprinted rows canonical. Canon failover on close and the min(id)
// tie-break are SQL behaviors verifiable only against a real Postgres.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// recomputeDuplicates runs the batched recompute exactly as the reindex does: fetch the
// companies needing work, then recompute each in its own short transaction.
func recomputeDuplicates(t *testing.T, q *Queries) {
	t.Helper()
	companies, err := q.CompaniesWithRoleClusters(context.Background())
	if err != nil {
		t.Fatalf("companies with clusters: %v", err)
	}
	for _, c := range companies {
		if _, err := q.RecomputeRoleDuplicatesForCompany(context.Background(), c); err != nil {
			t.Fatalf("recompute company %q: %v", c, err)
		}
	}
}

// dupOf reads a job's id and duplicate_of by external_id. dup is -1 when NULL (canon).
func dupOf(t *testing.T, pool *pgxpool.Pool, ext string) (id int64, dup int64) {
	t.Helper()
	var d *int64
	if err := pool.QueryRow(context.Background(),
		"SELECT id, duplicate_of FROM jobs WHERE external_id = $1", ext).Scan(&id, &d); err != nil {
		t.Fatalf("read %s: %v", ext, err)
	}
	if d == nil {
		return id, -1
	}
	return id, *d
}

func TestRecomputeRoleDuplicates_CollapsesClusterToMinIDCanon(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	// A cluster of three identical-fingerprint open jobs; first inserted has min(id).
	const fp = "role-dup"
	for _, ext := range []string{"acme:1", "acme:2", "acme:3"} {
		if _, err := q.UpsertJob(ctx, withFingerprint(ext, "Staff Engineer", fp)); err != nil {
			t.Fatalf("upsert %s: %v", ext, err)
		}
	}
	// A singleton fingerprinted role and an unfingerprinted row must stay canonical.
	if _, err := q.UpsertJob(ctx, withFingerprint("acme:solo", "Solo", "role-solo")); err != nil {
		t.Fatalf("upsert solo: %v", err)
	}
	if _, err := q.UpsertJob(ctx, withFingerprint("acme:nofp", "Untagged", "")); err != nil {
		t.Fatalf("upsert nofp: %v", err)
	}

	recomputeDuplicates(t, q)

	canonID, canonDup := dupOf(t, pool, "acme:1")
	if canonDup != -1 {
		t.Errorf("canon acme:1 duplicate_of = %d, want NULL", canonDup)
	}
	for _, ext := range []string{"acme:2", "acme:3"} {
		if _, dup := dupOf(t, pool, ext); dup != canonID {
			t.Errorf("%s duplicate_of = %d, want canon %d", ext, dup, canonID)
		}
	}
	if _, dup := dupOf(t, pool, "acme:solo"); dup != -1 {
		t.Errorf("singleton duplicate_of = %d, want NULL", dup)
	}
	if _, dup := dupOf(t, pool, "acme:nofp"); dup != -1 {
		t.Errorf("unfingerprinted duplicate_of = %d, want NULL", dup)
	}

	// Idempotent: a second run does not flip the canon.
	recomputeDuplicates(t, q)
	if _, dup := dupOf(t, pool, "acme:1"); dup != -1 {
		t.Errorf("canon changed on re-run: acme:1 duplicate_of = %d, want NULL", dup)
	}

	// Failover: close the canon; the next min(id) (acme:2) becomes canonical.
	if _, err := pool.Exec(ctx, "UPDATE jobs SET closed_at = now() WHERE external_id = $1", "acme:1"); err != nil {
		t.Fatalf("close canon: %v", err)
	}
	recomputeDuplicates(t, q)
	newCanonID, newCanonDup := dupOf(t, pool, "acme:2")
	if newCanonDup != -1 {
		t.Errorf("failover: acme:2 duplicate_of = %d, want NULL (new canon)", newCanonDup)
	}
	if _, dup := dupOf(t, pool, "acme:3"); dup != newCanonID {
		t.Errorf("failover: acme:3 duplicate_of = %d, want new canon %d", dup, newCanonID)
	}
}

// When a cluster shrinks to a single open row, the survivor's stale duplicate_of must be
// cleared — otherwise the last remaining posting stays hidden forever. CompaniesWithRoleClusters
// surfaces the company via its lingering marker even though it no longer has a cluster > 1.
func TestRecomputeRoleDuplicates_ClearsMarkerWhenClusterShrinksToOne(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	const fp = "role-dup"
	for _, ext := range []string{"acme:1", "acme:2"} {
		if _, err := q.UpsertJob(ctx, withFingerprint(ext, "Staff Engineer", fp)); err != nil {
			t.Fatalf("upsert %s: %v", ext, err)
		}
	}
	recomputeDuplicates(t, q)
	if _, dup := dupOf(t, pool, "acme:2"); dup == -1 {
		t.Fatal("precondition: acme:2 should be a repost after first recompute")
	}

	// Close the canon, leaving acme:2 as the only open row in its cluster.
	if _, err := pool.Exec(ctx, "UPDATE jobs SET closed_at = now() WHERE external_id = $1", "acme:1"); err != nil {
		t.Fatalf("close canon: %v", err)
	}
	recomputeDuplicates(t, q)

	if _, dup := dupOf(t, pool, "acme:2"); dup != -1 {
		t.Errorf("acme:2 duplicate_of = %d, want NULL (sole survivor must be canonical)", dup)
	}
}

func TestDuplicateReposts_HiddenFromListAndEnrichment(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	const fp = "role-dup"
	for _, ext := range []string{"acme:1", "acme:2"} {
		if _, err := q.UpsertJob(ctx, withFingerprint(ext, "Staff Engineer", fp)); err != nil {
			t.Fatalf("upsert %s: %v", ext, err)
		}
	}
	recomputeDuplicates(t, q)
	canonID, _ := dupOf(t, pool, "acme:1")
	repostID, _ := dupOf(t, pool, "acme:2")

	// ListJobs returns the canon, not the repost.
	jobs, err := q.ListJobs(ctx, ListJobsParams{Limit: 100, Offset: 0})
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	seen := map[int64]bool{}
	for _, j := range jobs {
		seen[j.ID] = true
	}
	if !seen[canonID] {
		t.Errorf("ListJobs missing canon %d", canonID)
	}
	if seen[repostID] {
		t.Errorf("ListJobs returned repost %d, want it hidden", repostID)
	}

	// EnqueuePendingJobs enqueues the canon, not the repost.
	if _, err := q.EnqueuePendingJobs(ctx, EnqueuePendingJobsParams{TargetVersion: 1}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if !outboxHas(t, pool, canonID) {
		t.Errorf("canon %d not enqueued", canonID)
	}
	if outboxHas(t, pool, repostID) {
		t.Errorf("repost %d enqueued, want it skipped", repostID)
	}
}

func outboxHas(t *testing.T, pool *pgxpool.Pool, jobID int64) bool {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM enrichment_outbox WHERE job_id = $1", jobID).Scan(&n); err != nil {
		t.Fatalf("outbox count %d: %v", jobID, err)
	}
	return n > 0
}
