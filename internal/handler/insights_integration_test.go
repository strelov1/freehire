//go:build integration

// Integration test for the public Trends & Insights endpoints. The rollups are SQL
// over jobs and the handlers read through a concrete *db.Queries, so the wire
// contract — envelope shape, scoping, and (critically) that only aggregate data is
// exposed — can only be exercised against a real Postgres. It seeds jobs carrying
// distinctive record-level strings, recomputes the rollups, hits each route, and
// asserts the aggregates plus the absence of any leak.
// Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/db"
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
	mustExec(q.DeleteAllInsightsSalaryStats(ctx))
	_, err = q.RebuildInsightsSalaryStatsGlobal(ctx, 1) // min sample 1 so a small band survives
	mustExec(err)
	_, err = q.RebuildInsightsSalaryStatsByCountry(ctx, 1)
	mustExec(err)
	mustExec(q.DeleteAllInsightsVelocityDaily(ctx))
	_, err = q.RebuildInsightsVelocityDaily(ctx)
	mustExec(err)
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

	h := &API{pool: pool, queries: q}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/insights/roles", h.InsightsRoles)
	app.Get("/api/v1/insights/skills", h.InsightsSkills)
	app.Get("/api/v1/insights/velocity", h.InsightsVelocity)
	app.Get("/api/v1/insights/salary", h.InsightsSalary)

	get := func(t *testing.T, path string) (map[string]any, string, int) {
		t.Helper()
		resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, path, nil))
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
