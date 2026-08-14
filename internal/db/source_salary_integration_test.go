//go:build integration

// Integration tests for the structured source-salary overlay (migration 0093):
// SetJobEnrichment must coalesce salary_*_source over the LLM payload it writes, but
// never over an authoritative manual salary — the effective precedence is
// manual > source > LLM-guessed. SQL behavior — verifiable only against a real
// Postgres. Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// insertJobWithSourceSalary inserts an open job carrying a structured ATS salary
// (Lever/Ashby/Recruitee), as ingest's UpsertJob would.
func insertJobWithSourceSalary(t *testing.T, pool *pgxpool.Pool, externalID string, min, max int, currency, period string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, public_slug,
		    salary_min_source, salary_max_source, salary_currency_source, salary_period_source)
		 VALUES ('test', $1, 'http://example.test', 'A job', 'job-' || $1, $2, $3, $4, $5)
		 RETURNING id`,
		externalID, min, max, currency, period).Scan(&id)
	if err != nil {
		t.Fatalf("insert job with source salary: %v", err)
	}
	return id
}

func TestSetJobEnrichment_SourceSalaryOverlay(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	t.Run("source salary overrides the LLM-guessed salary but keeps the rest of the payload", func(t *testing.T) {
		truncate(t, pool)
		id := insertJobWithSourceSalary(t, pool, "withsource", 90000, 120000, "EUR", "year")
		payload := json.RawMessage(`{"summary":"keep me","salary_min":50000,"salary_max":60000,"salary_currency":"USD","salary_period":"month"}`)
		if err := q.SetJobEnrichment(ctx, SetJobEnrichmentParams{
			Enrichment:        payload,
			EnrichedAt:        pgtype.Timestamptz{},
			EnrichmentVersion: 1,
			ID:                id,
		}); err != nil {
			t.Fatalf("SetJobEnrichment: %v", err)
		}
		got := readEnrichment(t, pool, id)
		if got.SalaryMin == nil || *got.SalaryMin != 90000 || got.SalaryMax == nil || *got.SalaryMax != 120000 {
			t.Errorf("salary range = %v/%v, want source 90000/120000", got.SalaryMin, got.SalaryMax)
		}
		if got.SalaryCurrency != "EUR" || got.SalaryPeriod != "year" {
			t.Errorf("salary currency/period = %q/%q, want source EUR/year", got.SalaryCurrency, got.SalaryPeriod)
		}
		if got.Summary != "keep me" {
			t.Errorf("summary = %q, want the payload's (non-salary fields untouched)", got.Summary)
		}
	})

	t.Run("manual salary still wins over a structured source salary", func(t *testing.T) {
		truncate(t, pool)
		var id int64
		err := pool.QueryRow(context.Background(),
			`INSERT INTO jobs (source, external_id, url, title, public_slug,
			    salary_min_source, salary_max_source, salary_currency_source, salary_period_source,
			    salary_min_manual, salary_max_manual, salary_currency_manual, salary_period_manual)
			 VALUES ('test', 'both', 'http://example.test', 'A job', 'job-both',
			    90000, 120000, 'EUR', 'year',
			    130000, 150000, 'GBP', 'year')
			 RETURNING id`).Scan(&id)
		if err != nil {
			t.Fatalf("insert job with source and manual salary: %v", err)
		}
		payload := json.RawMessage(`{"salary_min":50000,"salary_currency":"USD","salary_period":"month"}`)
		if err := q.SetJobEnrichment(ctx, SetJobEnrichmentParams{
			Enrichment:        payload,
			EnrichedAt:        pgtype.Timestamptz{},
			EnrichmentVersion: 1,
			ID:                id,
		}); err != nil {
			t.Fatalf("SetJobEnrichment: %v", err)
		}
		got := readEnrichment(t, pool, id)
		if got.SalaryMin == nil || *got.SalaryMin != 130000 || got.SalaryMax == nil || *got.SalaryMax != 150000 || got.SalaryCurrency != "GBP" {
			t.Errorf("salary = %v/%v %q, want the manual 130000/150000 GBP (manual > source > LLM)", got.SalaryMin, got.SalaryMax, got.SalaryCurrency)
		}
	})

	t.Run("a job with no source or manual salary keeps the LLM-guessed salary", func(t *testing.T) {
		truncate(t, pool)
		id := insertJob(t, pool, "neither")
		payload := json.RawMessage(`{"salary_min":50000,"salary_currency":"USD","salary_period":"year"}`)
		if err := q.SetJobEnrichment(ctx, SetJobEnrichmentParams{
			Enrichment:        payload,
			EnrichedAt:        pgtype.Timestamptz{},
			EnrichmentVersion: 1,
			ID:                id,
		}); err != nil {
			t.Fatalf("SetJobEnrichment: %v", err)
		}
		got := readEnrichment(t, pool, id)
		if got.SalaryMin == nil || *got.SalaryMin != 50000 || got.SalaryCurrency != "USD" {
			t.Errorf("salary = %v/%q, want the LLM-guessed 50000/USD (unchanged)", got.SalaryMin, got.SalaryCurrency)
		}
	})
}
