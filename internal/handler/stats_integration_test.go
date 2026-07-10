//go:build integration

// Integration test for the job-activity rollup + read endpoint. The recompute is
// pure SQL (a full-table aggregate) and the handler reads through a concrete
// *db.Queries, so both can only be exercised against a real Postgres. It seeds
// jobs with controlled created_at/closed_at, runs the atomic rebuild, and asserts
// the materialized rows, the dense day/week series over the endpoint, and — the
// key correctness point — that reopening a job drops it from its old removed day.
// Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/db"
)

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 12, 0, 0, 0, time.UTC) // mid-day, to prove UTC-date bucketing
}

// rebuildRollup runs the worker's atomic rebuild (delete + recompute) against the
// pool, the same two queries cmd/rollup-stats runs in a transaction.
func rebuildRollup(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	q := db.New(pool)
	if err := q.DeleteAllJobDailyStats(ctx); err != nil {
		t.Fatalf("delete rollup: %v", err)
	}
	if _, err := q.RebuildJobDailyStats(ctx); err != nil {
		t.Fatalf("rebuild rollup: %v", err)
	}
}

func TestJobActivityRollupAndEndpoint(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	seed := func(ext string, created time.Time, closed *time.Time) {
		var closedArg any
		if closed != nil {
			closedArg = *closed
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO jobs (source, external_id, url, title, public_slug, created_at, closed_at)
			 VALUES ('test', $1, 'http://example.test', 'J', $1, $2, $3)`,
			ext, created, closedArg); err != nil {
			t.Fatalf("seed job %q: %v", ext, err)
		}
	}
	closedOn := func(tm time.Time) *time.Time { return &tm }

	// added days: 01-05(D), 01-10(A,B), 01-11(C); removed days: 01-06(D), 01-12(B,C).
	seed("A", day(2026, 1, 10), nil)
	seed("B", day(2026, 1, 10), closedOn(day(2026, 1, 12)))
	seed("C", day(2026, 1, 11), closedOn(day(2026, 1, 12)))
	seed("D", day(2026, 1, 5), closedOn(day(2026, 1, 6)))

	rebuildRollup(t, pool)

	// --- Materialized rows -----------------------------------------------------
	assertDay := func(d string, wantAdded, wantRemoved int) {
		t.Helper()
		var added, removed int
		err := pool.QueryRow(ctx,
			`SELECT added, removed FROM job_daily_stats WHERE day = $1`, d).Scan(&added, &removed)
		if err != nil {
			t.Fatalf("read rollup %s: %v", d, err)
		}
		if added != wantAdded || removed != wantRemoved {
			t.Errorf("%s: added=%d removed=%d, want added=%d removed=%d", d, added, removed, wantAdded, wantRemoved)
		}
	}
	assertDay("2026-01-05", 1, 0)
	assertDay("2026-01-06", 0, 1)
	assertDay("2026-01-10", 2, 0)
	assertDay("2026-01-11", 1, 0)
	assertDay("2026-01-12", 0, 2)

	// --- Endpoint: dense daily series -----------------------------------------
	h := &API{pool: pool, queries: db.New(pool)}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/stats/jobs-activity", h.JobsActivity)

	type point struct {
		Period  string `json:"period"`
		Added   int    `json:"added"`
		Removed int    `json:"removed"`
	}
	type envelope struct {
		Data []point `json:"data"`
		Meta struct {
			Granularity string `json:"granularity"`
			From        string `json:"from"`
			To          string `json:"to"`
		} `json:"meta"`
	}
	get := func(url string) envelope {
		t.Helper()
		resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, url, nil))
		if err != nil {
			t.Fatalf("request %q: %v", url, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, want 200 for %q", resp.StatusCode, url)
		}
		var env envelope
		if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return env
	}

	daily := get("/api/v1/stats/jobs-activity?granularity=day&from=2026-01-05&to=2026-01-12")
	if daily.Meta.Granularity != "day" || daily.Meta.From != "2026-01-05" || daily.Meta.To != "2026-01-12" {
		t.Errorf("meta = %+v, want day/2026-01-05/2026-01-12", daily.Meta)
	}
	if len(daily.Data) != 8 { // 05..12 inclusive, gap-filled
		t.Fatalf("daily points = %d, want 8 (dense, gap-filled)", len(daily.Data))
	}
	byDay := map[string]point{}
	for _, p := range daily.Data {
		byDay[p.Period] = p
	}
	if p := byDay["2026-01-08"]; p.Added != 0 || p.Removed != 0 {
		t.Errorf("gap day 2026-01-08 = %+v, want zeros", p)
	}
	if p := byDay["2026-01-12"]; p.Added != 0 || p.Removed != 2 {
		t.Errorf("2026-01-12 = %+v, want added=0 removed=2", p)
	}

	// --- Endpoint: weekly aggregation sums the days ---------------------------
	weekly := get("/api/v1/stats/jobs-activity?granularity=week&from=2026-01-05&to=2026-01-12")
	var wAdded, wRemoved int
	for _, p := range weekly.Data {
		wAdded += p.Added
		wRemoved += p.Removed
	}
	if wAdded != 4 || wRemoved != 3 { // totals: added A,B,C,D=4; removed B,C,D=3
		t.Errorf("weekly totals added=%d removed=%d, want 4/3", wAdded, wRemoved)
	}

	// --- Reopen drops the job from its old removed day ------------------------
	if _, err := pool.Exec(ctx, `UPDATE jobs SET closed_at = NULL WHERE external_id = 'D'`); err != nil {
		t.Fatalf("reopen D: %v", err)
	}
	rebuildRollup(t, pool)

	var n int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM job_daily_stats WHERE day = '2026-01-06'`).Scan(&n); err != nil {
		t.Fatalf("count 01-06: %v", err)
	}
	if n != 0 {
		t.Errorf("2026-01-06 still has %d row(s) after reopen, want 0 (orphan day dropped)", n)
	}
	// D's add-day survives (created_at is unchanged), so the rebuild is not just a wipe.
	assertDay("2026-01-05", 1, 0)
}
