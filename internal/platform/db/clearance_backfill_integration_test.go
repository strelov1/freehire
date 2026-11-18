//go:build integration

// Integration tests for the nullable requires_clearance column: NULL until stated, and
// kept across a failed detail fetch — SQL behaviour only a real Postgres can confirm.
// Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func trueFlag() pgtype.Bool { return pgtype.Bool{Bool: true, Valid: true} }

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

func TestUpsertPreservesClearanceAfterFailedDetailFetch(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)
	p := ingestParams("acme:clearance", "Cleared Engineer")
	p.Description = "Active Secret clearance required. Health insurance and stock options provided."
	p.Equity = pgtype.Text{String: "salary_and_equity", Valid: true}
	p.Perks = []string{"health_insurance"}
	p.RequiresClearance = trueFlag()
	p.ClearanceLevels = []string{"secret"}
	if _, err := ingestUpsert(ctx, q, p); err != nil {
		t.Fatal(err)
	}
	p.Description = ""
	p.RequiresClearance = pgtype.Bool{}
	p.ClearanceLevels = nil
	p.Perks = nil
	p.Equity = pgtype.Text{}
	got, err := ingestUpsert(ctx, q, p)
	if err != nil {
		t.Fatal(err)
	}
	if !got.RequiresClearance.Bool || len(got.ClearanceLevels) != 1 || got.ClearanceLevels[0] != "secret" {
		t.Fatalf("failed detail fetch erased clearance: %+v, %v", got.RequiresClearance, got.ClearanceLevels)
	}
	if len(got.Perks) != 1 || got.Perks[0] != "health_insurance" {
		t.Fatalf("failed detail fetch erased perks: %v", got.Perks)
	}
	if !got.Equity.Valid || got.Equity.String != "salary_and_equity" {
		t.Fatalf("failed detail fetch erased equity: %+v", got.Equity)
	}
	p.Description = "Build public web applications."
	got, err = ingestUpsert(ctx, q, p)
	if err != nil {
		t.Fatal(err)
	}
	if got.RequiresClearance.Valid || len(got.ClearanceLevels) != 0 || len(got.Perks) != 0 || got.Equity.Valid {
		t.Fatalf("real edit retained description facets: %+v, %v, %v, %+v", got.RequiresClearance, got.ClearanceLevels, got.Perks, got.Equity)
	}
}
