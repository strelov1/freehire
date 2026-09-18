//go:build integration

// Integration tests for the Trends & Insights rollups (insights_*). The recompute
// logic lives entirely in SQL (the Rebuild* queries cmd/rollup-stats calls), so it
// is only verifiable against a real Postgres. These seed jobs with known facets,
// timestamps, and salaries, run the recompute, and assert the rollups and the read
// queries. Run with: go test -tags=integration ./internal/platform/db/
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

func TestInsightsSalaryCurrencyCaseMerges(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	// Same role, same currency in mixed case ('USD' and 'usd'): the rollup upper()s
	// the code so both fold into one band rather than two split ones.
	for i, cur := range []string{"USD", "usd", "USD", "usd"} {
		seedInsightsJob(t, ctx, q, pool, fmt.Sprintf("c%d", i), insightSeed{
			category: "engineering", seniority: "senior", countries: []string{"us"},
			createdAgo: 5, salary: 150000, currency: cur, period: "year",
		})
	}
	if err := q.DeleteAllInsightsSalaryStats(ctx); err != nil {
		t.Fatalf("delete salary: %v", err)
	}
	if _, err := q.RebuildInsightsSalaryStatsGlobal(ctx, 1); err != nil {
		t.Fatalf("rebuild salary: %v", err)
	}

	bands, err := q.ListInsightsSalary(ctx, ListInsightsSalaryParams{Category: "engineering", Seniority: "senior", Country: ""})
	if err != nil {
		t.Fatalf("list salary: %v", err)
	}
	if len(bands) != 1 || bands[0].Currency != "USD" || bands[0].SampleSize != 4 {
		t.Errorf("bands = %+v, want a single USD band with sample 4 (usd+USD merged)", bands)
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

// TestInsightsSkillHistorySnapshotIdempotentAndPruned exercises the three queries
// GET /me/market-pulse's data depends on: the snapshot insert is idempotent within
// a week (the "no separate weekly scheduler" decision relies on this), the read
// returns what was snapshotted, and pruning removes only rows past the cutoff.
func TestInsightsSkillHistorySnapshotIdempotentAndPruned(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	seedInsightsJob(t, ctx, q, pool, "h1", insightSeed{createdAgo: 5, skills: []string{"go"}})
	seedInsightsJob(t, ctx, q, pool, "h2", insightSeed{createdAgo: 5, skills: []string{"go", "rust"}})

	if err := q.DeleteAllInsightsSkillStats(ctx); err != nil {
		t.Fatalf("delete skills: %v", err)
	}
	if _, err := q.RebuildInsightsSkillStatsGlobal(ctx, windowStart()); err != nil {
		t.Fatalf("rebuild skills global: %v", err)
	}

	thisWeek := pgtype.Date{Time: time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC), Valid: true}

	n, err := q.SnapshotInsightsSkillHistory(ctx, thisWeek)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if n != 2 { // go, rust
		t.Errorf("first snapshot rows = %d, want 2", n)
	}

	// Same week again: the intra-day rollup-stats cadence calls this several times a
	// week; only the first run within a week should insert.
	n, err = q.SnapshotInsightsSkillHistory(ctx, thisWeek)
	if err != nil {
		t.Fatalf("snapshot again: %v", err)
	}
	if n != 0 {
		t.Errorf("second snapshot rows = %d, want 0 (idempotent)", n)
	}

	rows, err := q.ListInsightsSkillHistory(ctx, []string{"go", "rust"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("history rows = %d, want 2", len(rows))
	}
	if got := findSkillHistory(t, rows, "go"); got.OpenCount != 2 {
		t.Errorf("go open_count = %d, want 2", got.OpenCount)
	}
	if got := findSkillHistory(t, rows, "rust"); got.OpenCount != 1 {
		t.Errorf("rust open_count = %d, want 1", got.OpenCount)
	}

	// Plant a row well past the retention window directly (SnapshotInsightsSkillHistory
	// only ever writes the current week), then confirm prune drops it and nothing else.
	oldWeek := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx,
		`INSERT INTO insights_skill_history (skill, week_start, open_count) VALUES ('go', $1, 1)`,
		oldWeek); err != nil {
		t.Fatalf("seed old row: %v", err)
	}

	cutoff := pgtype.Date{Time: time.Now().UTC().AddDate(0, 0, -182), Valid: true} // ~26 weeks
	pruned, err := q.PruneInsightsSkillHistory(ctx, cutoff)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if pruned != 1 {
		t.Errorf("pruned rows = %d, want 1", pruned)
	}

	rows, err = q.ListInsightsSkillHistory(ctx, []string{"go"})
	if err != nil {
		t.Fatalf("list after prune: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("rows after prune = %d, want 1 (only this week's row survives)", len(rows))
	}
}

// --- helpers ---

func findSkillHistory(t *testing.T, rows []InsightsSkillHistory, skill string) InsightsSkillHistory {
	t.Helper()
	for _, r := range rows {
		if r.Skill == skill {
			return r
		}
	}
	t.Fatalf("skill %s not found in %d history rows", skill, len(rows))
	return InsightsSkillHistory{}
}

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

// TestInsightsRoleSkillRollup exercises the per-role skill distribution and the
// separate denominator it is divided by. The two are separate rollups on purpose:
// measured on production 2026-09-18, 11% of the eligible postings carry no tagged
// skill at all, so dividing a skill's count by the role's OPEN count would fold our
// own tagging gap into every published share — and because that gap differs per
// role, it would make two roles' shares incomparable.
func TestInsightsRoleSkillRollup(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	// backend/senior: four open postings, one of which carries no skill at all, plus
	// one closed posting whose skills must not count.
	seedInsightsJob(t, ctx, q, pool, "rs1", insightSeed{category: "backend", seniority: "senior", createdAgo: 5, skills: []string{"go", "docker"}})
	seedInsightsJob(t, ctx, q, pool, "rs2", insightSeed{category: "backend", seniority: "senior", createdAgo: 5, skills: []string{"go", "docker"}})
	seedInsightsJob(t, ctx, q, pool, "rs3", insightSeed{category: "backend", seniority: "senior", createdAgo: 5, skills: []string{"go"}})
	seedInsightsJob(t, ctx, q, pool, "rs4", insightSeed{category: "backend", seniority: "senior", createdAgo: 5})
	closed := 1
	seedInsightsJob(t, ctx, q, pool, "rs5", insightSeed{category: "backend", seniority: "senior", createdAgo: 5, closedAgo: &closed, skills: []string{"go", "docker", "kafka"}})

	// A different seniority in the same category must not be folded in. Two postings,
	// not one, so junior clears the same floor senior does — with one it would be
	// suppressed and the assertion below could not tell "not mixed in" apart from
	// "below the floor".
	seedInsightsJob(t, ctx, q, pool, "rs6", insightSeed{category: "backend", seniority: "junior", createdAgo: 5, skills: []string{"go"}})
	seedInsightsJob(t, ctx, q, pool, "rs6b", insightSeed{category: "backend", seniority: "junior", createdAgo: 5, skills: []string{"go"}})
	// A posting with no seniority is out of scope entirely — the rollup describes
	// postings that STATE a level, which is only 39% of the catalogue.
	seedInsightsJob(t, ctx, q, pool, "rs7", insightSeed{category: "backend", createdAgo: 5, skills: []string{"go"}})

	rebuildRoleSkills(t, ctx, q, 2)

	// go is on rs1,rs2,rs3; docker on rs1,rs2. rs5 is closed and rs7 has no
	// seniority, so neither contributes.
	got := roleSkillCounts(t, ctx, q, "backend", "senior")
	want := map[string]int32{"go": 3, "docker": 2}
	if len(got) != len(want) {
		t.Errorf("backend/senior skills = %v, want %v", got, want)
	}
	for skill, n := range want {
		if got[skill] != n {
			t.Errorf("backend/senior %s = %d, want %d", skill, got[skill], n)
		}
	}
	if _, ok := got["kafka"]; ok {
		t.Errorf("closed posting's skill leaked into the rollup: %v", got)
	}

	// The denominator counts the role's SKILL-BEARING open postings (rs1..rs3), not
	// its four open ones: rs4 has no tagged skill.
	if n := roleSkillSample(t, ctx, q, "backend", "senior"); n != 3 {
		t.Errorf("backend/senior sample_size = %d, want 3 (rs4 carries no skill)", n)
	}

	// The sibling seniority is its own role, and its denominator is its own: junior's
	// two go postings must not raise senior's count of 3, nor vice versa.
	if got := roleSkillCounts(t, ctx, q, "backend", "junior"); got["go"] != 2 {
		t.Errorf("backend/junior go = %d, want 2", got["go"])
	}
	if n := roleSkillSample(t, ctx, q, "backend", "junior"); n != 2 {
		t.Errorf("backend/junior sample_size = %d, want 2", n)
	}

	// A raised floor drops docker (2) and keeps go (3). The floor applies to the
	// distribution only — the denominator must survive it, or a share that did clear
	// the floor would have nothing to divide by.
	rebuildRoleSkills(t, ctx, q, 3)
	got = roleSkillCounts(t, ctx, q, "backend", "senior")
	if _, ok := got["docker"]; ok {
		t.Errorf("docker (2) survived a floor of 3: %v", got)
	}
	if got["go"] != 3 {
		t.Errorf("go = %d, want 3 at floor 3", got["go"])
	}
	if n := roleSkillSample(t, ctx, q, "backend", "senior"); n != 3 {
		t.Errorf("sample_size = %d after raising the floor, want 3 — the floor must not reach the denominator", n)
	}

	// Rerunning writes the same rows: the worker's delete-and-reinsert is the only
	// thing between two runs, so a second run must not double any count.
	rebuildRoleSkills(t, ctx, q, 2)
	if got := roleSkillCounts(t, ctx, q, "backend", "senior"); got["go"] != 3 || got["docker"] != 2 {
		t.Errorf("rerun = %v, want go 3 / docker 2 (not idempotent)", got)
	}
}

func rebuildRoleSkills(t *testing.T, ctx context.Context, q *Queries, minSample int32) {
	t.Helper()
	if err := q.DeleteAllInsightsRoleSkillStats(ctx); err != nil {
		t.Fatalf("delete role skills: %v", err)
	}
	if _, err := q.RebuildInsightsRoleSkillStats(ctx, minSample); err != nil {
		t.Fatalf("rebuild role skills: %v", err)
	}
	if err := q.DeleteAllInsightsRoleSkillSample(ctx); err != nil {
		t.Fatalf("delete role skill sample: %v", err)
	}
	if _, err := q.RebuildInsightsRoleSkillSample(ctx); err != nil {
		t.Fatalf("rebuild role skill sample: %v", err)
	}
}

func roleSkillCounts(t *testing.T, ctx context.Context, q *Queries, cat, sen string) map[string]int32 {
	t.Helper()
	rows, err := q.ListInsightsRoleSkills(ctx, ListInsightsRoleSkillsParams{
		Category: cat, Seniority: sen, Lim: 100,
	})
	if err != nil {
		t.Fatalf("list role skills: %v", err)
	}
	out := map[string]int32{}
	for _, r := range rows {
		out[r.Skill] = r.OpenCount
	}
	return out
}

func roleSkillSample(t *testing.T, ctx context.Context, q *Queries, cat, sen string) int32 {
	t.Helper()
	n, err := q.GetInsightsRoleSkillSample(ctx, GetInsightsRoleSkillSampleParams{
		Category: cat, Seniority: sen,
	})
	if err != nil {
		t.Fatalf("get role skill sample: %v", err)
	}
	return n
}
