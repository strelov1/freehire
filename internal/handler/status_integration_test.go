//go:build integration

// Integration test for the public ingest-status endpoint. The per-provider rollup
// is SQL over board_health and the handler reads through a concrete *db.Queries,
// so the wire contract (envelope shape, derived status, and — critically —
// sanitization) can only be exercised against a real Postgres. It asserts the
// empty-fleet case, then seeds boards in controlled states and checks the rollup,
// the derived per-provider and overall status, and that no raw error text or
// board identifier leaks into the public body.
// Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/db"
)

func TestIngestStatusEndpoint(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	h := &statsHandlers{queries: db.New(pool)}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/status", h.IngestStatus)

	type providerEntry struct {
		Provider      string  `json:"provider"`
		Kind          string  `json:"kind"`
		Status        string  `json:"status"`
		TotalBoards   int     `json:"total_boards"`
		HealthyBoards int     `json:"healthy_boards"`
		CooledBoards  int     `json:"cooled_boards"`
		LastRun       *string `json:"last_run"`
		LastSuccess   *string `json:"last_success"`
		IngestedTotal int     `json:"ingested_total"`
	}
	type statusData struct {
		Overall        string          `json:"overall"`
		GeneratedAt    string          `json:"generated_at"`
		LastJobAddedAt *string         `json:"last_job_added_at"`
		Providers      []providerEntry `json:"providers"`
	}
	type envelope struct {
		Data statusData `json:"data"`
	}

	get := func(t *testing.T) (envelope, string) {
		t.Helper()
		resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/api/v1/status", nil))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, want 200 (public, unauthenticated read)", resp.StatusCode)
		}
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return env, string(raw)
	}

	// --- Empty fleet: 200, operational, no providers, no jobs yet --------------
	if env, _ := get(t); env.Data.Overall != "operational" || len(env.Data.Providers) != 0 || env.Data.LastJobAddedAt != nil {
		t.Fatalf("empty fleet: overall=%q providers=%d last_job_added_at=%v, want operational/0/nil", env.Data.Overall, len(env.Data.Providers), env.Data.LastJobAddedAt)
	}

	// --- Seed boards in controlled states --------------------------------------
	// A healthy board: no failures, fresh success. board id + count control the rollup.
	healthy := func(provider, board string, ingested int) {
		if _, err := pool.Exec(ctx, `
			INSERT INTO board_health
			  (provider, board, consecutive_failures, last_success_at, last_ingested_count, last_run_at)
			VALUES ($1, $2, 0, now(), $3, now())`, provider, board, ingested); err != nil {
			t.Fatalf("seed healthy %s/%s: %v", provider, board, err)
		}
	}
	// A soft-failing board: erred a couple of times but below the cooldown threshold,
	// so it is still crawled every cycle — it counts as served (healthy), not sidelined.
	// Carries a distinctive error text and board id that must NOT surface in the public body.
	failing := func(provider, board string) {
		if _, err := pool.Exec(ctx, `
			INSERT INTO board_health
			  (provider, board, consecutive_failures, last_error, last_error_at, last_run_at)
			VALUES ($1, $2, 2, 'SECRET_ERROR_TEXT', now(), now())`, provider, board); err != nil {
			t.Fatalf("seed failing %s/%s: %v", provider, board, err)
		}
	}
	// A cooled board: failing and currently in an active cooldown window, so it is
	// sidelined — it counts toward cooled_boards and NOT toward healthy_boards.
	cooled := func(provider, board string) {
		if _, err := pool.Exec(ctx, `
			INSERT INTO board_health
			  (provider, board, consecutive_failures, last_error, last_error_at, cooldown_until, last_run_at)
			VALUES ($1, $2, 5, 'SECRET_ERROR_TEXT', now(), now() + interval '1 hour', now())`, provider, board); err != nil {
			t.Fatalf("seed cooled %s/%s: %v", provider, board, err)
		}
	}

	// greenhouse: 2/2 served, fresh → operational, ingested_total 100.
	healthy("greenhouse", "acme", 40)
	healthy("greenhouse", "globex", 60)
	// recruitee: 1 clean + 2 soft-failing (below the cooldown threshold, still crawled),
	// fresh success present → all 3 served, none sidelined → operational. This is the
	// regression for "a board that merely erred is not counted unhealthy".
	healthy("recruitee", "rc-clean", 7)
	for _, b := range []string{"secret-rc-1", "secret-rc-2"} {
		failing("recruitee", b)
	}
	// lever: 4/10 served (6 in active cooldown), fresh success present → degraded.
	for _, b := range []string{"lv1", "lv2", "lv3", "lv4"} {
		healthy("lever", b, 5)
	}
	for _, b := range []string{"secret-board-1", "secret-board-2", "secret-board-3", "secret-board-4", "secret-board-5", "secret-board-6"} {
		cooled("lever", b)
	}
	// workday: 5/5 in active cooldown, no success at all → down.
	for _, b := range []string{"secret-wd-1", "secret-wd-2", "secret-wd-3", "secret-wd-4", "secret-wd-5"} {
		cooled("workday", b)
	}
	// jobstash: an aggregator adapter — proves provider-kind classification wires
	// through from the sources registry into the DTO.
	healthy("jobstash", "", 500)

	// One open, public job with a known created_at, so last_job_added_at has
	// something to report.
	latestJobAddedAt := statusNow.Add(-5 * time.Minute)
	if _, err := pool.Exec(ctx, `
		INSERT INTO jobs (source, external_id, url, title, public_slug, created_at)
		VALUES ('test', 'status-latest', 'http://example.test', 'Job', 'status-latest-job', $1)`, latestJobAddedAt); err != nil {
		t.Fatalf("seed latest job: %v", err)
	}

	env, raw := get(t)

	if env.Data.LastJobAddedAt == nil || *env.Data.LastJobAddedAt != latestJobAddedAt.Format(time.RFC3339) {
		t.Errorf("last_job_added_at = %v, want %s", env.Data.LastJobAddedAt, latestJobAddedAt.Format(time.RFC3339))
	}

	// Overall folds every provider into one fleet rollup: 21 boards, 11 cooled → 10/21
	// served (~0.48) with a fresh success present → degraded. Note it is NOT down even
	// though workday is fully down — a single small down provider no longer reds a fleet
	// with a healthy majority (the whole point of the fleet-aggregate verdict).
	if env.Data.Overall != "degraded" {
		t.Errorf("overall = %q, want degraded (fleet aggregate, not worst-provider)", env.Data.Overall)
	}
	if env.Data.GeneratedAt == "" {
		t.Error("generated_at is empty")
	}

	byProvider := map[string]providerEntry{}
	for _, p := range env.Data.Providers {
		byProvider[p.Provider] = p
	}

	gh, ok := byProvider["greenhouse"]
	if !ok {
		t.Fatal("greenhouse missing from providers")
	}
	if gh.Status != "operational" || gh.TotalBoards != 2 || gh.HealthyBoards != 2 || gh.IngestedTotal != 100 {
		t.Errorf("greenhouse = %+v, want operational/2/2/100", gh)
	}
	// Provider-kind classification: a board-based ATS vs a many-company aggregator.
	if gh.Kind != "ats" {
		t.Errorf("greenhouse kind = %q, want ats", gh.Kind)
	}
	if js, ok := byProvider["jobstash"]; !ok {
		t.Fatal("jobstash missing from providers")
	} else if js.Kind != "aggregator" {
		t.Errorf("jobstash kind = %q, want aggregator", js.Kind)
	}

	// recruitee: soft-failing boards (below the cooldown threshold) count as served, so
	// a provider whose only blemish is a couple of transient errors reads operational.
	rc, ok := byProvider["recruitee"]
	if !ok {
		t.Fatal("recruitee missing from providers")
	}
	if rc.Status != "operational" || rc.TotalBoards != 3 || rc.HealthyBoards != 3 || rc.CooledBoards != 0 {
		t.Errorf("recruitee = %+v, want operational/3/3/0 (soft-fails count served)", rc)
	}

	lv, ok := byProvider["lever"]
	if !ok {
		t.Fatal("lever missing from providers")
	}
	if lv.Status != "degraded" || lv.TotalBoards != 10 || lv.HealthyBoards != 4 || lv.CooledBoards != 6 {
		t.Errorf("lever = %+v, want degraded/10/4/6", lv)
	}

	wd, ok := byProvider["workday"]
	if !ok {
		t.Fatal("workday missing from providers")
	}
	if wd.Status != "down" || wd.TotalBoards != 5 || wd.HealthyBoards != 0 || wd.CooledBoards != 5 {
		t.Errorf("workday = %+v, want down/5/0/5 (all cooled)", wd)
	}
	// A down provider never succeeded, so last_success serializes as null.
	if wd.LastSuccess != nil {
		t.Errorf("workday last_success = %v, want null (never succeeded)", *wd.LastSuccess)
	}
	// It was still crawled, so last_run is present.
	if wd.LastRun == nil {
		t.Error("workday last_run is null, want a timestamp (it was crawled)")
	}

	// --- Sanitization: no internal detail leaks --------------------------------
	for _, leak := range []string{"SECRET_ERROR_TEXT", "secret-board", "last_error"} {
		if strings.Contains(raw, leak) {
			t.Errorf("public body leaked %q:\n%s", leak, raw)
		}
	}
}
