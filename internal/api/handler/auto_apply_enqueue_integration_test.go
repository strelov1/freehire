//go:build integration

// Integration tests for the candidate-facing auto-apply trigger
// (openspec/changes/auto-apply-submit-trigger): the enqueue endpoint's eligibility gates,
// its dedup/decline-is-terminal contract, the event publish, and the job detail response's
// own auto-apply status overlay.
// Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

// newAutoApplyEnqueueApp wires GetJob (for the auto_apply_status overlay) and
// PostJobAutoApply behind the SAME auth gates the real routes use: optionalAuth for the
// read, cookie-only auth for the write. events (nilable) is the same
// autoApplyEventPublisher fake auto_apply_review_publish_integration_test.go already
// defines.
func newAutoApplyEnqueueApp(pool *pgxpool.Pool, iss *auth.Issuer, events autoApplyEventPublisher) (*fiber.App, *assistantHandlers) {
	queries := db.New(pool)
	_, h, _ := newAutoApplyTailorAppFull(pool, iss, &turnModel{}, plan.DefaultConfig(), "", nil)
	h.events = events

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	api := app.Group("/api/v1")
	optionalAuth := auth.OptionalAuth(iss, testVersions, apiKeys{queries})
	cookieAuth := auth.RequireAuth(iss, testVersions)
	api.Get("/jobs/:slug", optionalAuth, (&jobsHandlers{queries: queries}).GetJob)
	api.Post("/jobs/:slug/auto-apply", cookieAuth, h.PostJobAutoApply)
	return app, h
}

func truncateAutoApplyEnqueueTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		"TRUNCATE auto_apply_queue, cvs, users, jobs RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

// seedEnqueueJob inserts a job with the given source and slug — insertAutoApplyJob (same
// package) always hardcodes greenhouse, which most of these tests want, but the
// non-Greenhouse refusal test needs another source.
func seedEnqueueJob(t *testing.T, pool *pgxpool.Pool, source, slug string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, company, public_slug)
		 VALUES ($1, $2, 'https://example.test/j/'||$2, 'Go Developer', 'Acme', $2)
		 RETURNING id`, source, slug).Scan(&id); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	return id
}

// makePro puts an account on the pro plan. It writes pro_until_granted, not pro_until: since
// migration 0135 the plan column is derived from three sources and assigning it fails.
//
// Granted is the right source rather than a convenient one — this test is about what a plan
// ALLOWS, and no payment provider is involved in it.
func makePro(t *testing.T, pool *pgxpool.Pool, userID int64) {
	t.Helper()
	q := db.New(pool)
	if err := q.SetProUntilGranted(context.Background(), db.SetProUntilGrantedParams{
		ID: userID, Until: pgtype.Timestamptz{Time: time.Now().Add(24 * time.Hour), Valid: true},
	}); err != nil {
		t.Fatalf("make pro: %v", err)
	}
}

func enqueueRequest(t *testing.T, app *fiber.App, slug, cookie string) *http.Response {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/jobs/"+slug+"/auto-apply", nil)
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	}
	resp, err := app.Test(req, 10_000)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	return resp
}

func TestPostJobAutoApply_RefusesNonGreenhouseSource(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyEnqueueTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAutoApplyEnqueueApp(pool, iss, nil)

	userID, cookie := autoApplyTailorUser(t, pool, iss, "nongh@example.test")
	makePro(t, pool, userID)
	insertBaseCV(t, pool, userID)
	seedEnqueueJob(t, pool, "lever", "nongh-job")

	resp := enqueueRequest(t, app, "nongh-job", cookie)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a non-Greenhouse job", resp.StatusCode)
	}
}

func TestPostJobAutoApply_RefusesNonProCaller(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyEnqueueTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAutoApplyEnqueueApp(pool, iss, nil)

	userID, cookie := autoApplyTailorUser(t, pool, iss, "free@example.test")
	insertBaseCV(t, pool, userID)
	seedEnqueueJob(t, pool, "greenhouse", "free-job")

	resp := enqueueRequest(t, app, "free-job", cookie)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusPaymentRequired {
		t.Fatalf("status = %d, want 402 for a free-tier caller", resp.StatusCode)
	}
}

func TestPostJobAutoApply_RefusesNoBaseCV(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyEnqueueTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAutoApplyEnqueueApp(pool, iss, nil)

	userID, cookie := autoApplyTailorUser(t, pool, iss, "nocv@example.test")
	makePro(t, pool, userID)
	seedEnqueueJob(t, pool, "greenhouse", "nocv-job")

	resp := enqueueRequest(t, app, "nocv-job", cookie)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("status = %d, want 409 for no base CV", resp.StatusCode)
	}
}

func TestPostJobAutoApply_RejectsAPIKeyOnly(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyEnqueueTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAutoApplyEnqueueApp(pool, iss, nil)

	userID, _ := autoApplyTailorUser(t, pool, iss, "keyonly@example.test")
	makePro(t, pool, userID)
	insertBaseCV(t, pool, userID)
	seedEnqueueJob(t, pool, "greenhouse", "keyonly-job")

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/jobs/keyonly-job/auto-apply", nil)
	req.Header.Set("Authorization", "Bearer fhk_whatever")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 — this route is cookie-only", resp.StatusCode)
	}
}

func TestPostJobAutoApply_FreshRequestCreatesOneEntryAndPublishes(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyEnqueueTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	events := &fakeEventPublisher{}
	app, _ := newAutoApplyEnqueueApp(pool, iss, events)

	userID, cookie := autoApplyTailorUser(t, pool, iss, "fresh@example.test")
	makePro(t, pool, userID)
	insertBaseCV(t, pool, userID)
	seedEnqueueJob(t, pool, "greenhouse", "fresh-job")

	resp := enqueueRequest(t, app, "fresh-job", cookie)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var count int
	if err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM auto_apply_queue WHERE user_id = $1", userID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("auto_apply_queue rows = %d, want 1", count)
	}
	waitForPublishCalls(t, events, 1)
}

func TestPostJobAutoApply_RepeatRequestIsIdempotentAndDoesNotRepublish(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyEnqueueTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	events := &fakeEventPublisher{}
	app, _ := newAutoApplyEnqueueApp(pool, iss, events)

	userID, cookie := autoApplyTailorUser(t, pool, iss, "repeat@example.test")
	makePro(t, pool, userID)
	insertBaseCV(t, pool, userID)
	seedEnqueueJob(t, pool, "greenhouse", "repeat-job")

	first := enqueueRequest(t, app, "repeat-job", cookie)
	defer first.Body.Close()
	if first.StatusCode != fiber.StatusOK {
		t.Fatalf("first status = %d, want 200", first.StatusCode)
	}
	second := enqueueRequest(t, app, "repeat-job", cookie)
	defer second.Body.Close()
	if second.StatusCode != fiber.StatusOK {
		t.Fatalf("second status = %d, want 200 (idempotent)", second.StatusCode)
	}

	var count int
	if err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM auto_apply_queue WHERE user_id = $1", userID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("auto_apply_queue rows = %d, want 1 (no duplicate)", count)
	}
	waitForPublishCalls(t, events, 1)
}

func TestPostJobAutoApply_PublishFailureDoesNotChangeTheResponse(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyEnqueueTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	events := &fakeEventPublisher{err: errors.New("inngest event api unreachable")}
	app, _ := newAutoApplyEnqueueApp(pool, iss, events)

	userID, cookie := autoApplyTailorUser(t, pool, iss, "publishfail@example.test")
	makePro(t, pool, userID)
	insertBaseCV(t, pool, userID)
	seedEnqueueJob(t, pool, "greenhouse", "publishfail-job")

	resp := enqueueRequest(t, app, "publishfail-job", cookie)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200 — a publish failure must not surface as a request failure", resp.StatusCode)
	}

	var count int
	if err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM auto_apply_queue WHERE user_id = $1", userID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("auto_apply_queue rows = %d, want 1 — the row is committed regardless of the publish outcome", count)
	}
}

func TestPostJobAutoApply_DeclinedEntryIsRefusedPermanently(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyEnqueueTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAutoApplyEnqueueApp(pool, iss, nil)

	userID, cookie := autoApplyTailorUser(t, pool, iss, "declined@example.test")
	makePro(t, pool, userID)
	insertBaseCV(t, pool, userID)
	jobID := seedEnqueueJob(t, pool, "greenhouse", "declined-job")
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO auto_apply_queue (user_id, job_id, review_decision, reviewed_at, blocked_at, last_error)
		 VALUES ($1, $2, 'declined', now(), now(), 'candidate declined the tailored CV')`,
		userID, jobID); err != nil {
		t.Fatalf("seed declined entry: %v", err)
	}

	resp := enqueueRequest(t, app, "declined-job", cookie)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("status = %d, want 409 — a declined attempt must never be retried", resp.StatusCode)
	}

	var count int
	if err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM auto_apply_queue WHERE user_id = $1", userID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("auto_apply_queue rows = %d, want 1 (no second row)", count)
	}
}

// TestPostJobAutoApply_RefusesAlreadyApplied guards the duplicate-submission risk a code
// review found: cmd/auto-apply/store.go's Submit deletes the auto_apply_queue row in the
// same transaction it stamps user_jobs.applied_at, so once a real ATS submission
// completes, EnqueueAutoApply's own ON CONFLICT dedup sees no row to conflict with — a
// re-click would otherwise start a second, genuine application for the same job.
func TestPostJobAutoApply_RefusesAlreadyApplied(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyEnqueueTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAutoApplyEnqueueApp(pool, iss, nil)

	userID, cookie := autoApplyTailorUser(t, pool, iss, "applied@example.test")
	makePro(t, pool, userID)
	insertBaseCV(t, pool, userID)
	jobID := seedEnqueueJob(t, pool, "greenhouse", "applied-job")
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO applications (user_id, job_id, applied_at) VALUES ($1, $2, now())`,
		userID, jobID); err != nil {
		t.Fatalf("seed applications row: %v", err)
	}

	resp := enqueueRequest(t, app, "applied-job", cookie)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("status = %d, want 409 — already applied for real", resp.StatusCode)
	}

	var count int
	if err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM auto_apply_queue WHERE user_id = $1", userID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("auto_apply_queue rows = %d, want 0 — no new attempt after a real application", count)
	}
}

func decodeAutoApplyStatus(t *testing.T, resp *http.Response) *string {
	t.Helper()
	defer resp.Body.Close()
	var out struct {
		Data struct {
			AutoApplyStatus *string `json:"auto_apply_status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out.Data.AutoApplyStatus
}

func TestGetJob_AutoApplyStatusOverlay(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyEnqueueTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAutoApplyEnqueueApp(pool, iss, nil)

	userID, cookie := autoApplyTailorUser(t, pool, iss, "status@example.test")
	makePro(t, pool, userID)
	insertBaseCV(t, pool, userID)
	seedEnqueueJob(t, pool, "greenhouse", "status-job")

	get := func(slug, cookie string) *http.Response {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/jobs/"+slug, nil)
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		return resp
	}

	beforeResp := get("status-job", cookie)
	defer beforeResp.Body.Close()
	if got := decodeAutoApplyStatus(t, beforeResp); got != nil {
		t.Fatalf("status before any attempt = %v, want nil", got)
	}
	anonResp := get("status-job", "")
	defer anonResp.Body.Close()
	if got := decodeAutoApplyStatus(t, anonResp); got != nil {
		t.Fatalf("status for an anonymous caller = %v, want nil", got)
	}

	enqueueResp := enqueueRequest(t, app, "status-job", cookie)
	defer enqueueResp.Body.Close()
	if enqueueResp.StatusCode != fiber.StatusOK {
		t.Fatalf("enqueue status = %d, want 200", enqueueResp.StatusCode)
	}
	queuedResp := get("status-job", cookie)
	defer queuedResp.Body.Close()
	if got := decodeAutoApplyStatus(t, queuedResp); got == nil || *got != "queued" {
		t.Fatalf("status after enqueue = %v, want \"queued\"", got)
	}

	var queueID int64
	if err := pool.QueryRow(context.Background(),
		"SELECT id FROM auto_apply_queue WHERE user_id = $1", userID).Scan(&queueID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(),
		`UPDATE auto_apply_queue SET review_decision = 'declined', reviewed_at = now(), blocked_at = now(), last_error = 'x' WHERE id = $1`,
		queueID); err != nil {
		t.Fatalf("mark declined: %v", err)
	}
	declinedResp := get("status-job", cookie)
	defer declinedResp.Body.Close()
	if got := decodeAutoApplyStatus(t, declinedResp); got == nil || *got != "declined" {
		t.Fatalf("status after decline = %v, want \"declined\"", got)
	}
}

// TestGetJob_AutoApplyStatusOverlay_FailedEntry guards a gap a code review found:
// GetAutoApplyQueueEntryForJob used to select only id and review_decision, so a
// dead-lettered (failed_at) or parked (blocked_at) submission — both of which leave
// review_decision at 'approved', since only an approved entry is ever claimed — was
// indistinguishable from a healthy entry still in flight. The candidate would see
// "queued" forever with no way to learn the attempt died.
func TestGetJob_AutoApplyStatusOverlay_FailedEntry(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyEnqueueTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAutoApplyEnqueueApp(pool, iss, nil)

	userID, cookie := autoApplyTailorUser(t, pool, iss, "failed@example.test")
	makePro(t, pool, userID)
	insertBaseCV(t, pool, userID)
	jobID := seedEnqueueJob(t, pool, "greenhouse", "failed-job")

	var queueID int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO auto_apply_queue (user_id, job_id, review_decision, reviewed_at, failed_at, last_error, attempts)
		 VALUES ($1, $2, 'approved', now(), now(), 'boom', 3) RETURNING id`,
		userID, jobID).Scan(&queueID); err != nil {
		t.Fatalf("seed dead-lettered entry: %v", err)
	}

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/jobs/failed-job", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if got := decodeAutoApplyStatus(t, resp); got == nil || *got != autoApplyStatusFailed {
		t.Fatalf("status for a dead-lettered entry = %v, want %q", got, autoApplyStatusFailed)
	}

	repeat := enqueueRequest(t, app, "failed-job", cookie)
	defer repeat.Body.Close()
	if repeat.StatusCode != fiber.StatusConflict {
		t.Fatalf("repeat enqueue status = %d, want 409 — a dead-lettered attempt must not report a false queued success", repeat.StatusCode)
	}

	var count int
	if err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM auto_apply_queue WHERE user_id = $1", userID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("auto_apply_queue rows = %d, want 1 (no second row)", count)
	}
}
