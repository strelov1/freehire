//go:build integration

// Integration tests for the Trends & Insights rollups (insights_*). The recompute
// logic lives entirely in SQL (the Rebuild* queries cmd/rollup-stats calls), so it
// is only verifiable against a real Postgres. These seed jobs with known facets,
// timestamps, and salaries, run the recompute, and assert the rollups and the read
// queries. Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// insightSeed is one job to plant: its role facets, geography, skills, age, an
// optional closure age, and an optional single-figure salary.
type insightSeed struct {
	category   string
	seniority  string
	skills     []string
	countries  []string
	createdAgo int  // days before now
	closedAgo  *int // days before now; nil = still open
	salary     int  // 0 = no salary disclosed
	currency   string
	period     string
}

func seedInsightsJob(t *testing.T, ctx context.Context, q *Queries, pool *pgxpool.Pool, ext string, s insightSeed) {
	t.Helper()
	if _, err := q.UpsertJob(ctx, ingestParams(ext, "Job "+ext)); err != nil {
		t.Fatalf("seed upsert %s: %v", ext, err)
	}
	enrichment := "{}"
	if s.salary > 0 {
		enrichment = fmt.Sprintf(`{"salary_min":%d,"salary_max":%d,"salary_currency":%q,"salary_period":%q}`,
			s.salary, s.salary, s.currency, s.period)
	}
	var closed any
	if s.closedAgo != nil {
		closed = *s.closedAgo
	}
	// skills/countries are NOT NULL text[]; a nil Go slice would bind as NULL.
	if s.skills == nil {
		s.skills = []string{}
	}
	if s.countries == nil {
		s.countries = []string{}
	}
	if _, err := pool.Exec(ctx, `
		UPDATE jobs SET
			category   = $1,
			seniority  = $2,
			skills     = $3,
			countries  = $4,
			created_at = now() - make_interval(days => $5::int),
			closed_at  = CASE WHEN $6::int IS NULL THEN NULL ELSE now() - make_interval(days => $6::int) END,
			enrichment = $7::jsonb
		WHERE external_id = $8`,
		s.category, s.seniority, s.skills, s.countries, s.createdAgo, closed, enrichment, ext,
	); err != nil {
		t.Fatalf("seed facets %s: %v", ext, err)
	}
}

// windowStart is the growth-window start the worker supplies (30 days before now).
// Computed in Go: the day-scale offsets in these tests never sit near the boundary,
// so sub-second skew against the DB's now() is immaterial.
func windowStart() pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: time.Now().UTC().AddDate(0, 0, -30), Valid: true}
}

func TestInsightsRoleRollupGrowthAndGeography(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	// Same role (engineering/senior). r1,r2 in DE; r3 in US. r2 is newer than the
	// 30-day window, so it lifts open_count but not the prior-window count → growth.
	seedInsightsJob(t, ctx, q, pool, "r1", insightSeed{category: "engineering", seniority: "senior", countries: []string{"de"}, createdAgo: 60})
	seedInsightsJob(t, ctx, q, pool, "r2", insightSeed{category: "engineering", seniority: "senior", countries: []string{"de"}, createdAgo: 10})
	seedInsightsJob(t, ctx, q, pool, "r3", insightSeed{category: "engineering", seniority: "senior", countries: []string{"us"}, createdAgo: 60})

	rebuildRoles(t, ctx, q, windowStart())

	// Global bucket ('' country): 3 open now, 2 open a window ago (r2 too new) → +1.
	global := findRole(t, roles(t, ctx, q, ""), "engineering", "senior")
	if global.OpenCount != 3 || global.Growth != 1 {
		t.Errorf("global eng/senior = {open %d, growth %d}, want {3, 1}", global.OpenCount, global.Growth)
	}
	// DE slice: r1,r2 → 2 open, 1 prior (r2 too new) → +1.
	de := findRole(t, roles(t, ctx, q, "de"), "engineering", "senior")
	if de.OpenCount != 2 || de.Growth != 1 {
		t.Errorf("DE eng/senior = {open %d, growth %d}, want {2, 1}", de.OpenCount, de.Growth)
	}
	// US slice: r3 only → 1 open, 1 prior → 0.
	us := findRole(t, roles(t, ctx, q, "us"), "engineering", "senior")
	if us.OpenCount != 1 || us.Growth != 0 {
		t.Errorf("US eng/senior = {open %d, growth %d}, want {1, 0}", us.OpenCount, us.Growth)
	}
}

func TestInsightsSalaryPerCurrencyAndSuppression(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	// Three USD jobs (100k/110k/120k) and one EUR job, same role. With min_sample=3
	// the USD band survives and the single EUR job is suppressed.
	for i, v := range []int{100000, 110000, 120000} {
		seedInsightsJob(t, ctx, q, pool, fmt.Sprintf("usd%d", i), insightSeed{
			category: "engineering", seniority: "senior", countries: []string{"us"},
			createdAgo: 5, salary: v, currency: "USD", period: "year",
		})
	}
	seedInsightsJob(t, ctx, q, pool, "eur0", insightSeed{
		category: "engineering", seniority: "senior", countries: []string{"de"},
		createdAgo: 5, salary: 90000, currency: "EUR", period: "year",
	})

	if err := q.DeleteAllInsightsSalaryStats(ctx); err != nil {
		t.Fatalf("delete salary: %v", err)
	}
	if _, err := q.RebuildInsightsSalaryStatsGlobal(ctx, 3); err != nil {
		t.Fatalf("rebuild salary global: %v", err)
	}

	bands, err := q.ListInsightsSalary(ctx, ListInsightsSalaryParams{Category: "engineering", Seniority: "senior", Country: ""})
	if err != nil {
		t.Fatalf("list salary: %v", err)
	}
	if len(bands) != 1 {
		t.Fatalf("bands = %d, want 1 (EUR suppressed by min sample)", len(bands))
	}
	b := bands[0]
	if b.Currency != "USD" || b.SampleSize != 3 {
		t.Errorf("band = {%s, n=%d}, want {USD, 3}", b.Currency, b.SampleSize)
	}
	// percentile_cont over [100k,110k,120k]: p25=105k, p50=110k, p75=115k.
	if b.P25 != 105000 || b.P50 != 110000 || b.P75 != 115000 {
		t.Errorf("percentiles = {%d,%d,%d}, want {105000,110000,115000}", b.P25, b.P50, b.P75)
	}

	// CUBE also materializes the seniority-only slice (category ''): a query with
	// seniority set and category omitted must still find the USD band.
	senOnly, err := q.ListInsightsSalary(ctx, ListInsightsSalaryParams{Category: "", Seniority: "senior", Country: ""})
	if err != nil {
		t.Fatalf("list salary seniority-only: %v", err)
	}
	if len(senOnly) != 1 || senOnly[0].Currency != "USD" || senOnly[0].SampleSize != 3 {
		t.Errorf("seniority-only band = %+v, want one USD band n=3", senOnly)
	}
}

func TestInsightsVelocityFaceted(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	closedAt := 1
	seedInsightsJob(t, ctx, q, pool, "v1", insightSeed{category: "engineering", countries: []string{"de"}, createdAgo: 2})
	seedInsightsJob(t, ctx, q, pool, "v2", insightSeed{category: "design", countries: []string{"fr"}, createdAgo: 2})
	seedInsightsJob(t, ctx, q, pool, "v3", insightSeed{category: "engineering", countries: []string{"de"}, createdAgo: 5, closedAgo: &closedAt})

	if err := q.DeleteAllInsightsVelocityDaily(ctx); err != nil {
		t.Fatalf("delete velocity: %v", err)
	}
	if _, err := q.RebuildInsightsVelocityDaily(ctx); err != nil {
		t.Fatalf("rebuild velocity: %v", err)
	}

	// 'all' slice over the last week: 3 added, 1 removed.
	if added, removed := velocitySum(t, ctx, q, "all", ""); added != 3 || removed != 1 {
		t.Errorf("all velocity = {added %d, removed %d}, want {3, 1}", added, removed)
	}
	// engineering slice: v1,v3 added; v3 removed.
	if added, removed := velocitySum(t, ctx, q, "category", "engineering"); added != 2 || removed != 1 {
		t.Errorf("engineering velocity = {added %d, removed %d}, want {2, 1}", added, removed)
	}
	// DE country slice: v1,v3 added; v3 removed.
	if added, removed := velocitySum(t, ctx, q, "country", "de"); added != 2 || removed != 1 {
		t.Errorf("DE velocity = {added %d, removed %d}, want {2, 1}", added, removed)
	}
}

func TestInsightsSkillRollupScoping(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	seedInsightsJob(t, ctx, q, pool, "s1", insightSeed{category: "engineering", countries: []string{"de"}, createdAgo: 5, skills: []string{"go", "sql"}})
	seedInsightsJob(t, ctx, q, pool, "s2", insightSeed{category: "engineering", countries: []string{"us"}, createdAgo: 5, skills: []string{"go"}})
	seedInsightsJob(t, ctx, q, pool, "s3", insightSeed{category: "design", countries: []string{"de"}, createdAgo: 5, skills: []string{"go"}})

	prev := windowStart()
	if err := q.DeleteAllInsightsSkillStats(ctx); err != nil {
		t.Fatalf("delete skills: %v", err)
	}
	for _, rebuild := range []func(context.Context, pgtype.Timestamptz) (int64, error){
		q.RebuildInsightsSkillStatsGlobal, q.RebuildInsightsSkillStatsByCategory, q.RebuildInsightsSkillStatsByCountry,
	} {
		if _, err := rebuild(ctx, prev); err != nil {
			t.Fatalf("rebuild skills: %v", err)
		}
	}

	// Global 'go' demand = all 3 jobs.
	if got := findSkill(t, skills(t, ctx, q, "", ""), "go"); got.OpenCount != 3 {
		t.Errorf("global go open = %d, want 3", got.OpenCount)
	}
	// engineering-scoped 'go' = s1,s2.
	if got := findSkill(t, skills(t, ctx, q, "engineering", ""), "go"); got.OpenCount != 2 {
		t.Errorf("engineering go open = %d, want 2", got.OpenCount)
	}
	// DE-scoped 'go' = s1,s3.
	if got := findSkill(t, skills(t, ctx, q, "", "de"), "go"); got.OpenCount != 2 {
		t.Errorf("DE go open = %d, want 2", got.OpenCount)
	}
}

// --- helpers ---

func rebuildRoles(t *testing.T, ctx context.Context, q *Queries, prev pgtype.Timestamptz) {
	t.Helper()
	if err := q.DeleteAllInsightsRoleStats(ctx); err != nil {
		t.Fatalf("delete roles: %v", err)
	}
	if _, err := q.RebuildInsightsRoleStatsGlobal(ctx, prev); err != nil {
		t.Fatalf("rebuild roles global: %v", err)
	}
	if _, err := q.RebuildInsightsRoleStatsByCountry(ctx, prev); err != nil {
		t.Fatalf("rebuild roles by country: %v", err)
	}
}

func roles(t *testing.T, ctx context.Context, q *Queries, country string) []ListInsightsRolesRow {
	t.Helper()
	rows, err := q.ListInsightsRoles(ctx, ListInsightsRolesParams{Country: country, Sort: "open", Lim: 100})
	if err != nil {
		t.Fatalf("list roles: %v", err)
	}
	return rows
}

func findRole(t *testing.T, rows []ListInsightsRolesRow, cat, sen string) ListInsightsRolesRow {
	t.Helper()
	for _, r := range rows {
		if r.Category == cat && r.Seniority == sen {
			return r
		}
	}
	t.Fatalf("role %s/%s not found in %d rows", cat, sen, len(rows))
	return ListInsightsRolesRow{}
}

func skills(t *testing.T, ctx context.Context, q *Queries, cat, country string) []ListInsightsSkillsRow {
	t.Helper()
	rows, err := q.ListInsightsSkills(ctx, ListInsightsSkillsParams{Category: cat, Country: country, Sort: "open", Lim: 100})
	if err != nil {
		t.Fatalf("list skills: %v", err)
	}
	return rows
}

func findSkill(t *testing.T, rows []ListInsightsSkillsRow, skill string) ListInsightsSkillsRow {
	t.Helper()
	for _, r := range rows {
		if r.Skill == skill {
			return r
		}
	}
	t.Fatalf("skill %s not found in %d rows", skill, len(rows))
	return ListInsightsSkillsRow{}
}

func velocitySum(t *testing.T, ctx context.Context, q *Queries, kind, value string) (int32, int32) {
	t.Helper()
	now := time.Now().UTC()
	rows, err := q.ListInsightsVelocity(ctx, ListInsightsVelocityParams{
		Unit:       "day",
		FromTs:     pgtype.Timestamp{Time: now.AddDate(0, 0, -8), Valid: true},
		ToTs:       pgtype.Timestamp{Time: now.AddDate(0, 0, 1), Valid: true},
		FacetKind:  kind,
		FacetValue: value,
	})
	if err != nil {
		t.Fatalf("list velocity: %v", err)
	}
	var a, r int32
	for _, row := range rows {
		a += row.Added
		r += row.Removed
	}
	return a, r
}
