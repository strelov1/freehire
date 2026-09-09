//go:build integration

// Integration test for dbStore.FillCompanyDescription — the ingest-time company-info
// write path a sources.CompanyDescriber adapter (Greenhouse today) feeds. Run with:
// go test -tags=integration ./cmd/ingest/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestFillCompanyDescription_NewCompanyIsInsertedAsNonReference(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE companies RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	store := newDBStore(pool, 1, nil, nil, defaultSeenPolicy())

	if err := store.FillCompanyDescription(ctx, "coinbase", "Coinbase", "Ready to be pushed beyond what you think you're capable of?"); err != nil {
		t.Fatalf("FillCompanyDescription: %v", err)
	}

	var isReference bool
	var companyInfo []byte
	if err := pool.QueryRow(ctx,
		`SELECT is_reference, company_info FROM companies WHERE slug = 'coinbase'`).Scan(&isReference, &companyInfo); err != nil {
		t.Fatalf("select: %v", err)
	}
	if isReference {
		t.Error("is_reference = true, want false for an ingest-discovered company")
	}
	var info map[string]string
	if err := json.Unmarshal(companyInfo, &info); err != nil {
		t.Fatalf("company_info not valid JSON: %v", err)
	}
	if info["summary"] == "" {
		t.Error("company_info.summary missing")
	}
}

func TestFillCompanyDescription_NeverOverwritesAnotherSourcesTagline(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE companies RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO companies (slug, name, tagline) VALUES ('acme', 'Acme', 'Tagline from YC')`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	store := newDBStore(pool, 1, nil, nil, defaultSeenPolicy())

	if err := store.FillCompanyDescription(ctx, "acme", "Acme", "From the Greenhouse board page"); err != nil {
		t.Fatalf("FillCompanyDescription: %v", err)
	}

	var tagline string
	if err := pool.QueryRow(ctx, `SELECT tagline FROM companies WHERE slug = 'acme'`).Scan(&tagline); err != nil {
		t.Fatalf("select: %v", err)
	}
	if tagline != "Tagline from YC" {
		t.Errorf("tagline = %q, want the stored one preserved", tagline)
	}
}
