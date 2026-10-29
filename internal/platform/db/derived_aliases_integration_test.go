//go:build integration

// Integration test for InsertDerivedCompanySlugAliases: the redirects cmd/backfill-derive
// records, and the registry invariants the statement holds on its own.
// Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"testing"
)

func TestInsertDerivedCompanySlugAliasesGuards(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE company_slug_aliases"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	seed := func(alias, canonical, key string) {
		t.Helper()
		if _, err := q.InsertCompanySlugAlias(ctx, InsertCompanySlugAliasParams{
			AliasSlug: alias, CanonicalSlug: canonical, FoldedKey: key, Reason: "spelling",
		}); err != nil {
			t.Fatalf("seed %s: %v", alias, err)
		}
	}
	seed("frozen-old", "frozen", "frozen")    // "frozen" is a canon
	seed("retired", "elsewhere", "elsewhere") // "retired" is itself retired
	seed("k-old", "k-canon", "kkey")          // key kkey belongs to k-canon
	seed("taken", "first", "taken")           // already retired once

	n, err := q.InsertDerivedCompanySlugAliases(ctx, InsertDerivedCompanySlugAliasesParams{
		AliasSlugs:     []string{"acme-pvt", "frozen", "chain-old", "k-other", "taken"},
		CanonicalSlugs: []string{"acme", "frozen-new", "retired", "k-new", "second"},
		FoldedKeys:     []string{"acme", "frozen", "chain", "kkey", "taken"},
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if n != 1 {
		t.Errorf("inserted %d, want only acme-pvt", n)
	}
	for alias, want := range map[string]string{
		"acme-pvt":  "acme",
		"frozen":    "",      // a canon never moves
		"chain-old": "",      // no redirect to a retired slug
		"k-other":   "",      // kkey already names k-canon
		"taken":     "first", // the canon froze at first merge
	} {
		var got string
		err := pool.QueryRow(ctx, `SELECT canonical_slug FROM company_slug_aliases WHERE alias_slug = $1`, alias).Scan(&got)
		if err != nil && want != "" {
			t.Errorf("%s: %v", alias, err)
		}
		if got != want {
			t.Errorf("%s -> %q, want %q", alias, got, want)
		}
	}
	var reason string
	if err := pool.QueryRow(ctx, `SELECT reason FROM company_slug_aliases WHERE alias_slug = 'acme-pvt'`).Scan(&reason); err != nil || reason != "legal_form" {
		t.Errorf("reason = %q, %v; want legal_form", reason, err)
	}
}
