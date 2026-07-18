//go:build integration

// Integration test for the public company hiring-signal leaderboard endpoint
// (GET /api/v1/insights/companies). It seeds companies with known ramping/freezing
// shapes, rebuilds the growth scalar, and asserts ordering, the min_open filter, the
// display-name join, and input validation. Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
)

func TestInsightsCompaniesEndpoint(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE jobs, companies RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	// ramp: 5 open jobs opened 3d ago → open_now 5, open 30d ago 0, growth +5.
	// freeze: 6 opened 60d ago, 4 closed 2d ago → open_now 2, open 30d ago 6, growth -4.
	// small: 1 open job (open_now 1) — excluded by min_open>=2.
	mustExec := func(sql string) {
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	mustExec(`INSERT INTO companies (slug, name) VALUES ('ramp','Ramp Inc'),('freeze','Freeze Co'),('small','Small LLC')`)
	mustExec(`INSERT INTO jobs (source,external_id,url,title,public_slug,company_slug,created_at)
	          SELECT 'test','rp-'||g,'http://x','J','rp-'||g,'ramp', now()-interval '3 days' FROM generate_series(1,5) g`)
	mustExec(`INSERT INTO jobs (source,external_id,url,title,public_slug,company_slug,created_at)
	          SELECT 'test','fz-o-'||g,'http://x','J','fz-o-'||g,'freeze', now()-interval '60 days' FROM generate_series(1,2) g`)
	mustExec(`INSERT INTO jobs (source,external_id,url,title,public_slug,company_slug,created_at,closed_at)
	          SELECT 'test','fz-c-'||g,'http://x','J','fz-c-'||g,'freeze', now()-interval '60 days', now()-interval '2 days' FROM generate_series(1,4) g`)
	mustExec(`INSERT INTO jobs (source,external_id,url,title,public_slug,company_slug,created_at)
	          VALUES ('test','sm-1','http://x','J','sm-1','small', now()-interval '3 days')`)

	prev := pgtype.Timestamptz{Time: time.Now().UTC().AddDate(0, 0, -30), Valid: true}
	if err := q.DeleteAllInsightsCompanyGrowth(ctx); err != nil {
		t.Fatalf("delete growth: %v", err)
	}
	if _, err := q.RebuildInsightsCompanyGrowth(ctx, prev); err != nil {
		t.Fatalf("rebuild growth: %v", err)
	}

	h := &API{pool: pool, queries: q}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/insights/companies", h.InsightsCompanies)

	get := func(path string) (map[string]any, int) {
		resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, path, nil))
		if err != nil {
			t.Fatalf("request %s: %v", path, err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		var out map[string]any
		if resp.StatusCode == fiber.StatusOK {
			if err := json.Unmarshal(raw, &out); err != nil {
				t.Fatalf("decode %s: %v (%s)", path, err, raw)
			}
		}
		return out, resp.StatusCode
	}

	rowsOf := func(out map[string]any) []map[string]any {
		data, _ := out["data"].([]any)
		res := make([]map[string]any, len(data))
		for i, r := range data {
			res[i], _ = r.(map[string]any)
		}
		return res
	}

	// sort=growth, min_open=2 → ramp (+5) before freeze (-4); small excluded.
	out, code := get("/api/v1/insights/companies?sort=growth&min_open=2")
	if code != fiber.StatusOK {
		t.Fatalf("growth: code %d", code)
	}
	rows := rowsOf(out)
	if len(rows) != 2 {
		t.Fatalf("growth rows = %d, want 2 (small filtered)", len(rows))
	}
	if rows[0]["company_slug"] != "ramp" || rows[0]["company_name"] != "Ramp Inc" {
		t.Errorf("top = %v/%v, want ramp/Ramp Inc", rows[0]["company_slug"], rows[0]["company_name"])
	}
	if rows[0]["open_now"].(float64) != 5 || rows[0]["open_prev_30d"].(float64) != 0 || rows[0]["growth_30d"].(float64) != 5 {
		t.Errorf("ramp row = %+v, want open_now=5 open_prev_30d=0 growth_30d=5", rows[0])
	}
	if rows[1]["company_slug"] != "freeze" {
		t.Errorf("second = %v, want freeze", rows[1]["company_slug"])
	}

	// sort=-growth → freeze (largest decline) first.
	out, _ = get("/api/v1/insights/companies?sort=-growth&min_open=2")
	rows = rowsOf(out)
	if rows[0]["company_slug"] != "freeze" || rows[0]["growth_30d"].(float64) != -4 {
		t.Errorf("freezing top = %+v, want freeze growth -4", rows[0])
	}

	// invalid sort → 400.
	if _, code := get("/api/v1/insights/companies?sort=bogus"); code != fiber.StatusBadRequest {
		t.Errorf("bogus sort code = %d, want 400", code)
	}
}
