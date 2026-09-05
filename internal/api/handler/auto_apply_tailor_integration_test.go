//go:build integration

// Integration tests for the auto-apply queue's own tailoring trigger and review-decision
// endpoints (openspec/changes/auto-apply-tailored-resume): ownership (a foreign entry is
// reported missing, never forbidden), the review-recorded refusal, a successful tailoring
// run recording the tailored CV, and the review decision's effect on
// autoapply.Store.Claim's own predicate.
// Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/api/ratelimit"
	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/cvedit"
	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/candidate/fitanalysis"
	"github.com/strelov1/freehire/internal/candidate/matchanalysis"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

// newAutoApplyTailorApp wires the assistant routes (which carry PostAutoApplyTailor/
// PostAutoApplyReview) plus a real plan.Store — newAssistantApp's own harness leaves plans
// nil, which is fine for the ordinary turn surface but panics the moment this endpoint's
// h.plans.StartSession runs, so this variant adds it to both assistantHandlers.plans and
// cvHandlers.plans (refuseNewTailoring's own field).
func newAutoApplyTailorApp(pool *pgxpool.Pool, iss *auth.Issuer, model assistant.Model) (*fiber.App, *assistantHandlers) {
	return newAutoApplyTailorAppWithPlanConfig(pool, iss, model, plan.DefaultConfig())
}

// newAutoApplyTailorAppWithPlanConfig wires the harness with NO auto-apply orchestrator
// secret configured — every request must authenticate as the entry's own owner (cookie or
// API key), exactly as before the shared-secret gate existed. Tests exercising the shared
// secret itself use newAutoApplyTailorAppWithOrchestratorSecret instead.
func newAutoApplyTailorAppWithPlanConfig(pool *pgxpool.Pool, iss *auth.Issuer, model assistant.Model, planCfg plan.Config) (*fiber.App, *assistantHandlers) {
	app, h, _ := newAutoApplyTailorAppFull(pool, iss, model, planCfg, "", nil)
	return app, h
}

// newAutoApplyTailorAppWithOrchestratorSecret wires the harness with a shared auto-apply
// orchestrator secret configured, so a request presenting it authenticates as the trusted
// orchestrator rather than as any entry's own owner — see resolveAutoApplyEntry and
// openspec/changes/auto-apply-inngest-orchestration/design.md.
func newAutoApplyTailorAppWithOrchestratorSecret(pool *pgxpool.Pool, iss *auth.Issuer, model assistant.Model, secret string, throttler ratelimit.Throttler) (*fiber.App, *assistantHandlers) {
	app, h, _ := newAutoApplyTailorAppFull(pool, iss, model, plan.DefaultConfig(), secret, throttler)
	return app, h
}

func newAutoApplyTailorAppFull(pool *pgxpool.Pool, iss *auth.Issuer, model assistant.Model, planCfg plan.Config, orchestratorSecret string, throttler ratelimit.Throttler) (*fiber.App, *assistantHandlers, middleware) {
	queries := db.New(pool)
	bank := experience.NewStore(experience.NewQueriesRepository(queries))
	plans := plan.NewStore(queries, pool, planCfg)
	store := assistant.NewStore(queries)
	cvH := &cvHandlers{
		cvStore: cv.NewStore(cv.NewQueriesRepository(queries)),
		editor:  cvedit.NewEditor(cvedit.NewRepository(pool, queries), bankGate{bank: bank}),
		queries: queries, jobReader: queries,
		fit:   fitanalysis.New(queries, nil, matchanalysis.NewAnalyzer(nil)),
		plans: plans,
		// PostAutoApplyTailor bootstraps a tailoring session the same way TailorCV does
		// (h.cv.startTailoringSession) — without this, that call 503s as "the assistant is
		// not available" before ever reaching the run.
		assistantSessions: store,
	}
	h := &assistantHandlers{
		store: store, queries: queries,
		maxPrompt:  defaultAssistantMaxPrompt,
		stages:     queries,
		experience: bank,
		fit:        fitanalysis.New(queries, nil, matchanalysis.NewAnalyzer(nil)),
		jobs:       queries,
		cv:         cvH,
		plans:      plans,
	}
	if model != nil {
		h.runner = assistant.NewRunner(model, h.store, assistant.RunnerConfig{MaxSteps: 3})
	}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	api := app.Group("/api/v1")
	keyAuth := auth.RequireAuthOrKey(iss, testVersions, apiKeys{queries})
	mw := middleware{
		cookie:        auth.RequireAuth(iss, testVersions),
		key:           keyAuth,
		autoApplyGate: autoApplyOrchestratorGate(orchestratorSecret, keyAuth, throttler),
	}
	h.register(api, mw)
	h.cv.register(api, mw)
	return app, h, mw
}

func truncateAutoApplyTailorTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		"TRUNCATE auto_apply_queue, cvs, users, jobs, assistant_sessions, user_notifications, usage_ledger, usage_daily RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

func insertAutoApplyJob(t *testing.T, pool *pgxpool.Pool, slug string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, company, public_slug)
		 VALUES ('greenhouse', $1, 'https://example.test/j/'||$1, 'Go Developer', 'Acme', $1)
		 RETURNING id`, slug).Scan(&id); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	return id
}

// insertBaseCV seeds a plain (non-tailored) base CV directly, bypassing the résumé/seeder
// path entirely — cv.Store.Tailor only reads the seeder when NO base CV exists yet.
func insertBaseCV(t *testing.T, pool *pgxpool.Pool, userID int64) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO cvs (user_id, title, template_id, data) VALUES ($1, 'Base', 'classic-ats', '{}'::jsonb)`,
		userID); err != nil {
		t.Fatalf("seed base cv: %v", err)
	}
}

func insertAutoApplyQueueRow(t *testing.T, pool *pgxpool.Pool, userID, jobID int64) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO auto_apply_queue (user_id, job_id) VALUES ($1, $2) RETURNING id`, userID, jobID).Scan(&id); err != nil {
		t.Fatalf("seed auto_apply_queue row: %v", err)
	}
	return id
}

func autoApplyTailorUser(t *testing.T, pool *pgxpool.Pool, iss *auth.Issuer, email string) (int64, string) {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	token, err := iss.Issue(id, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return id, token
}

func autoApplyRequest(t *testing.T, app *fiber.App, method, path, cookie string, body any) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(b)
	}
	req := httptest.NewRequestWithContext(context.Background(), method, path, reader)
	req.Header.Set(fiber.HeaderContentType, "application/json")
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	}
	resp, err := app.Test(req, 10_000)
	if err != nil {
		t.Fatalf("request %s %s: %v", method, path, err)
	}
	return resp
}

func TestPostAutoApplyTailor_ForeignEntryIsNotFound(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyTailorTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAutoApplyTailorApp(pool, iss, &turnModel{})

	owner, _ := autoApplyTailorUser(t, pool, iss, "owner@example.test")
	_, otherCookie := autoApplyTailorUser(t, pool, iss, "other@example.test")
	job := insertAutoApplyJob(t, pool, "tailor-foreign")
	queueID := insertAutoApplyQueueRow(t, pool, owner, job)

	resp := autoApplyRequest(t, app, fiber.MethodPost,
		"/api/v1/me/auto-apply/"+strconv.FormatInt(queueID, 10)+"/tailor", otherCookie, nil)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("status = %d, want 404 for a foreign queue entry", resp.StatusCode)
	}
}

func TestPostAutoApplyTailor_AlreadyReviewedEntryIsRefused(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyTailorTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAutoApplyTailorApp(pool, iss, &turnModel{})

	userID, cookie := autoApplyTailorUser(t, pool, iss, "reviewed@example.test")
	job := insertAutoApplyJob(t, pool, "tailor-reviewed")
	queueID := insertAutoApplyQueueRow(t, pool, userID, job)
	if _, err := pool.Exec(context.Background(),
		`UPDATE auto_apply_queue SET review_decision = 'declined', reviewed_at = now(), blocked_at = now() WHERE id = $1`,
		queueID); err != nil {
		t.Fatalf("mark reviewed: %v", err)
	}

	resp := autoApplyRequest(t, app, fiber.MethodPost,
		"/api/v1/me/auto-apply/"+strconv.FormatInt(queueID, 10)+"/tailor", cookie, nil)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("status = %d, want 409 for an already-reviewed entry", resp.StatusCode)
	}
}

// A caller with no tailor allowance left is refused before any CV or session is created —
// the same pre-check TailorCV runs (refuseNewTailoring), reused rather than reimplemented.
func TestPostAutoApplyTailor_SpentAllowanceRefusesBeforeAnyCVOrSession(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyTailorTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAutoApplyTailorAppWithPlanConfig(pool, iss, &turnModel{}, plan.DefaultConfig().Enforcing())

	userID, cookie := autoApplyTailorUser(t, pool, iss, "poor@example.test")
	insertBaseCV(t, pool, userID)
	job := insertAutoApplyJob(t, pool, "tailor-poor")
	queueID := insertAutoApplyQueueRow(t, pool, userID, job)

	// Spend today's whole tailoring allowance, mirroring TestTailorCVOutOfCredits' own setup:
	// seeding the counter directly is the state a candidate who already opened today's
	// sessions would be in.
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO usage_daily (user_id, feature, day, used) VALUES ($1, 'tailor', CURRENT_DATE, $2)`,
		userID, plan.DefaultConfig().FreeDaily(plan.FeatureTailor)); err != nil {
		t.Fatalf("seed a spent allowance: %v", err)
	}

	resp := autoApplyRequest(t, app, fiber.MethodPost,
		"/api/v1/me/auto-apply/"+strconv.FormatInt(queueID, 10)+"/tailor", cookie, nil)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusPaymentRequired {
		t.Fatalf("status = %d, want 402 for a spent tailor allowance", resp.StatusCode)
	}

	var tailoredCVCount int
	if err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM cvs WHERE user_id = $1 AND is_tailored", userID).Scan(&tailoredCVCount); err != nil {
		t.Fatal(err)
	}
	if tailoredCVCount != 0 {
		t.Errorf("tailored cvs created on 402 = %d, want 0", tailoredCVCount)
	}
}

func TestPostAutoApplyTailor_RunsAndRecordsTheTailoredCV(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyTailorTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	model := &turnModel{replies: []*llms.ContentChoice{{Content: "Walked the requirements."}}}
	app, _ := newAutoApplyTailorApp(pool, iss, model)

	userID, cookie := autoApplyTailorUser(t, pool, iss, "run@example.test")
	insertBaseCV(t, pool, userID)
	job := insertAutoApplyJob(t, pool, "tailor-run")
	queueID := insertAutoApplyQueueRow(t, pool, userID, job)

	resp := autoApplyRequest(t, app, fiber.MethodPost,
		"/api/v1/me/auto-apply/"+strconv.FormatInt(queueID, 10)+"/tailor", cookie, nil)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var out struct {
		Data autoApplyTailorResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Data.TailoredCVID == "" {
		t.Fatal("response carries no tailored_cv_id")
	}

	var stored uuid.UUID
	if err := pool.QueryRow(context.Background(),
		"SELECT tailored_cv_id FROM auto_apply_queue WHERE id = $1", queueID).Scan(&stored); err != nil {
		t.Fatalf("read stored tailored_cv_id: %v", err)
	}
	if stored.String() != out.Data.TailoredCVID {
		t.Errorf("stored tailored_cv_id = %s, want %s", stored, out.Data.TailoredCVID)
	}

	var notifCount int
	if err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM user_notifications WHERE user_id = $1 AND kind = 'auto_apply_tailor_ready'", userID).
		Scan(&notifCount); err != nil {
		t.Fatalf("count notifications: %v", err)
	}
	if notifCount != 1 {
		t.Errorf("notifications = %d, want 1", notifCount)
	}
}

// TestPostAutoApplyTailor_RetryAgainstAnAlreadyTailoredEntryDoesNotRenotify guards a
// wasteful re-run a code review found: nothing stopped a retried or resumed call (an
// Inngest retry after a client-side timeout, say) against an entry that already has a
// tailored CV from re-sending the "ready to review" notification the candidate already
// got. cv.Store.Tailor's own "one tailored copy per vacancy" idempotency means the CV
// itself, and the plan charge, are already safe — only the notification needed a guard.
func TestPostAutoApplyTailor_RetryAgainstAnAlreadyTailoredEntryDoesNotRenotify(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyTailorTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	model := &turnModel{replies: []*llms.ContentChoice{{Content: "Walked the requirements."}}}
	app, _ := newAutoApplyTailorApp(pool, iss, model)

	userID, cookie := autoApplyTailorUser(t, pool, iss, "retry@example.test")
	insertBaseCV(t, pool, userID)
	job := insertAutoApplyJob(t, pool, "tailor-retry")
	queueID := insertAutoApplyQueueRow(t, pool, userID, job)

	first := autoApplyRequest(t, app, fiber.MethodPost,
		"/api/v1/me/auto-apply/"+strconv.FormatInt(queueID, 10)+"/tailor", cookie, nil)
	defer first.Body.Close()
	if first.StatusCode != fiber.StatusOK {
		t.Fatalf("first status = %d, want 200", first.StatusCode)
	}

	second := autoApplyRequest(t, app, fiber.MethodPost,
		"/api/v1/me/auto-apply/"+strconv.FormatInt(queueID, 10)+"/tailor", cookie, nil)
	defer second.Body.Close()
	if second.StatusCode != fiber.StatusOK {
		t.Fatalf("second (retried) status = %d, want 200 — a retry against an unreviewed entry is not itself an error", second.StatusCode)
	}

	var notifCount int
	if err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM user_notifications WHERE user_id = $1 AND kind = 'auto_apply_tailor_ready'", userID).
		Scan(&notifCount); err != nil {
		t.Fatalf("count notifications: %v", err)
	}
	if notifCount != 1 {
		t.Errorf("notifications = %d, want 1 — the retry must not re-notify", notifCount)
	}
}

func TestPostAutoApplyReview_ForeignEntryIsNotFound(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyTailorTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAutoApplyTailorApp(pool, iss, &turnModel{})

	owner, _ := autoApplyTailorUser(t, pool, iss, "rowner@example.test")
	_, otherCookie := autoApplyTailorUser(t, pool, iss, "rother@example.test")
	job := insertAutoApplyJob(t, pool, "review-foreign")
	queueID := insertAutoApplyQueueRow(t, pool, owner, job)

	resp := autoApplyRequest(t, app, fiber.MethodPost,
		"/api/v1/me/auto-apply/"+strconv.FormatInt(queueID, 10)+"/review", otherCookie,
		autoApplyReviewRequest{Decision: "approved"})
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("status = %d, want 404 for a foreign queue entry", resp.StatusCode)
	}
}

func TestPostAutoApplyReview_NoTailoredCVYetIsRefused(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyTailorTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAutoApplyTailorApp(pool, iss, &turnModel{})

	userID, cookie := autoApplyTailorUser(t, pool, iss, "notailored@example.test")
	job := insertAutoApplyJob(t, pool, "review-no-cv")
	queueID := insertAutoApplyQueueRow(t, pool, userID, job)

	resp := autoApplyRequest(t, app, fiber.MethodPost,
		"/api/v1/me/auto-apply/"+strconv.FormatInt(queueID, 10)+"/review", cookie,
		autoApplyReviewRequest{Decision: "approved"})
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("status = %d, want 409 for an entry with no tailored cv yet", resp.StatusCode)
	}
}

// setTailoredCV seeds a tailored CV and points the queue row at it directly — the review
// endpoint's own precondition, reached here without running a real tailoring pass.
func setTailoredCV(t *testing.T, pool *pgxpool.Pool, userID, jobID, queueID int64) uuid.UUID {
	t.Helper()
	var cvID uuid.UUID
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO cvs (user_id, title, template_id, data, job_id, is_tailored)
		 VALUES ($1, 'Tailored', 'classic-ats', '{}'::jsonb, $2, true) RETURNING id`,
		userID, jobID).Scan(&cvID); err != nil {
		t.Fatalf("seed tailored cv: %v", err)
	}
	if _, err := pool.Exec(context.Background(),
		"UPDATE auto_apply_queue SET tailored_cv_id = $1 WHERE id = $2", cvID, queueID); err != nil {
		t.Fatalf("attach tailored cv to queue row: %v", err)
	}
	return cvID
}

func TestPostAutoApplyReview_ApprovingMakesTheEntryClaimable(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyTailorTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAutoApplyTailorApp(pool, iss, &turnModel{})
	queries := db.New(pool)

	userID, cookie := autoApplyTailorUser(t, pool, iss, "approve@example.test")
	job := insertAutoApplyJob(t, pool, "review-approve")
	queueID := insertAutoApplyQueueRow(t, pool, userID, job)
	setTailoredCV(t, pool, userID, job, queueID)

	resp := autoApplyRequest(t, app, fiber.MethodPost,
		"/api/v1/me/auto-apply/"+strconv.FormatInt(queueID, 10)+"/review", cookie,
		autoApplyReviewRequest{Decision: "approved"})
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	claimed, err := queries.ClaimAutoApplyBatch(context.Background(), db.ClaimAutoApplyBatchParams{LeaseSeconds: 3600, BatchSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 1 || claimed[0].ID != queueID {
		t.Fatalf("claimed = %+v, want the approved entry", claimed)
	}
}

func TestPostAutoApplyReview_DecliningParksAndExcludesFromClaim(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyTailorTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAutoApplyTailorApp(pool, iss, &turnModel{})
	queries := db.New(pool)

	userID, cookie := autoApplyTailorUser(t, pool, iss, "decline@example.test")
	job := insertAutoApplyJob(t, pool, "review-decline")
	queueID := insertAutoApplyQueueRow(t, pool, userID, job)
	setTailoredCV(t, pool, userID, job, queueID)

	resp := autoApplyRequest(t, app, fiber.MethodPost,
		"/api/v1/me/auto-apply/"+strconv.FormatInt(queueID, 10)+"/review", cookie,
		autoApplyReviewRequest{Decision: "declined"})
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var blockedAt any
	var lastError string
	if err := pool.QueryRow(context.Background(),
		"SELECT blocked_at, last_error FROM auto_apply_queue WHERE id = $1", queueID).
		Scan(&blockedAt, &lastError); err != nil {
		t.Fatal(err)
	}
	if blockedAt == nil {
		t.Error("blocked_at not set on decline")
	}
	if lastError != autoApplyDeclineReason {
		t.Errorf("last_error = %q, want the distinct decline reason", lastError)
	}

	claimed, err := queries.ClaimAutoApplyBatch(context.Background(), db.ClaimAutoApplyBatchParams{LeaseSeconds: 3600, BatchSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 0 {
		t.Errorf("claimed = %+v, want none — a declined entry is never claimed", claimed)
	}
}

func TestPostAutoApplyReview_DecisionCannotBeRecordedTwice(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyTailorTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := newAutoApplyTailorApp(pool, iss, &turnModel{})

	userID, cookie := autoApplyTailorUser(t, pool, iss, "twice@example.test")
	job := insertAutoApplyJob(t, pool, "review-twice")
	queueID := insertAutoApplyQueueRow(t, pool, userID, job)
	setTailoredCV(t, pool, userID, job, queueID)

	first := autoApplyRequest(t, app, fiber.MethodPost,
		"/api/v1/me/auto-apply/"+strconv.FormatInt(queueID, 10)+"/review", cookie,
		autoApplyReviewRequest{Decision: "approved"})
	defer first.Body.Close()
	if first.StatusCode != fiber.StatusOK {
		t.Fatalf("first review status = %d, want 200", first.StatusCode)
	}

	second := autoApplyRequest(t, app, fiber.MethodPost,
		"/api/v1/me/auto-apply/"+strconv.FormatInt(queueID, 10)+"/review", cookie,
		autoApplyReviewRequest{Decision: "declined"})
	defer second.Body.Close()
	if second.StatusCode != fiber.StatusConflict {
		t.Fatalf("second review status = %d, want 409", second.StatusCode)
	}

	var decision string
	if err := pool.QueryRow(context.Background(),
		"SELECT review_decision FROM auto_apply_queue WHERE id = $1", queueID).Scan(&decision); err != nil {
		t.Fatal(err)
	}
	if decision != "approved" {
		t.Errorf("review_decision = %q, want the first decision to stand unchanged", decision)
	}
}
