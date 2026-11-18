//go:build integration

// GetJobsForDigest serves the salary the job page does (jobview.FromDomain): manual beats
// source beats the model's, and a stated layer replaces the whole range, currency and
// period included. Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"fmt"
	"testing"
)

func TestGetJobsForDigest_SalaryPrecedence(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	const model = `{"salary_min":50000,"salary_max":60000,"salary_currency":"USD","salary_period":"month"}`
	for _, tc := range []struct {
		name   string
		cols   string // extra column assignments for the INSERT
		values []any
		want   GetJobsForDigestRow
	}{
		{"manual beats source and model",
			"salary_min_manual, salary_currency_manual, salary_min_source, salary_max_source, salary_currency_source, salary_period_source",
			[]any{70000, "GBP", 90000, 120000, "EUR", "year"},
			GetJobsForDigestRow{SalaryMin: 70000, SalaryCurrency: "GBP"}},
		{"source beats model, whole range",
			"salary_min_source, salary_currency_source, salary_period_source, salary_max_manual, salary_currency_manual",
			[]any{90000, "", "year", nil, "GBP"},
			GetJobsForDigestRow{SalaryMin: 90000, SalaryPeriod: "year"}},
		{"model alone", "", nil,
			GetJobsForDigestRow{SalaryMin: 50000, SalaryMax: 60000, SalaryCurrency: "USD", SalaryPeriod: "month"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncate(t, pool)
			cols, args := "", []any{model}
			if tc.cols != "" {
				cols = ", " + tc.cols
				args = append(args, tc.values...)
			}
			placeholders := ""
			for i := range tc.values {
				placeholders += fmt.Sprintf(", $%d", i+2)
			}
			var id int64
			if err := pool.QueryRow(ctx, `INSERT INTO jobs (source, external_id, url, title, public_slug, enrichment`+cols+`)
				VALUES ('test', 'x', 'http://example.test', 'A job', 'job-x', $1`+placeholders+`) RETURNING id`, args...).Scan(&id); err != nil {
				t.Fatal(err)
			}
			rows, err := q.GetJobsForDigest(ctx, []int64{id})
			if err != nil || len(rows) != 1 {
				t.Fatalf("GetJobsForDigest: %v (%d rows)", err, len(rows))
			}
			got := rows[0]
			if got.SalaryMin != tc.want.SalaryMin || got.SalaryMax != tc.want.SalaryMax ||
				got.SalaryCurrency != tc.want.SalaryCurrency || got.SalaryPeriod != tc.want.SalaryPeriod {
				t.Errorf("salary = %d/%d %q %q, want %d/%d %q %q", got.SalaryMin, got.SalaryMax, got.SalaryCurrency, got.SalaryPeriod,
					tc.want.SalaryMin, tc.want.SalaryMax, tc.want.SalaryCurrency, tc.want.SalaryPeriod)
			}
		})
	}
}
