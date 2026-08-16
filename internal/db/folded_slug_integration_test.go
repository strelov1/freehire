//go:build integration

// Integration tests for jobs.company_slug_folded: every write path fills it, a rename
// keeps it in step, and the backfill closes the gap for rows that predate it. The
// column is maintained by SQL rather than by the engine (migrations/0109 explains why
// it is not GENERATED), so "does the value actually land" is only answerable against a
// real Postgres.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"strings"
	"testing"
)

func foldedOf(ctx context.Context, t *testing.T, q *Queries, id int64) string {
	t.Helper()
	job, err := q.GetJob(ctx, id)
	if err != nil {
		t.Fatalf("read job %d: %v", id, err)
	}
	return job.CompanySlugFolded.String
}

func TestCompanySlugFoldedIsFilledByUpsert(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	p := ingestParams("acme:1", "Backend Engineer")
	p.CompanySlug = "cfo-insights-gmbh"
	p.Company = "CFO Insights GmbH"
	job, err := ingestUpsert(ctx, q, p)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if got := foldedOf(ctx, t, q, job.ID); got != "cfoinsightsgmbh" {
		t.Fatalf("folded = %q, want %q", got, "cfoinsightsgmbh")
	}

	// The ON CONFLICT branch is a separate assignment list from the INSERT, and it is
	// the one a re-crawl takes — so it gets its own assertion rather than being assumed
	// to follow from the insert above.
	p.CompanySlug = "cfoinsights"
	p.Company = "Cfoinsights"
	if _, err := ingestUpsert(ctx, q, p); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	if got := foldedOf(ctx, t, q, job.ID); got != "cfoinsights" {
		t.Fatalf("folded after re-upsert = %q, want it to follow the new slug", got)
	}
}

// The fold exists so two spellings of one employer agree. This is that property stated
// as a test rather than as a comment: different slugs, same folded value.
func TestCompanySlugFoldedCollapsesSpellings(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	a := ingestParams("acme:a", "Backend Engineer")
	a.CompanySlug, a.Company = "cfo-insights", "CFO Insights"
	jobA, err := ingestUpsert(ctx, q, a)
	if err != nil {
		t.Fatalf("upsert a: %v", err)
	}
	b := ingestParams("acme:b", "Backend Engineer")
	b.CompanySlug, b.Company = "cfoinsights", "Cfoinsights"
	jobB, err := ingestUpsert(ctx, q, b)
	if err != nil {
		t.Fatalf("upsert b: %v", err)
	}
	if foldedOf(ctx, t, q, jobA.ID) != foldedOf(ctx, t, q, jobB.ID) {
		t.Fatalf("two spellings folded apart: %q vs %q",
			foldedOf(ctx, t, q, jobA.ID), foldedOf(ctx, t, q, jobB.ID))
	}
}

// RenameSlugCompany re-keys company_slug on every job under a slug-like company. If it
// left the folded column behind, those rows would stop matching the suppression pass
// while looking perfectly correct in the table.
func TestCompanySlugFoldedFollowsARename(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	p := ingestParams("acme:rename", "Backend Engineer")
	// The rename only touches rows whose company still looks like a slug.
	p.CompanySlug, p.Company = "acme-corp", "acme-corp"
	job, err := ingestUpsert(ctx, q, p)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	if _, err := q.RenameSlugCompany(ctx, RenameSlugCompanyParams{
		Name:    "Acme Corporation",
		NewSlug: "acme-corporation",
		OldSlug: "acme-corp",
	}); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if got := foldedOf(ctx, t, q, job.ID); got != "acmecorporation" {
		t.Fatalf("folded after rename = %q, want %q", got, "acmecorporation")
	}
}

// The backfill's contract: it fills rows that predate the column, it is idempotent, and
// re-running it writes nothing. The last part is what makes it safe to run repeatedly
// on a 7.4M-row table without bloating it.
func TestCompanySlugFoldedBackfill(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	p := ingestParams("acme:backfill", "Backend Engineer")
	p.CompanySlug, p.Company = "old-rows-ltd", "Old Rows Ltd"
	job, err := ingestUpsert(ctx, q, p)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	// Simulate a row written before the column existed.
	if _, err := pool.Exec(ctx, `UPDATE jobs SET company_slug_folded = NULL WHERE id = $1`, job.ID); err != nil {
		t.Fatalf("clear folded: %v", err)
	}

	bounds, err := q.CompanySlugFoldedBackfillBounds(ctx)
	if err != nil {
		t.Fatalf("bounds: %v", err)
	}
	if bounds.Remaining != 1 {
		t.Fatalf("remaining = %d, want 1", bounds.Remaining)
	}

	n, err := q.BackfillCompanySlugFoldedChunk(ctx, BackfillCompanySlugFoldedChunkParams{
		FromID: bounds.MinID, ToID: bounds.MaxID + 1,
	})
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if n != 1 {
		t.Fatalf("backfill filled %d rows, want 1", n)
	}
	if got := foldedOf(ctx, t, q, job.ID); got != "oldrowsltd" {
		t.Fatalf("folded after backfill = %q, want %q", got, "oldrowsltd")
	}

	// Second pass must be a no-op: the IS DISTINCT FROM guard is the difference between
	// "safe to re-run" and "rewrites 7.4M rows every time".
	again, err := q.BackfillCompanySlugFoldedChunk(ctx, BackfillCompanySlugFoldedChunkParams{
		FromID: bounds.MinID, ToID: bounds.MaxID + 1,
	})
	if err != nil {
		t.Fatalf("backfill again: %v", err)
	}
	if again != 0 {
		t.Fatalf("re-run wrote %d rows, want 0 — the guard is not holding", again)
	}
}

// The suppression pass must keep working while the backfill is still in flight: a row
// with a NULL folded column is simply not matched, never mismatched.
func TestSuppressionIgnoresUnbackfilledRows(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	ats := ingestParams("greenhouse:1", "Backend Engineer")
	ats.CompanySlug, ats.Company = "acme", "Acme"
	atsJob, err := ingestUpsert(ctx, q, ats)
	if err != nil {
		t.Fatalf("upsert ats: %v", err)
	}
	agg := ingestParams("remoteok:1", "Backend Engineer")
	agg.Source = "remoteok"
	agg.CompanySlug, agg.Company = "acme", "Acme"
	aggJob, err := ingestUpsert(ctx, q, agg)
	if err != nil {
		t.Fatalf("upsert agg: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET company_slug_folded = NULL WHERE id = $1`, aggJob.ID); err != nil {
		t.Fatalf("clear folded: %v", err)
	}

	if _, err := q.SuppressAggregatorDuplicatesForCompanies(ctx, SuppressAggregatorDuplicatesForCompaniesParams{
		FoldedCompanies: []string{strings.ReplaceAll("acme", "-", "")},
		Aggregators:     []string{"remoteok"},
	}); err != nil {
		t.Fatalf("suppress: %v", err)
	}

	got, err := q.GetJob(ctx, aggJob.ID)
	if err != nil {
		t.Fatalf("read agg: %v", err)
	}
	if got.DuplicateOf.Valid {
		t.Fatalf("an un-backfilled row was suppressed (duplicate_of=%d) — it must be skipped until filled",
			got.DuplicateOf.Int64)
	}
	// And the ATS row is untouched either way.
	if atsRow, err := q.GetJob(ctx, atsJob.ID); err != nil {
		t.Fatalf("read ats: %v", err)
	} else if atsRow.DuplicateOf.Valid {
		t.Fatal("the ATS row must stay canonical")
	}
}
