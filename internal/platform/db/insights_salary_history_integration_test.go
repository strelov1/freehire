//go:build integration

// Integration tests for the effective salary (jobs_effective_salary) every salary
// rollup reads, and the monthly salary history (insights_salary_monthly).
// Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// setSalaryTier writes one salary tier's columns on a seeded posting. tier is
// "manual" or "source"; the enrichment tier is seeded through insightSeed.salary.
func setSalaryTier(t *testing.T, ctx context.Context, pool *pgxpool.Pool, ext, tier string, amount int, currency, period string) {
	t.Helper()
	sql := `UPDATE jobs SET salary_min_` + tier + ` = $1, salary_max_` + tier + ` = $1,
		salary_currency_` + tier + ` = $2, salary_period_` + tier + ` = $3 WHERE external_id = $4`
	if _, err := pool.Exec(ctx, sql, amount, currency, period, ext); err != nil {
		t.Fatalf("set %s salary on %s: %v", tier, ext, err)
	}
}

func TestInsightsSalaryBandsReadTheEffectiveSalary(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	base := insightSeed{category: "sales", seniority: "senior", countries: []string{"us"}, createdAgo: 5}

	// all3: manual, source and enrichment disagree — only the manual figure counts, in
	// the manual currency and period.
	all3 := base
	all3.salary, all3.currency, all3.period = 50, "USD", "year"
	seedInsightsJob(t, ctx, q, pool, "all3", all3)
	setSalaryTier(t, ctx, pool, "all3", "source", 100, "EUR", "month")
	setSalaryTier(t, ctx, pool, "all3", "manual", 200000, "USD", "year")

	// src: a posting never enriched (non-technical) whose ATS states a salary.
	seedInsightsJob(t, ctx, q, pool, "src", base)
	setSalaryTier(t, ctx, pool, "src", "source", 90000, "usd", "year")

	// llm: enrichment only.
	llm := base
	llm.salary, llm.currency, llm.period = 70000, "USD", "year"
	seedInsightsJob(t, ctx, q, pool, "llm", llm)

	// none: no salary anywhere.
	seedInsightsJob(t, ctx, q, pool, "none", base)

	if err := q.DeleteAllInsightsSalaryStats(ctx); err != nil {
		t.Fatalf("delete salary: %v", err)
	}
	if _, err := q.RebuildInsightsSalaryStatsGlobal(ctx, 1); err != nil {
		t.Fatalf("rebuild salary global: %v", err)
	}
	if _, err := q.RebuildInsightsSalaryStatsByCountry(ctx, 1); err != nil {
		t.Fatalf("rebuild salary by country: %v", err)
	}

	for _, country := range []string{"", "us"} {
		bands, err := q.ListInsightsSalary(ctx, ListInsightsSalaryParams{Category: "sales", Seniority: "senior", Country: country})
		if err != nil {
			t.Fatalf("list salary: %v", err)
		}
		// One USD/year band of {70k, 90k, 200k}; no EUR band — the source tier of
		// all3 lost to its manual tier.
		if len(bands) != 1 {
			t.Fatalf("country %q: bands = %+v, want exactly one", country, bands)
		}
		b := bands[0]
		if b.Currency != "USD" || b.Period != "year" || b.SampleSize != 3 || b.P50 != 90000 {
			t.Errorf("country %q: band = %+v, want USD/year n=3 p50=90000", country, b)
		}
	}
}

// monthPosting is one posting for the monthly-history test, dated by calendar month
// offsets from the current month rather than days, so the test cannot straddle a
// month boundary the way "N days ago" would on the 1st.
type monthPosting struct {
	category               string
	publishedAgo, foundAgo int // months before the current one
	amount                 int
	currency, period       string
	closed, dup, private   bool
}

func seedMonthPostings(t *testing.T, ctx context.Context, q *Queries, pool *pgxpool.Pool, prefix string, n int, p monthPosting) {
	t.Helper()
	for i := range n {
		ext := fmt.Sprintf("%s-%d", prefix, i)
		seedInsightsJob(t, ctx, q, pool, ext, insightSeed{category: p.category, createdAgo: 1})
		setSalaryTier(t, ctx, pool, ext, "source", p.amount+i, p.currency, p.period)
		if _, err := pool.Exec(ctx, `
			UPDATE jobs SET
				posted_at  = date_trunc('month', now()) - make_interval(months => $1) + interval '10 days',
				created_at = date_trunc('month', now()) - make_interval(months => $2) + interval '12 days',
				closed_at  = CASE WHEN $3 THEN now() ELSE NULL END,
				is_private = $4
			WHERE external_id = $5`,
			p.publishedAgo, p.foundAgo, p.closed, p.private, ext); err != nil {
			t.Fatalf("date %s: %v", ext, err)
		}
		if p.dup {
			if _, err := pool.Exec(ctx, `UPDATE jobs SET duplicate_of_role = (SELECT min(id) FROM jobs) WHERE external_id = $1`, ext); err != nil {
				t.Fatalf("dup %s: %v", ext, err)
			}
		}
	}
}

func TestInsightsSalaryMonthlyRollup(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	usdYear := monthPosting{category: "backend", amount: 100000, currency: "USD", period: "year"}

	// Two months ago: 28 ordinary postings, one closed since, and one published then
	// but found this month — 30, all belonging to that month.
	two := usdYear
	two.publishedAgo, two.foundAgo = 2, 2
	seedMonthPostings(t, ctx, q, pool, "two", 28, two)
	closed := two
	closed.closed = true
	seedMonthPostings(t, ctx, q, pool, "two-closed", 1, closed)
	late := two
	late.foundAgo = 0
	seedMonthPostings(t, ctx, q, pool, "two-late", 1, late)
	// A duplicate and a private posting in the same month would make it 32.
	dup := two
	dup.dup = true
	seedMonthPostings(t, ctx, q, pool, "two-dup", 1, dup)
	priv := two
	priv.private = true
	seedMonthPostings(t, ctx, q, pool, "two-priv", 1, priv)

	// Same month, another currency and period: its own row.
	eur := two
	eur.currency, eur.period, eur.amount = "EUR", "month", 5000
	seedMonthPostings(t, ctx, q, pool, "two-eur", 30, eur)

	// Last month: 29 — under the floor, not stored.
	one := usdYear
	one.publishedAgo, one.foundAgo = 1, 1
	seedMonthPostings(t, ctx, q, pool, "one", 29, one)

	// Thirteen months ago: outside the 12-month window.
	old := usdYear
	old.publishedAgo, old.foundAgo = 13, 13
	seedMonthPostings(t, ctx, q, pool, "old", 30, old)

	if err := q.DeleteAllInsightsSalaryMonthly(ctx); err != nil {
		t.Fatalf("delete monthly: %v", err)
	}
	if _, err := q.RebuildInsightsSalaryMonthly(ctx, 30); err != nil {
		t.Fatalf("rebuild monthly: %v", err)
	}
	rows, err := q.ListInsightsSalaryHistory(ctx, "backend")
	if err != nil {
		t.Fatalf("list history: %v", err)
	}

	// From the 1st: AddDate(0, -2, 0) on 30 April lands on 2 March, not February.
	now := time.Now().UTC()
	wantMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -2, 0).Format("2006-01")
	if len(rows) != 2 {
		t.Fatalf("history = %+v, want exactly two rows (EUR/month and USD/year, %s)", rows, wantMonth)
	}
	got := map[string]ListInsightsSalaryHistoryRow{}
	for _, r := range rows {
		got[r.Currency+"/"+r.Period] = r
		if m := r.Month.Time.Format("2006-01"); m != wantMonth {
			t.Errorf("%s/%s month = %s, want %s", r.Currency, r.Period, m, wantMonth)
		}
	}
	if usd := got["USD/year"]; usd.SampleSize != 30 || usd.P50 == 0 {
		t.Errorf("USD/year = %+v, want sample 30 (closed and late-found in, duplicate and private out)", usd)
	}
	if e := got["EUR/month"]; e.SampleSize != 30 {
		t.Errorf("EUR/month = %+v, want sample 30", e)
	}
}
