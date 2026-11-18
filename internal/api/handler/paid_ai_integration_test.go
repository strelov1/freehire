//go:build integration

// Integration tests for the paid-AI gate against a real Postgres and the real plan store:
// a free account is refused before any model call, a subscriber goes through, and the
// auto-apply orchestrator's run is gated by the queue entry's owner. Run with:
// go test -tags=integration ./internal/api/handler/ -run PaidAI
package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// gateCountingModel is an assistant model that records every call, so a refusal can be shown
// to have happened before the model was reached.
type gateCountingModel struct{ calls atomic.Int32 }

func (m *gateCountingModel) Chat(_ context.Context, _ []llms.MessageContent, _ []llms.Tool, s llm.ChatStream) (*llms.ContentChoice, error) {
	m.calls.Add(1)
	if s.OnText != nil {
		s.OnText("hello")
	}
	return &llms.ContentChoice{Content: "hello"}, nil
}

// realGateAssistantApp mounts the assistant routes behind the REAL paid gate, reading the
// plan from Postgres, rather than the markPaidAI stand-in the other assistant tests use.
func realGateAssistantApp(t *testing.T, pool *pgxpool.Pool, iss *auth.Issuer, model assistant.Model) *fiber.App {
	t.Helper()
	queries := db.New(pool)
	_, h := newAssistantApp(pool, iss, model)
	plans := plan.NewStore(queries, pool, plan.DefaultConfig())
	h.plans = plans
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	h.register(app.Group("/api/v1"), middleware{
		paidAI: paidAIGate(entitlementOf(plans)),
		cookie: auth.RequireAuth(iss, testVersions),
		key:    auth.RequireAuthOrKey(iss, testVersions, apiKeys{queries}),
	}, newContract(contractHandlers{assistantHandlers: h}))
	return app
}

func TestPaidAIAssistantTurnRefusesFreeBeforeTheStream(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	model := &gateCountingModel{}
	app := realGateAssistantApp(t, pool, iss, model)

	_, freeToken := assistantUser(t, pool, iss, "paid-gate-free@example.test", false)
	freeSession := createAssistantSession(t, app, freeToken, assistant.PresetChat)
	status, body := postTurn(t, app, freeSession, freeToken, "hello")
	if status != fiber.StatusPaymentRequired {
		t.Fatalf("free turn status = %d (%s), want 402", status, body)
	}
	if strings.Contains(body, "event:") {
		t.Errorf("the refusal went out inside a stream: %q", body)
	}
	if !strings.Contains(body, "upgrade_url") {
		t.Errorf("the refusal carries no upgrade link: %q", body)
	}
	if n := model.calls.Load(); n != 0 {
		t.Fatalf("the model was called %d times for a free account", n)
	}

	_, proToken := assistantSubscriber(t, pool, iss, "paid-gate-pro@example.test", false)
	proSession := createAssistantSession(t, app, proToken, assistant.PresetChat)
	if status, body := postTurn(t, app, proSession, proToken, "hello"); status != fiber.StatusOK || !strings.Contains(body, "event:") {
		t.Fatalf("subscriber turn status = %d (%s), want a 200 stream", status, body)
	}
	if model.calls.Load() == 0 {
		t.Error("a subscriber's turn never reached the model")
	}
}

// The fit stream sits behind the same gate: a free caller gets a 402 status, not an
// EventSource error frame inside a 200.
func TestPaidAIFitStreamRefusesFree(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	plans := plan.NewStore(queries, pool, plan.DefaultConfig())
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	(&matchHandlers{}).register(app.Group("/api/v1"), middleware{
		paidAI: paidAIGate(entitlementOf(plans)),
		key:    auth.RequireAuthOrKey(iss, testVersions, apiKeys{queries}),
	}, newContract(contractHandlers{}))

	userID, token := assistantUser(t, pool, iss, "paid-gate-fit@example.test", false)
	insertAutoApplyJob(t, pool, "any-job")
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/api/v1/jobs/any-job/fit/stream", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	// The SPA's EventSource shape: a same-origin cookie request stamps Fetch Metadata,
	// and the route refuses a cookie request without it. The 402 under test is the
	// paid-AI gate's answer, which runs before the handler's own refusal.
	req.Header.Set(headerSecFetchSite, secFetchSiteSameOrigin)
	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatalf("stream request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusPaymentRequired {
		t.Fatalf("free fit stream status = %d, want 402", resp.StatusCode)
	}
	if ct := resp.Header.Get(fiber.HeaderContentType); strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("the refusal went out as a stream (%s)", ct)
	}
	if got := usedToday(t, pool, userID, plan.FeatureFit); got != 0 {
		t.Errorf("a refused free caller was charged %d analyses", got)
	}

	paywallWrites.Wait()
	var feature, slug string
	if err := pool.QueryRow(context.Background(),
		`SELECT h.feature, j.public_slug FROM paywall_hits h JOIN jobs j ON j.id = h.job_id WHERE h.user_id = $1`,
		userID).Scan(&feature, &slug); err != nil {
		t.Fatalf("the refusal left no paywall hit naming its job: %v", err)
	}
	if feature != string(plan.FeatureFit) || slug != "any-job" {
		t.Errorf("paywall hit = %s on %s, want %s on any-job", feature, slug, plan.FeatureFit)
	}
}

// The orchestrator authenticates as the deployment, not as a candidate, so the gate is
// asked inside the handler for the entry's OWNER. A candidate who lapsed after queueing
// gets no unattended run.
func TestPaidAIAutoApplyTailorIsGatedByTheEntryOwner(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyTailorTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	model := &gateCountingModel{}
	app, _ := newAutoApplyTailorApp(pool, iss, model)

	userID, cookie := autoApplyTailorUser(t, pool, iss, "lapsed-owner@example.test")
	endPro(t, pool, userID)
	insertBaseCV(t, pool, userID)
	job := insertAutoApplyJob(t, pool, "tailor-lapsed")
	queueID := insertAutoApplyQueueRow(t, pool, userID, job)

	resp := autoApplyRequest(t, app, fiber.MethodPost,
		"/api/v1/me/auto-apply/"+strconv.FormatInt(queueID, 10)+"/tailor", cookie, nil)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusPaymentRequired {
		t.Fatalf("status = %d, want 402 for a free entry owner", resp.StatusCode)
	}
	if n := model.calls.Load(); n != 0 {
		t.Errorf("the model was called %d times for a free entry owner", n)
	}
	paywallWrites.Wait()
	var hits int
	if err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM paywall_hits WHERE user_id = $1", userID).Scan(&hits); err != nil {
		t.Fatalf("count paywall hits: %v", err)
	}
	if hits != 0 {
		t.Errorf("the orchestrator's refusal recorded %d paywall hits, want 0 — nobody pressed a button", hits)
	}
	var tailored *string
	if err := pool.QueryRow(context.Background(),
		"SELECT tailored_cv_id::text FROM auto_apply_queue WHERE id = $1", queueID).Scan(&tailored); err != nil {
		t.Fatalf("read queue entry: %v", err)
	}
	if tailored != nil {
		t.Errorf("a refused run recorded a tailored CV %s", *tailored)
	}
}
