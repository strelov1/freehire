//go:build integration

// Integration test for the scoped companies reconcile cmd/backfill-derive runs after a slug
// re-key: InsertMissingCompaniesForSlugs and DeleteOrphanCompaniesForSlugs reach only the
// slugs they are given. Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"testing"
)

func TestCompaniesReconcileForSlugs(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE companies, jobs RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	job := func(externalID, slug, company, createdAt string, private bool) {
		t.Helper()
		if _, err := pool.Exec(ctx,
			`INSERT INTO jobs (source, external_id, url, title, public_slug, company_slug, company, created_at, is_private)
			 VALUES ('test', $1, 'http://example.test', 'A job', 'job-' || $1, $2, $3, $4::timestamptz, $5)`,
			externalID, slug, company, createdAt, private); err != nil {
			t.Fatalf("insert job %q: %v", externalID, err)
		}
	}
	// Moved onto: two names, the newer one wins.
	job("1", "acme", "ACME (old)", "2025-01-01", false)
	job("2", "acme", "Acme", "2026-01-01", false)
	// Moved onto, but only by a private row: no public company is named after it.
	job("3", "pasted", "Pasted Employer", "2026-01-01", true)
	// Not in the list: left alone even though it has no row.
	job("4", "unlisted", "Unlisted", "2026-01-01", false)
	// Moved away from: the old row goes. A listed slug that still holds a job stays, and
	// so does a reference row, and an unlisted orphan.
	insertCompany(t, pool, "acme-old", "Acme Old")
	insertCompany(t, pool, "still-used", "Still Used")
	job("5", "still-used", "Still Used", "2026-01-01", false)
	if _, err := pool.Exec(ctx,
		`INSERT INTO companies (slug, name, is_reference) VALUES ('ref-old', 'Ref', true)`); err != nil {
		t.Fatalf("seed ref-old: %v", err)
	}
	insertCompany(t, pool, "unlisted-orphan", "Orphan")

	created, err := q.InsertMissingCompaniesForSlugs(ctx, []string{"acme", "pasted", "still-used"})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	deleted, err := q.DeleteOrphanCompaniesForSlugs(ctx, []string{"acme-old", "still-used", "ref-old"})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if created != 1 || deleted != 1 {
		t.Errorf("created %d, deleted %d; want 1 and 1", created, deleted)
	}
	for slug, want := range map[string]bool{
		"acme": true, "pasted": false, "unlisted": false,
		"acme-old": false, "still-used": true, "ref-old": true, "unlisted-orphan": true,
	} {
		got, err := q.CompanyExists(ctx, slug)
		if err != nil {
			t.Fatalf("exists %s: %v", slug, err)
		}
		if got != want {
			t.Errorf("CompanyExists(%q) = %v, want %v", slug, got, want)
		}
	}
	if c, err := q.GetCompany(ctx, "acme"); err != nil || c.Name != "Acme" {
		t.Errorf("acme name = %q, %v; want the newest, Acme", c.Name, err)
	}
}
