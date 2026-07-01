//go:build integration

// Integration tests for the company-list name filter: ListCompanies and
// CountCompanies must filter by a case-insensitive substring of the name and
// treat an empty filter as "no filter". This is SQL behavior (ILIKE + the
// optional-predicate short-circuit), so it is verified against a real Postgres.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func insertCompany(t *testing.T, pool *pgxpool.Pool, slug, name string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO companies (slug, name) VALUES ($1, $2)`, slug, name); err != nil {
		t.Fatalf("insert company %q: %v", slug, err)
	}
}

func TestListCompaniesSearch(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, "TRUNCATE companies RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	insertCompany(t, pool, "acme", "Acme Corp")
	insertCompany(t, pool, "acme-labs", "ACME Labs")
	insertCompany(t, pool, "globex", "Globex")

	t.Run("search filters by name case-insensitively", func(t *testing.T) {
		rows, err := q.ListCompanies(ctx, ListCompaniesParams{Search: "acme", Limit: 50, Offset: 0})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(rows) != 2 {
			t.Fatalf("rows = %d, want 2 (both ACME companies)", len(rows))
		}
		for _, r := range rows {
			if !strings.Contains(strings.ToLower(r.Name), "acme") {
				t.Errorf("unexpected company %q in acme search", r.Name)
			}
		}
	})

	t.Run("empty search returns all", func(t *testing.T) {
		rows, err := q.ListCompanies(ctx, ListCompaniesParams{Search: "", Limit: 50, Offset: 0})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(rows) != 3 {
			t.Fatalf("rows = %d, want 3 (all companies)", len(rows))
		}
	})

	t.Run("count reflects the filter", func(t *testing.T) {
		got, err := q.CountCompanies(ctx, CountCompaniesParams{Search: "acme"})
		if err != nil {
			t.Fatalf("count: %v", err)
		}
		if got != 2 {
			t.Errorf("count(acme) = %d, want 2", got)
		}
		all, err := q.CountCompanies(ctx, CountCompaniesParams{Search: ""})
		if err != nil {
			t.Fatalf("count all: %v", err)
		}
		if all != 3 {
			t.Errorf("count(\"\") = %d, want 3", all)
		}
	})
}
