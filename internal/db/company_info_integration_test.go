//go:build integration

// Integration test for DeleteOrphanCompanies: it preserves reference rows (companies
// with no jobs of their own, such as those UpsertYCCompany inserts) while still
// sweeping jobless non-reference companies. This is SQL behavior, verified against a
// real Postgres. Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"
)

func TestCompanyExistsAndOrphanCleanupPreservesReference(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE companies RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	if _, err := pool.Exec(ctx, "TRUNCATE jobs RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate jobs: %v", err)
	}

	// A job-backed company, a jobless non-reference orphan, and a jobless reference row.
	insertCompany(t, pool, "withjob", "With Job")
	insertJobWithFacets(t, pool, "j1", "withjob", []string{}, []string{}, "{}")
	insertCompany(t, pool, "orphan", "Orphan Co") // is_reference defaults false
	if _, err := pool.Exec(ctx,
		`INSERT INTO companies (slug, name, is_reference) VALUES ('refco', 'Ref Co', true)`); err != nil {
		t.Fatalf("seed refco: %v", err)
	}

	t.Run("CompanyExists reflects presence", func(t *testing.T) {
		for slug, want := range map[string]bool{"withjob": true, "refco": true, "missing": false} {
			got, err := q.CompanyExists(ctx, slug)
			if err != nil {
				t.Fatalf("exists %s: %v", slug, err)
			}
			if got != want {
				t.Errorf("CompanyExists(%q) = %v, want %v", slug, got, want)
			}
		}
	})

	t.Run("orphan cleanup deletes jobless non-reference only", func(t *testing.T) {
		if _, err := q.DeleteOrphanCompanies(ctx); err != nil {
			t.Fatalf("delete orphans: %v", err)
		}
		for slug, wantExists := range map[string]bool{"withjob": true, "refco": true, "orphan": false} {
			got, err := q.CompanyExists(ctx, slug)
			if err != nil {
				t.Fatalf("exists %s: %v", slug, err)
			}
			if got != wantExists {
				t.Errorf("after cleanup CompanyExists(%q) = %v, want %v", slug, got, wantExists)
			}
		}
	})
}
