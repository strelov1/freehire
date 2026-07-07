//go:build integration

// Integration tests for the job-reality repost-clustering: UpsertJob persists
// role_fingerprint, and RoleClusterCount / RoleClusterCountsAll group postings of the
// same role (by company_slug + role_fingerprint) into repost-history and concurrent
// open counts. Grouping and the NULL/empty-fingerprint exclusion are SQL behaviors
// verifiable only against a real Postgres.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func withFingerprint(externalID, title, fingerprint string) UpsertJobParams {
	p := ingestParams(externalID, title)
	if fingerprint != "" {
		p.RoleFingerprint = pgtype.Text{String: fingerprint, Valid: true}
	}
	return p
}

func TestRoleClusterCount_ClustersRepostsAndCountsOpen(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	// Three postings of ONE role under distinct external_ids (reposts over time),
	// plus an unrelated role and an unfingerprinted row that must not interfere.
	const fp = "role-abc"
	for _, ext := range []string{"acme:1", "acme:2", "acme:3"} {
		if _, err := q.UpsertJob(ctx, withFingerprint(ext, "Staff Engineer", fp)); err != nil {
			t.Fatalf("upsert %s: %v", ext, err)
		}
	}
	if _, err := q.UpsertJob(ctx, withFingerprint("acme:other", "Designer", "role-xyz")); err != nil {
		t.Fatalf("upsert other role: %v", err)
	}
	if _, err := q.UpsertJob(ctx, withFingerprint("acme:nofp", "Untagged", "")); err != nil {
		t.Fatalf("upsert unfingerprinted: %v", err)
	}

	// The role persisted its fingerprint and clusters all three postings.
	c, err := q.RoleClusterCount(ctx, RoleClusterCountParams{CompanySlug: "acme", RoleFingerprint: pgtype.Text{String: fp, Valid: true}})
	if err != nil {
		t.Fatalf("RoleClusterCount: %v", err)
	}
	if c.RepostCount != 3 {
		t.Errorf("repost_count = %d, want 3", c.RepostCount)
	}
	if c.MassCount != 3 {
		t.Errorf("mass_count = %d, want 3 (all open)", c.MassCount)
	}

	// Closing one posting drops the concurrent (open) count but keeps repost history.
	if _, err := pool.Exec(ctx, "UPDATE jobs SET closed_at = now() WHERE external_id = $1", "acme:3"); err != nil {
		t.Fatalf("close: %v", err)
	}
	c2, err := q.RoleClusterCount(ctx, RoleClusterCountParams{CompanySlug: "acme", RoleFingerprint: pgtype.Text{String: fp, Valid: true}})
	if err != nil {
		t.Fatalf("RoleClusterCount after close: %v", err)
	}
	if c2.RepostCount != 3 {
		t.Errorf("repost_count after close = %d, want 3 (history keeps closed)", c2.RepostCount)
	}
	if c2.MassCount != 2 {
		t.Errorf("mass_count after close = %d, want 2 (one closed)", c2.MassCount)
	}
}

// An empty/NULL fingerprint must never cluster: unfingerprinted rows would otherwise
// all group under one bucket and falsely read as a mass-posted ghost.
func TestRoleClusterCountsAll_ExcludesUnfingerprintedAndSingletons(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	// Two unfingerprinted rows (empty) — must not form a cluster.
	if _, err := q.UpsertJob(ctx, withFingerprint("acme:a", "Role A", "")); err != nil {
		t.Fatalf("upsert a: %v", err)
	}
	if _, err := q.UpsertJob(ctx, withFingerprint("acme:b", "Role B", "")); err != nil {
		t.Fatalf("upsert b: %v", err)
	}
	// A singleton fingerprinted role — excluded (HAVING COUNT(*) > 1).
	if _, err := q.UpsertJob(ctx, withFingerprint("acme:solo", "Solo", "role-solo")); err != nil {
		t.Fatalf("upsert solo: %v", err)
	}
	// A real cluster of two.
	for _, ext := range []string{"acme:c1", "acme:c2"} {
		if _, err := q.UpsertJob(ctx, withFingerprint(ext, "Cluster", "role-cluster")); err != nil {
			t.Fatalf("upsert %s: %v", ext, err)
		}
	}

	rows, err := q.RoleClusterCountsAll(ctx)
	if err != nil {
		t.Fatalf("RoleClusterCountsAll: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("clusters = %d, want 1 (only the real cluster)", len(rows))
	}
	if rows[0].RoleFingerprint.String != "role-cluster" || rows[0].RepostCount != 2 {
		t.Errorf("cluster = %+v, want role-cluster repost_count 2", rows[0])
	}
}
