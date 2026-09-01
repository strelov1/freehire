//go:build integration

// Integration tests for what a turn costs, against a real Postgres: a chat turn draws on
// the assistant allowance, a spent allowance is a 402 BEFORE the stream opens, a retry of
// an interrupted turn is not charged twice, a failed turn gives its allowance back, and a
// tailoring turn is bounded by its session's ceiling rather than by the assistant
// allowance. Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

// meteredAssistantApp is newAssistantApp with a plan store wired in, which is what makes
// a turn cost anything.
func meteredAssistantApp(t *testing.T, pool *pgxpool.Pool, iss *auth.Issuer, model assistant.Model, cfg plan.Config) *fiber.App {
	t.Helper()
	app, h := newAssistantApp(pool, iss, model)
	h.plans = plan.NewStore(db.New(pool), pool, cfg)
	return app
}

// postTurn sends one message to a session and returns the status and the body.
func postTurn(t *testing.T, app *fiber.App, sessionID, token, text string) (int, string) {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/assistant/sessions/"+sessionID+"/messages",
		strings.NewReader(`{"text":"`+text+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatalf("post turn: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

// usedToday reads the day's counter for a feature, or 0 when nothing was charged.
//
// The day is spelled in UTC rather than as CURRENT_DATE, which is the DATABASE's date: the
// counter is keyed by the UTC day, so a server on any other zone would look up a row the
// code never wrote — a failure that only appears for a few hours around midnight.
func usedToday(t *testing.T, pool *pgxpool.Pool, userID int64, feature plan.Feature) int {
	t.Helper()
	var n int
	err := pool.QueryRow(context.Background(),
		`SELECT used FROM usage_daily WHERE user_id=$1 AND feature=$2 AND day=(now() AT TIME ZONE 'utc')::date`,
		userID, string(feature)).Scan(&n)
	if err != nil {
		return 0
	}
	return n
}

func TestAChatTurnDrawsOnTheAssistantAllowance(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	cfg := plan.DefaultConfig().Enforcing()
	app := meteredAssistantApp(t, pool, iss, &turnModel{replies: []*llms.ContentChoice{{Content: "hello"}}}, cfg)

	userID, token := assistantUser(t, pool, iss, "turn-charged@example.test", false)
	sessionID := createAssistantSession(t, app, token, assistant.PresetChat)

	if status, _ := postTurn(t, app, sessionID, token, "hi"); status != fiber.StatusOK {
		t.Fatalf("turn status = %d, want 200", status)
	}
	if got := usedToday(t, pool, userID, plan.FeatureAssistant); got != 1 {
		t.Errorf("assistant allowance used = %d, want 1 — the turn was free", got)
	}
}

func TestASpentAssistantAllowanceIs402BeforeTheStreamOpens(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	// One message a day, so the second is refused.
	cfg := plan.DefaultConfig().Enforcing().WithFreeDaily(plan.FeatureAssistant, 1)
	model := &turnModel{replies: []*llms.ContentChoice{{Content: "first"}, {Content: "second"}}}
	app := meteredAssistantApp(t, pool, iss, model, cfg)

	_, token := assistantUser(t, pool, iss, "turn-refused@example.test", false)
	sessionID := createAssistantSession(t, app, token, assistant.PresetChat)

	if status, _ := postTurn(t, app, sessionID, token, "one"); status != fiber.StatusOK {
		t.Fatalf("first turn status = %d, want 200", status)
	}

	status, body := postTurn(t, app, sessionID, token, "two")
	// A real status, not an error frame inside a 200: anything checking status codes —
	// including the SPA deciding whether to show an upgrade prompt — sees only this.
	if status != fiber.StatusPaymentRequired {
		t.Fatalf("second turn status = %d, want 402", status)
	}
	if strings.Contains(body, "event:") || strings.Contains(body, "data:") {
		t.Errorf("the refusal opened an event stream: %q", body)
	}
	if !strings.Contains(body, "allowance") {
		t.Errorf("the 402 body does not carry the allowance: %q", body)
	}
}

func TestATailoringTurnIsBoundedByItsSessionNotTheAssistantAllowance(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	// No assistant allowance at all, and a one-turn tailoring ceiling. If tailoring drew on
	// the assistant allowance the first turn would be refused; it must not be.
	cfg := plan.DefaultConfig().Enforcing().WithFreeDaily(plan.FeatureAssistant, 1)
	cfg.TailorTurnsPerSession = 1
	replies := []*llms.ContentChoice{{Content: "one"}, {Content: "two"}}
	app := meteredAssistantApp(t, pool, iss, &turnModel{replies: replies}, cfg)

	userID, token := assistantUser(t, pool, iss, "tailor-ceiling@example.test", false)
	// Created through the store, not through POST /assistant/sessions: that endpoint mints
	// only the presets that bind to nothing, and a tailoring session binds to a CV. The
	// tailoring bootstrap is what creates one in production, and this is the state it
	// leaves behind.
	sess, err := assistant.NewStore(db.New(pool)).CreateSession(context.Background(), userID, assistant.PresetTailor, nil, nil)
	if err != nil {
		t.Fatalf("create tailoring session: %v", err)
	}
	sessionID := sess.ID.String()

	// The session has to hold a charge, or it was never paid for and runs no turns at all.
	plans := plan.NewStore(db.New(pool), pool, cfg)
	if _, err := plans.StartSession(context.Background(), userID, sessionID); err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	if status, _ := postTurn(t, app, sessionID, token, "first"); status != fiber.StatusOK {
		t.Fatalf("first tailoring turn status = %d, want 200", status)
	}
	if got := usedToday(t, pool, userID, plan.FeatureAssistant); got != 0 {
		t.Errorf("a tailoring turn consumed %d of the assistant allowance, want 0", got)
	}

	// The ceiling is one turn, and it has been used.
	status, body := postTurn(t, app, sessionID, token, "second")
	if status != fiber.StatusPaymentRequired {
		t.Fatalf("second tailoring turn status = %d, want 402 at the ceiling", status)
	}
	if !strings.Contains(body, string(plan.FeatureTailor)) {
		t.Errorf("the refusal names the wrong feature: %q — it must send the candidate to their tailoring allowance", body)
	}
}

func TestAFailedTurnGivesItsAllowanceBack(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	cfg := plan.DefaultConfig().Enforcing()
	// A model that fails: the turn is charged before the first call and must be refunded.
	app := meteredAssistantApp(t, pool, iss, &failingModel{}, cfg)

	userID, token := assistantUser(t, pool, iss, "turn-failed@example.test", false)
	sessionID := createAssistantSession(t, app, token, assistant.PresetChat)

	if status, _ := postTurn(t, app, sessionID, token, "hi"); status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200 — the stream opens before the model is called", status)
	}
	// The release runs on a detached context inside the stream goroutine, so it lands
	// shortly after the response is complete.
	waitFor(t, func() bool { return usedToday(t, pool, userID, plan.FeatureAssistant) == 0 },
		"the failed turn kept its charge; a model fault is ours, not the candidate's")
}

// A turn is charged BEFORE the headers go out, which is the only place a refusal can still
// be a status — but that is also before the session's slot is claimed, and a session already
// running a turn with another queued behind it refuses the next one with a 409.
//
// That turn never reaches the runner, so it owes its allowance back. Without the release a
// candidate whose second tab was a moment too eager pays for a message the server declined
// to run, and nothing anywhere gives it back.
func TestATurnRefusedTheSessionSlotGivesItsAllowanceBack(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	model := newDisconnectModel(t)
	defer model.letGo()
	cfg := plan.DefaultConfig().Enforcing()
	app := meteredAssistantApp(t, pool, iss, model, cfg)

	userID, token := assistantUser(t, pool, iss, "turn-queue-full@example.test", true)
	sessionID := createSession(t, app, token)
	addr := serveOnSocket(t, app)

	// One turn running, held inside the model.
	running := startTurnInBackground(t, addr, sessionID, token)
	select {
	case <-model.started:
	case <-time.After(10 * time.Second):
		t.Fatal("the first turn never started")
	}

	// One queued behind it. Both of these are turns that will really run, so both keep
	// their charge.
	queued := dialTurn(t, addr, sessionID, token)
	defer func() { _ = queued.Close() }()
	awaitEvent(t, queued, "event: queued")
	charged := usedToday(t, pool, userID, plan.FeatureAssistant)

	// The third is refused the slot: one waiter is a courtesy, a queue a client can grow is
	// a way to hold the process open.
	resp := assistantRequest(t, app, fiber.MethodPost, "/api/v1/assistant/sessions/"+sessionID+"/messages", token,
		map[string]string{"text": "and another thing"})
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("third message: status %d, want 409", resp.StatusCode)
	}

	// The release runs on a detached context, so it lands shortly after the 409.
	// Nothing moves: the refused turn took no allowance of its own (Consume found the
	// reference already paid for by the turn queued ahead of it), so it has none to give
	// back — and it must not give back that one's.
	time.Sleep(2 * time.Second)
	if got := usedToday(t, pool, userID, plan.FeatureAssistant); got != charged {
		t.Errorf("the 409 moved the counter from %d to %d; it neither pays for a turn the server declined to run nor refunds the turn still waiting to", charged, got)
	}

	model.letGo()
	<-running
}

func TestARetriedTurnIsNotChargedTwice(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	cfg := plan.DefaultConfig().Enforcing()
	app := meteredAssistantApp(t, pool, iss, &turnModel{replies: []*llms.ContentChoice{{Content: "answer"}}}, cfg)

	userID, token := assistantUser(t, pool, iss, "turn-retried@example.test", false)
	sessionID := createAssistantSession(t, app, token, assistant.PresetChat)

	if status, _ := postTurn(t, app, sessionID, token, "hi"); status != fiber.StatusOK {
		t.Fatalf("first turn status = %d, want 200", status)
	}
	before := usedToday(t, pool, userID, plan.FeatureAssistant)

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/assistant/sessions/"+sessionID+"/retry", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	defer resp.Body.Close()

	if got := usedToday(t, pool, userID, plan.FeatureAssistant); got != before {
		t.Errorf("the retry consumed another allowance (%d → %d); resuming an interrupted turn is the same turn", before, got)
	}
}

// waitFor polls until cond holds or the deadline passes, for the detached cleanup a turn
// schedules after its stream is finished.
func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Error(msg)
}

// createAssistantSession opens a session of the given preset and returns its id.
func createAssistantSession(t *testing.T, app *fiber.App, token, preset string) string {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/assistant/sessions",
		strings.NewReader(`{"preset":"`+preset+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	defer resp.Body.Close()
	var body struct {
		Data struct {
			ID uuid.UUID `json:"id"`
		} `json:"data"`
	}
	decodeJSON(t, resp, &body)
	if body.Data.ID == uuid.Nil {
		t.Fatal("create session returned no id")
	}
	return body.Data.ID.String()
}
