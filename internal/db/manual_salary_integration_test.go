//go:build integration

// Integration tests for the authoritative manual-salary overlay: SetJobEnrichment must
// coalesce a job's manual salary columns over the LLM payload it writes, so a
// recruiter-stated salary is never displaced by enrichment. SQL behavior — verifiable
// only against a real Postgres. Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// insertJobWithManualSalary inserts an open job carrying an authoritative manual salary.
func insertJobWithManualSalary(t *testing.T, pool *pgxpool.Pool, externalID string, min, max int, currency, period string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, public_slug,
		    salary_min_manual, salary_max_manual, salary_currency_manual, salary_period_manual)
		 VALUES ('test', $1, 'http://example.test', 'A job', 'job-' || $1, $2, $3, $4, $5)
		 RETURNING id`,
		externalID, min, max, currency, period).Scan(&id)
	if err != nil {
		t.Fatalf("insert job with manual salary: %v", err)
	}
	return id
}

// enrichedSalary is the salary projection read back off jobs.enrichment.
type enrichedSalary struct {
	Summary        string `json:"summary"`
	SalaryMin      *int   `json:"salary_min"`
	SalaryMax      *int   `json:"salary_max"`
	SalaryCurrency string `json:"salary_currency"`
	SalaryPeriod   string `json:"salary_period"`
}

func readEnrichment(t *testing.T, pool *pgxpool.Pool, id int64) enrichedSalary {
	t.Helper()
	var raw []byte
	if err := pool.QueryRow(context.Background(), "SELECT enrichment FROM jobs WHERE id = $1", id).Scan(&raw); err != nil {
		t.Fatalf("read enrichment: %v", err)
	}
	var s enrichedSalary
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("decode enrichment %s: %v", raw, err)
	}
	return s
}

func TestSetJobEnrichment_ManualSalaryOverlay(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	t.Run("manual salary overrides the enriched salary but keeps the rest of the payload", func(t *testing.T) {
		truncate(t, pool)
		id := insertJobWithManualSalary(t, pool, "withmanual", 90000, 120000, "EUR", "year")
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
			t.Errorf("salary range = %v/%v, want manual 90000/120000", got.SalaryMin, got.SalaryMax)
		}
		if got.SalaryCurrency != "EUR" || got.SalaryPeriod != "year" {
			t.Errorf("salary currency/period = %q/%q, want manual EUR/year", got.SalaryCurrency, got.SalaryPeriod)
		}
		if got.Summary != "keep me" {
			t.Errorf("summary = %q, want the payload's (non-salary fields untouched)", got.Summary)
		}
	})

	t.Run("a job with no manual salary keeps the enriched salary", func(t *testing.T) {
		truncate(t, pool)
		id := insertJob(t, pool, "nomanual")
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
			t.Errorf("salary = %v/%q, want the enriched 50000/USD (unchanged)", got.SalaryMin, got.SalaryCurrency)
		}
	})
}

// A manual salary supplied to the mint (UpsertManualJob) is persisted to the
// salary_*_manual columns AND seeded into the enrichment payload, so the vacancy shows
// its salary immediately — before any enrichment pass runs.
func TestUpsertManualJob_SeedsManualSalary(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)
	author := insertUser(t, pool, "seed@example.test")

	p := manualParams("https://acme.example/jobs/seed", "Go Dev", author, author)
	p.SalaryMinManual = pgtype.Int4{Int32: 90000, Valid: true}
	p.SalaryMaxManual = pgtype.Int4{Int32: 120000, Valid: true}
	p.SalaryCurrencyManual = "EUR"
	p.SalaryPeriodManual = "year"

	row, err := q.UpsertManualJob(ctx, p)
	if err != nil {
		t.Fatalf("UpsertManualJob: %v", err)
	}
	if !row.SalaryMinManual.Valid || row.SalaryMinManual.Int32 != 90000 || !row.SalaryMaxManual.Valid || row.SalaryMaxManual.Int32 != 120000 {
		t.Errorf("manual columns = %v/%v, want 90000/120000", row.SalaryMinManual, row.SalaryMaxManual)
	}
	if row.SalaryCurrencyManual != "EUR" || row.SalaryPeriodManual != "year" {
		t.Errorf("manual currency/period = %q/%q, want EUR/year", row.SalaryCurrencyManual, row.SalaryPeriodManual)
	}
	got := readEnrichment(t, pool, row.ID)
	if got.SalaryMin == nil || *got.SalaryMin != 90000 || got.SalaryMax == nil || *got.SalaryMax != 120000 || got.SalaryCurrency != "EUR" || got.SalaryPeriod != "year" {
		t.Errorf("seeded enrichment salary = %+v, want 90000-120000 EUR year", got)
	}
}
