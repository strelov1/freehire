//go:build integration

// Integration test for the merge worker's scoped catalogue reconcile
// (InsertMissingCanonicalCompanies + DeleteOrphanAliasCompanies): it must make a retired slug
// 301 and its canonical answer 200, and touch nothing outside the alias registry.
// Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"testing"
)

func TestMergeReconcileIsScopedToTheAliasRegistry(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE companies, company_slug_aliases, jobs RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	job := func(externalID, slug, company string) {
		t.Helper()
		if _, err := pool.Exec(ctx,
			`INSERT INTO jobs (source, external_id, url, title, public_slug, company_slug, company)
			 VALUES ('test', $1, 'http://example.test', 'A job', 'job-' || $1, $2, $3)`,
			externalID, slug, company); err != nil {
			t.Fatalf("insert job %q: %v", externalID, err)
		}
	}
	alias := func(aliasSlug, canonical string) {
		t.Helper()
		if _, err := q.InsertCompanySlugAlias(ctx, InsertCompanySlugAliasParams{
			AliasSlug: aliasSlug, CanonicalSlug: canonical, FoldedKey: canonical, Reason: "spelling",
		}); err != nil {
			t.Fatalf("insert alias %s: %v", aliasSlug, err)
		}
	}

	// A merge whose re-key ran and whose reconcile did not: the jobs carry the canonical, the
	// retired row is still there, and the canonical has no row of its own.
	insertCompany(t, pool, "centerforce-kk", "CenterForce")
	alias("centerforce-kk", "center-force-kk")
	job("1", "center-force-kk", "Center Force")
	// A retired slug that is a reference row: kept, as DeleteOrphanCompanies keeps it.
	if _, err := pool.Exec(ctx,
		`INSERT INTO companies (slug, name, is_reference) VALUES ('ref-old', 'Ref', true)`); err != nil {
		t.Fatalf("seed ref-old: %v", err)
	}
	alias("ref-old", "center-force-kk")
	// A retired slug a crawl wrote a job to again since: it still holds a job, so it stays.
	insertCompany(t, pool, "revived", "Revived")
	alias("revived", "center-force-kk")
	job("2", "revived", "Revived")
	// Outside the registry: an orphan row and a job slug with no row. Neither is the merge's
	// business, and the full-table sync this replaces is exactly what reached them.
	insertCompany(t, pool, "unrelated-orphan", "Orphan")
	job("3", "unrelated-rowless", "Rowless")

	inserted, err := q.InsertMissingCanonicalCompanies(ctx)
	if err != nil {
		t.Fatalf("insert canonicals: %v", err)
	}
	deleted, err := q.DeleteOrphanAliasCompanies(ctx)
	if err != nil {
		t.Fatalf("delete orphan aliases: %v", err)
	}
	if inserted != 1 || deleted != 1 {
		t.Errorf("inserted %d, deleted %d; want 1 and 1", inserted, deleted)
	}
	for slug, want := range map[string]bool{
		"center-force-kk":   true,
		"centerforce-kk":    false,
		"ref-old":           true,
		"revived":           true,
		"unrelated-orphan":  true,
		"unrelated-rowless": false,
	} {
		got, err := q.CompanyExists(ctx, slug)
		if err != nil {
			t.Fatalf("exists %s: %v", slug, err)
		}
		if got != want {
			t.Errorf("CompanyExists(%q) = %v, want %v", slug, got, want)
		}
	}

	// A re-run finds nothing left to do.
	if n, err := q.InsertMissingCanonicalCompanies(ctx); err != nil || n != 0 {
		t.Errorf("re-run insert = %d, %v; want 0", n, err)
	}
	if n, err := q.DeleteOrphanAliasCompanies(ctx); err != nil || n != 0 {
		t.Errorf("re-run delete = %d, %v; want 0", n, err)
	}
}
