//go:build integration

// Integration test for the public Trends & Insights endpoints. The rollups are SQL
// over jobs and the handlers read through a concrete *db.Queries, so the wire
// contract — envelope shape, scoping, and (critically) that only aggregate data is
// exposed — can only be exercised against a real Postgres. It seeds jobs carrying
// distinctive record-level strings, recomputes the rollups, hits each route, and
// asserts the aggregates plus the absence of any leak.
// Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
)

func seedInsightsHandlerJob(t *testing.T, ctx context.Context, pool *pgxpool.Pool, q *db.Queries, n string, cat, sen string, countries, skills []string, salary int) {
	t.Helper()
	// Distinctive record-level fields that must never surface in an aggregate body.
	p := db.UpsertJobParams{
		Source: "greenhouse", ExternalID: "secret-ext-" + n, URL: "https://ex.test/secret-" + n,
		Title: "SECRET_TITLE_" + n, Company: "Acme", CompanySlug: "acme",
		PublicSlug: "secret-slug-" + n, Location: "Remote", Remote: true,
	}
	if _, err := q.UpsertJob(ctx, p); err != nil {
		t.Fatalf("seed %s: %v", n, err)
	}
	enrichment := "{}"
	if salary > 0 {
		s := strconv.Itoa(salary)
		enrichment = `{"salary_min":` + s + `,"salary_max":` + s + `,"salary_currency":"USD","salary_period":"year"}`
	}
	if _, err := pool.Exec(ctx, `
		UPDATE jobs SET category=$1, seniority=$2, countries=$3, skills=$4,
			created_at = now() - interval '2 days', enrichment=$5::jsonb
		WHERE external_id=$6`,
		cat, sen, countries, skills, enrichment, "secret-ext-"+n,
	); err != nil {
		t.Fatalf("seed facets %s: %v", n, err)
	}
}

func rebuildInsightsForTest(t *testing.T, ctx context.Context, q *db.Queries) {
	t.Helper()
	prev := pgtype.Timestamptz{Time: time.Now().UTC().AddDate(0, 0, -30), Valid: true}
	mustExec := func(err error) {
		if err != nil {
			t.Fatalf("rebuild: %v", err)
		}
	}
	mustExec(q.DeleteAllInsightsRoleStats(ctx))
	_, err := q.RebuildInsightsRoleStatsGlobal(ctx, prev)
	mustExec(err)
	_, err = q.RebuildInsightsRoleStatsByCountry(ctx, prev)
	mustExec(err)
	mustExec(q.DeleteAllInsightsSkillStats(ctx))
	_, err = q.RebuildInsightsSkillStatsGlobal(ctx, prev)
	mustExec(err)
	_, err = q.RebuildInsightsSkillStatsByCategory(ctx, prev)
	mustExec(err)
	_, err = q.RebuildInsightsSkillStatsByCountry(ctx, prev)
	mustExec(err)
	// Per-role skill demand at a floor of 2, and its denominator — which takes no floor,
	// or a surviving share would have nothing to divide by.
	mustExec(q.DeleteAllInsightsRoleSkillStats(ctx))
	_, err = q.RebuildInsightsRoleSkillStats(ctx, 2)
	mustExec(err)
	mustExec(q.DeleteAllInsightsRoleSkillSample(ctx))
	_, err = q.RebuildInsightsRoleSkillSample(ctx)
	mustExec(err)
	mustExec(q.DeleteAllInsightsSalaryStats(ctx))
	_, err = q.RebuildInsightsSalaryStatsGlobal(ctx, 1) // min sample 1 so a small band survives
	mustExec(err)
	_, err = q.RebuildInsightsSalaryStatsByCountry(ctx, 1)
	mustExec(err)
	mustExec(q.DeleteAllInsightsVelocityDaily(ctx))
	_, err = q.RebuildInsightsVelocityDaily(ctx)
	mustExec(err)
}

// getInsights issues one GET against the test app and returns the decoded body, the raw
// body (for failure messages, which is why it is returned even on success) and the status.
// A non-200 is left undecoded: an error envelope is not the shape a caller asserts on.
func getInsights(t *testing.T, ctx context.Context, app *fiber.App, path string) (map[string]any, string, int) {
	t.Helper()
	resp, err := app.Test(httptest.NewRequestWithContext(ctx, fiber.MethodGet, path, nil))
	if err != nil {
		t.Fatalf("request %s: %v", path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	if resp.StatusCode == fiber.StatusOK {
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
	}
	return out, string(raw), resp.StatusCode
}

func TestInsightsEndpoints(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE jobs, companies RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	seedInsightsHandlerJob(t, ctx, pool, q, "1", "backend", "senior", []string{"de"}, []string{"go"}, 100000)
	seedInsightsHandlerJob(t, ctx, pool, q, "2", "backend", "senior", []string{"de"}, []string{"go", "sql"}, 120000)
	seedInsightsHandlerJob(t, ctx, pool, q, "3", "backend", "senior", []string{"us"}, []string{"go"}, 0)
	seedInsightsHandlerJob(t, ctx, pool, q, "4", "design", "junior", []string{"fr"}, []string{"figma"}, 0)
	rebuildInsightsForTest(t, ctx, q)

	h := &statsHandlers{queries: q}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/insights/roles", h.InsightsRoles)
	app.Get("/api/v1/insights/skills", h.InsightsSkills)
	app.Get("/api/v1/insights/velocity", h.InsightsVelocity)
	app.Get("/api/v1/insights/salary", h.InsightsSalary)

	get := func(t *testing.T, path string) (map[string]any, string, int) {
		t.Helper()
		return getInsights(t, ctx, app, path)
	}

	bodies := []string{}

	// --- roles: DE slice has the backend/senior role with open_count 2 --------
	out, raw, code := get(t, "/api/v1/insights/roles?country=de")
	bodies = append(bodies, raw)
	if code != fiber.StatusOK {
		t.Fatalf("roles: status %d, want 200: %s", code, raw)
	}
	data, _ := out["data"].([]any)
	if !hasRole(data, "backend", "senior", 2) {
		t.Errorf("roles DE: missing backend/senior open_count=2: %s", raw)
	}

	// --- roles: invalid sort is a 400 -------------------------------------------
	if _, raw, code := get(t, "/api/v1/insights/roles?sort=bogus"); code != fiber.StatusBadRequest {
		t.Errorf("roles bad sort: status %d, want 400: %s", code, raw)
	}

	// --- skills: backend-scoped 'go' spans jobs 1,2,3 -----------------------
	out, raw, code = get(t, "/api/v1/insights/skills?category=backend")
	bodies = append(bodies, raw)
	if code != fiber.StatusOK {
		t.Fatalf("skills: status %d, want 200: %s", code, raw)
	}
	if got := skillOpen(out["data"], "go"); got != 3 {
		t.Errorf("skills backend go open = %v, want 3: %s", got, raw)
	}
	// skills reject category+country together.
	if _, raw, code := get(t, "/api/v1/insights/skills?category=backend&country=de"); code != fiber.StatusBadRequest {
		t.Errorf("skills both scopes: status %d, want 400: %s", code, raw)
	}

	// --- velocity: backend slice has recent additions -----------------------
	out, raw, code = get(t, "/api/v1/insights/velocity?granularity=day&category=backend")
	bodies = append(bodies, raw)
	if code != fiber.StatusOK {
		t.Fatalf("velocity: status %d, want 200: %s", code, raw)
	}
	if sumAdded(out["data"]) < 3 {
		t.Errorf("velocity backend added = %v, want >= 3: %s", sumAdded(out["data"]), raw)
	}

	// --- salary: backend/senior USD band present ----------------------------
	out, raw, code = get(t, "/api/v1/insights/salary?category=backend&seniority=senior")
	bodies = append(bodies, raw)
	if code != fiber.StatusOK {
		t.Fatalf("salary: status %d, want 200: %s", code, raw)
	}
	if !hasUSDBand(out["data"]) {
		t.Errorf("salary: missing USD band: %s", raw)
	}
	// seniority-only scope (category omitted) still resolves via CUBE.
	out, raw, code = get(t, "/api/v1/insights/salary?seniority=senior")
	bodies = append(bodies, raw)
	if code != fiber.StatusOK || !hasUSDBand(out["data"]) {
		t.Errorf("salary seniority-only: status %d, missing USD band: %s", code, raw)
	}

	// --- roles scoped to one category (SEO-page read) ---------------------------
	out, raw, code = get(t, "/api/v1/insights/roles?category=backend")
	bodies = append(bodies, raw)
	if code != fiber.StatusOK {
		t.Fatalf("roles?category: status %d, want 200: %s", code, raw)
	}
	rows, _ := out["data"].([]any)
	if len(rows) == 0 {
		t.Errorf("roles?category=backend returned no rows: %s", raw)
	}
	for _, e := range rows {
		if m, ok := e.(map[string]any); ok && m["category"] != "backend" {
			t.Errorf("roles?category=backend leaked category %v: %s", m["category"], raw)
		}
	}

	// --- per-category salary breakdown (all seniorities in one call) -------------
	out, raw, code = get(t, "/api/v1/insights/salary?category=backend")
	bodies = append(bodies, raw)
	if code != fiber.StatusOK {
		t.Fatalf("salary?category breakdown: status %d, want 200: %s", code, raw)
	}
	if out["meta"].(map[string]any)["breakdown"] != "seniority" {
		t.Errorf("salary?category: expected breakdown meta, got %v", out["meta"])
	}
	// Breakdown carries per-row seniority and includes the senior grade's USD band.
	foundSeniorUSD := false
	for _, e := range out["data"].([]any) {
		if m, ok := e.(map[string]any); ok && m["seniority"] == "senior" && m["currency"] == "USD" {
			foundSeniorUSD = true
		}
	}
	if !foundSeniorUSD {
		t.Errorf("salary?category=backend breakdown missing senior USD band: %s", raw)
	}

	// --- aggregate-only: no record-level string leaks into any body -------------
	for _, raw := range bodies {
		for _, leak := range []string{"SECRET_TITLE", "secret-slug", "secret-ext", "ex.test"} {
			if strings.Contains(raw, leak) {
				t.Errorf("aggregate body leaked %q:\n%s", leak, raw)
			}
		}
	}
}

func hasRole(data []any, cat, sen string, open float64) bool {
	for _, e := range data {
		m, ok := e.(map[string]any)
		if ok && m["category"] == cat && m["seniority"] == sen && m["open_count"] == open {
			return true
		}
	}
	return false
}

func skillOpen(data any, skill string) float64 {
	rows, _ := data.([]any)
	for _, e := range rows {
		if m, ok := e.(map[string]any); ok && m["skill"] == skill {
			v, _ := m["open_count"].(float64)
			return v
		}
	}
	return -1
}

func sumAdded(data any) float64 {
	rows, _ := data.([]any)
	var sum float64
	for _, e := range rows {
		if m, ok := e.(map[string]any); ok {
			v, _ := m["added"].(float64)
			sum += v
		}
	}
	return sum
}

func hasUSDBand(data any) bool {
	rows, _ := data.([]any)
	for _, e := range rows {
		if m, ok := e.(map[string]any); ok && m["currency"] == "USD" {
			if p50, _ := m["p50"].(float64); p50 > 0 {
				return true
			}
		}
	}
	return false
}

// TestInsightsRoleSkillsEndpoint covers the single-role read: naming both category and
// seniority narrows the ranking to one role and attaches that role's skill distribution.
//
// The seeds are chosen so the share's denominator is DISTINGUISHABLE from the role's open
// count — four open postings, only three of which carry a tagged skill — because those two
// numbers being equal is exactly the coincidence that would let a wrong denominator pass.
func TestInsightsRoleSkillsEndpoint(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE jobs, companies RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	// backend/senior: 4 open postings, 3 skill-bearing. go on all 3, docker on 2.
	seedInsightsHandlerJob(t, ctx, pool, q, "k1", "backend", "senior", []string{"de"}, []string{"go", "docker"}, 0)
	seedInsightsHandlerJob(t, ctx, pool, q, "k2", "backend", "senior", []string{"de"}, []string{"go", "docker"}, 0)
	seedInsightsHandlerJob(t, ctx, pool, q, "k3", "backend", "senior", []string{"us"}, []string{"go"}, 0)
	// k4 is the posting with NO tagged skill: open, in the role, and absent from the
	// denominator. skills is NOT NULL, so an empty array rather than nil.
	seedInsightsHandlerJob(t, ctx, pool, q, "k4", "backend", "senior", []string{"us"}, []string{}, 0)
	// qa/lead: one posting, so its only skill falls below the floor of 2.
	seedInsightsHandlerJob(t, ctx, pool, q, "k5", "qa", "lead", []string{"de"}, []string{"go"}, 0)
	rebuildInsightsForTest(t, ctx, q)

	h := &statsHandlers{queries: q}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/insights/roles", h.InsightsRoles)

	get := func(t *testing.T, path string) (map[string]any, string, int) {
		t.Helper()
		return getInsights(t, ctx, app, path)
	}
	// only returns the single role a body carries, failing if it carries any other count.
	only := func(t *testing.T, out map[string]any, raw string) map[string]any {
		t.Helper()
		data, _ := out["data"].([]any)
		if len(data) != 1 {
			t.Fatalf("data has %d roles, want exactly 1: %s", len(data), raw)
		}
		role, _ := data[0].(map[string]any)
		return role
	}

	// --- the distribution, and the denominator it divides by ---------------------
	out, raw, code := get(t, "/api/v1/insights/roles?category=backend&seniority=senior")
	if code != fiber.StatusOK {
		t.Fatalf("single role: status %d, want 200: %s", code, raw)
	}
	role := only(t, out, raw)
	if got := role["open_count"]; got != float64(4) {
		t.Errorf("open_count = %v, want 4: %s", got, raw)
	}
	if got := role["sample_size"]; got != float64(3) {
		t.Errorf("sample_size = %v, want 3 (k4 carries no skill): %s", got, raw)
	}
	skills, _ := role["skills"].([]any)
	if len(skills) != 2 {
		t.Fatalf("skills = %v, want go and docker: %s", skills, raw)
	}
	first, _ := skills[0].(map[string]any)
	if first["skill"] != "go" || first["open_count"] != float64(3) {
		t.Errorf("first skill = %v, want go/3: %s", first, raw)
	}
	// 3/3, not 3/4. If the denominator were open_count this would read 0.75, which is
	// the whole reason sample_size is a separate number.
	if got := first["share"]; got != float64(1) {
		t.Errorf("go share = %v, want 1 (3/3, not 3/4): %s", got, raw)
	}
	second, _ := skills[1].(map[string]any)
	if got, _ := second["share"].(float64); math.Abs(got-2.0/3.0) > 1e-9 {
		t.Errorf("docker share = %v, want 2/3: %s", got, raw)
	}

	// --- a role whose every skill fell below the floor ---------------------------
	out, raw, code = get(t, "/api/v1/insights/roles?category=qa&seniority=lead")
	if code != fiber.StatusOK {
		t.Fatalf("floored role: status %d, want 200 (never 404): %s", code, raw)
	}
	role = only(t, out, raw)
	got, ok := role["skills"].([]any)
	if !ok || len(got) != 0 {
		t.Errorf("floored role skills = %v, want an EMPTY array that is still present: %s", role["skills"], raw)
	}

	// --- country scopes the counts, never the distribution -----------------------
	out, raw, code = get(t, "/api/v1/insights/roles?category=backend&seniority=senior&country=de")
	if code != fiber.StatusOK {
		t.Fatalf("country-scoped role: status %d, want 200: %s", code, raw)
	}
	role = only(t, out, raw)
	if got := role["open_count"]; got != float64(2) {
		t.Errorf("DE open_count = %v, want 2: %s", got, raw)
	}
	if got := role["sample_size"]; got != float64(3) {
		t.Errorf("DE sample_size = %v, want the country-agnostic 3: %s", got, raw)
	}
	meta, _ := out["meta"].(map[string]any)
	if meta["skills_geography_scoped"] != false {
		t.Errorf("meta must state the distribution is not geography-scoped: %s", raw)
	}

	// --- the ranked answer carries no distribution at all ------------------------
	out, raw, code = get(t, "/api/v1/insights/roles")
	if code != fiber.StatusOK {
		t.Fatalf("ranked roles: status %d, want 200: %s", code, raw)
	}
	for _, e := range out["data"].([]any) {
		entry, _ := e.(map[string]any)
		if _, present := entry["skills"]; present {
			t.Errorf("ranked answer carried a skills array: %s", raw)
		}
		if _, present := entry["sample_size"]; present {
			t.Errorf("ranked answer carried a sample_size: %s", raw)
		}
	}

	// --- seniority is READ, so it narrows rather than being silently dropped -----
	out, raw, code = get(t, "/api/v1/insights/roles?category=backend")
	if code != fiber.StatusOK {
		t.Fatalf("category-only: status %d, want 200: %s", code, raw)
	}
	if n := len(out["data"].([]any)); n != 1 {
		t.Fatalf("backend has %d seniorities seeded, expected 1 for this comparison: %s", n, raw)
	}

	// --- refusals ----------------------------------------------------------------
	if _, raw, code := get(t, "/api/v1/insights/roles?seniority=senior"); code != fiber.StatusBadRequest {
		t.Errorf("seniority without category: status %d, want 400: %s", code, raw)
	}
	if _, raw, code := get(t, "/api/v1/insights/roles?category=backend&seniority=archmage"); code != fiber.StatusBadRequest {
		t.Errorf("unknown seniority: status %d, want 400: %s", code, raw)
	}
}
