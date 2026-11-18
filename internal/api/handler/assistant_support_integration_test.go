//go:build integration

// Integration tests for the support agent against a real Postgres: who may run it, what its
// tools read, and every bound on spend and on handovers. Run with:
// go test -tags=integration ./internal/api/handler/ -run Support
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

type recordingSender struct {
	mu       sync.Mutex
	sent     []emailnotify.Message
	failNext atomic.Bool
}

func (s *recordingSender) Send(_ context.Context, m emailnotify.Message) error {
	if s.failNext.CompareAndSwap(true, false) {
		return errors.New("transport down")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, m)
	return nil
}

func (s *recordingSender) messages() []emailnotify.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]emailnotify.Message(nil), s.sent...)
}

// supportApp mounts every assistant route, support's included, behind the REAL paid gate, so a
// free account meets exactly what production gives it.
func supportApp(t *testing.T, pool *pgxpool.Pool, iss *auth.Issuer, model assistant.Model, cfg plan.Config, sender emailnotify.Sender) (*fiber.App, *assistantHandlers) {
	t.Helper()
	queries := db.New(pool)
	_, h := newAssistantApp(pool, iss, model)
	plans := plan.NewStore(queries, pool, cfg)
	h.plans = plans
	h.support.pool = pool
	h.support.runner = assistant.NewRunner(model, h.store, assistant.RunnerConfig{MaxSteps: supportMaxSteps})
	if sender != nil {
		h.support.mail = supportMail{sender: sender, from: "freehire <noreply@example.test>", to: "support@example.test"}
	}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	mw := middleware{
		paidAI: paidAIGate(entitlementOf(plans)),
		cookie: auth.RequireAuth(iss, testVersions),
		key:    auth.RequireAuthOrKey(iss, testVersions, apiKeys{queries}),
	}
	h.register(app.Group("/api/v1"), mw, newContract(contractHandlers{assistantHandlers: h}))
	h.registerSupport(app.Group("/api/v1"), mw, newContract(contractHandlers{assistantHandlers: h}))
	return app, h
}

func supportPost(t *testing.T, app *fiber.App, path, token, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return resp.StatusCode, sb.String()
}

func newSupportSession(t *testing.T, app *fiber.App, token string) string {
	t.Helper()
	status, body := supportPost(t, app, "/api/v1/support/sessions", token, `{}`)
	if status != fiber.StatusCreated {
		t.Fatalf("create support session = %d (%s), want 201", status, body)
	}
	var out struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil || out.Data.ID == "" {
		t.Fatalf("support session body %q: %v", body, err)
	}
	return out.Data.ID
}

func sendSupport(t *testing.T, app *fiber.App, sessionID, token string) (int, string) {
	t.Helper()
	return supportPost(t, app, "/api/v1/support/sessions/"+sessionID+"/messages", token, `{"text":"hi"}`)
}

func grantPro(t *testing.T, pool *pgxpool.Pool, userID int64, until time.Time) {
	t.Helper()
	if err := db.New(pool).SetProUntilGranted(context.Background(), db.SetProUntilGrantedParams{
		ID: userID, Until: pgtype.Timestamptz{Time: until, Valid: true},
	}); err != nil {
		t.Fatalf("grant pro: %v", err)
	}
}

func TestSupportRunsForAFreeAccountAndChatDoesNot(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	model := &gateCountingModel{}
	app, _ := supportApp(t, pool, iss, model, plan.DefaultConfig().Enforcing(), nil)
	_, token := assistantUser(t, pool, iss, "support-free@example.test", false)

	id := newSupportSession(t, app, token)
	if status, body := sendSupport(t, app, id, token); status != fiber.StatusOK || !strings.Contains(body, "event:") {
		t.Fatalf("free support message = %d (%s), want a 200 stream", status, body)
	}

	chat := createAssistantSession(t, app, token, assistant.PresetChat)
	if status, _ := postTurn(t, app, chat, token, "hi"); status != fiber.StatusPaymentRequired {
		t.Errorf("free chat message = %d, want 402", status)
	}
}

func TestSupportSessionIsUnreachableThroughEveryGenericRoute(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	model := &gateCountingModel{}
	app, _ := supportApp(t, pool, iss, model, plan.DefaultConfig().Enforcing(), nil)

	_, free := assistantUser(t, pool, iss, "support-generic-free@example.test", false)
	_, pro := assistantSubscriber(t, pool, iss, "support-generic-pro@example.test", false)
	freeSession := newSupportSession(t, app, free)
	sessionID := newSupportSession(t, app, pro)

	// Paid-gated routes answer a free caller 402 before the handler; everything else, and
	// every route for a paying caller, finds no such session.
	gated := map[string]bool{"messages": true, "opening": true, "sync": true, "retry": true, "voice-token": true}
	for _, route := range []string{"messages", "opening", "sync", "retry", "voice-token", "voice-turns", "cancel", "extend"} {
		path := "/api/v1/assistant/sessions/" + sessionID + "/" + route
		if status, body := supportPost(t, app, path, pro, `{"text":"hi"}`); status != fiber.StatusNotFound {
			t.Errorf("pro POST %s = %d (%s), want 404", route, status, body)
		}
		want := fiber.StatusNotFound
		if gated[route] {
			want = fiber.StatusPaymentRequired
		}
		freePath := "/api/v1/assistant/sessions/" + freeSession + "/" + route
		if status, body := supportPost(t, app, freePath, free, `{"text":"hi"}`); status != want {
			t.Errorf("free POST %s = %d (%s), want %d", route, status, body, want)
		}
	}

	for _, method := range []string{fiber.MethodGet, fiber.MethodDelete} {
		req := httptest.NewRequestWithContext(context.Background(), method, "/api/v1/assistant/sessions/"+sessionID, nil)
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: pro})
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("%s: %v", method, err)
		}
		resp.Body.Close()
		if resp.StatusCode != fiber.StatusNotFound {
			t.Errorf("%s of a support session through the generic route = %d, want 404", method, resp.StatusCode)
		}
	}
	// Nor is it listed among the caller's chats, where opening it would only 404.
	listReq := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/api/v1/assistant/sessions", nil)
	listReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: pro})
	listResp, err := app.Test(listReq)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	listed, _ := io.ReadAll(listResp.Body)
	listResp.Body.Close()
	if listResp.StatusCode != fiber.StatusOK || strings.Contains(string(listed), sessionID) {
		t.Errorf("GET /assistant/sessions = %d and lists the support session: %s", listResp.StatusCode, listed)
	}

	if n := model.calls.Load(); n != 0 {
		t.Errorf("the model was called %d times through generic routes", n)
	}
}

func TestSupportRoutesRefuseAChatSession(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	model := &gateCountingModel{}
	app, _ := supportApp(t, pool, iss, model, plan.DefaultConfig().Enforcing(), nil)
	_, token := assistantSubscriber(t, pool, iss, "support-chatid@example.test", false)

	chat := createAssistantSession(t, app, token, assistant.PresetChat)
	if status, _ := sendSupport(t, app, chat, token); status != fiber.StatusNotFound {
		t.Errorf("support route on a chat session = %d, want 404", status)
	}
	if model.calls.Load() != 0 {
		t.Error("a chat session was driven through the free support route")
	}
}

func TestSupportAllowanceRefusesAfterNMessagesWithoutAnUpgradePrompt(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	model := &gateCountingModel{}
	cfg := plan.DefaultConfig().Enforcing()
	cfg.SupportMessagesPerDay = 2
	app, _ := supportApp(t, pool, iss, model, cfg, nil)
	userID, token := assistantUser(t, pool, iss, "support-allowance@example.test", false)

	for i := 0; i < 2; i++ {
		id := newSupportSession(t, app, token)
		if status, body := sendSupport(t, app, id, token); status != fiber.StatusOK {
			t.Fatalf("message %d = %d (%s), want 200", i+1, status, body)
		}
	}
	calls := model.calls.Load()
	id := newSupportSession(t, app, token)
	status, body := sendSupport(t, app, id, token)
	if status != fiber.StatusPaymentRequired {
		t.Fatalf("message past the allowance = %d (%s), want 402", status, body)
	}
	if strings.Contains(body, "upgrade_url") || !strings.Contains(body, "support_email") {
		t.Errorf("refusal body %q: must name the support address and carry no upgrade link", body)
	}
	if model.calls.Load() != calls {
		t.Error("the model was called for a refused message")
	}
	if got := usedToday(t, pool, userID, plan.FeatureSupport); got != 2 {
		t.Errorf("support used today = %d, want 2", got)
	}
}

func TestSupportSessionCeilingRefuses(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	model := &gateCountingModel{}
	cfg := plan.DefaultConfig().Enforcing()
	cfg.SupportTurnsPerSession = 1
	app, _ := supportApp(t, pool, iss, model, cfg, nil)
	_, token := assistantUser(t, pool, iss, "support-session-ceiling@example.test", false)

	id := newSupportSession(t, app, token)
	if status, body := sendSupport(t, app, id, token); status != fiber.StatusOK {
		t.Fatalf("first message = %d (%s), want 200", status, body)
	}
	if status, body := sendSupport(t, app, id, token); status != fiber.StatusTooManyRequests {
		t.Fatalf("second message in a one-message session = %d (%s), want 429", status, body)
	}
	other := newSupportSession(t, app, token)
	if status, body := sendSupport(t, app, other, token); status != fiber.StatusOK {
		t.Errorf("a fresh session after the ceiling = %d (%s), want 200", status, body)
	}
}

func TestSupportGlobalCeilingRefusesEveryAccount(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	model := &gateCountingModel{}
	cfg := plan.DefaultConfig().Enforcing()
	cfg.SupportMessagesGlobalPerDay = 1
	app, _ := supportApp(t, pool, iss, model, cfg, nil)
	_, first := assistantUser(t, pool, iss, "support-global-a@example.test", false)
	_, second := assistantUser(t, pool, iss, "support-global-b@example.test", false)

	if status, body := sendSupport(t, app, newSupportSession(t, app, first), first); status != fiber.StatusOK {
		t.Fatalf("first account = %d (%s), want 200", status, body)
	}
	calls := model.calls.Load()
	if status, body := sendSupport(t, app, newSupportSession(t, app, second), second); status != fiber.StatusTooManyRequests {
		t.Fatalf("second account past the day's budget = %d (%s), want 429", status, body)
	}
	if model.calls.Load() != calls {
		t.Error("the model was called past the global ceiling")
	}
}

func TestMyAccountReturnsOnlyTheCallersOwnData(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	iss := auth.NewIssuer("test-secret", time.Hour)
	_, h := supportApp(t, pool, iss, &gateCountingModel{}, plan.DefaultConfig().Enforcing(), nil)

	aID, _ := assistantUser(t, pool, iss, "my-account-a@example.test", false)
	bID, _ := assistantUser(t, pool, iss, "my-account-b@example.test", false)
	aUntil := time.Now().UTC().AddDate(0, 0, 11)
	bUntil := time.Now().UTC().AddDate(0, 0, 200)
	grantPro(t, pool, aID, aUntil)
	grantPro(t, pool, bID, bUntil)
	// A date apart from Pro's, so the assertion proves which column it came from.
	aAPIUntil := aUntil.AddDate(0, 0, 3)
	if _, err := pool.Exec(ctx, `UPDATE users SET api_until_stripe = $2 WHERE id = $1`, aID, aAPIUntil); err != nil {
		t.Fatalf("seed api plan: %v", err)
	}
	for _, row := range []struct {
		user int64
		used int
	}{{aID, 2}, {bID, 7}} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO usage_daily (user_id, feature, day, used) VALUES ($1, $2, (now() AT TIME ZONE 'utc')::date, $3)`,
			row.user, string(plan.FeatureFit), row.used); err != nil {
			t.Fatalf("seed usage_daily: %v", err)
		}
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO usage_ledger (user_id, feature, day, kind, delta, ref) VALUES
		 ($1, 'tailor', (now() AT TIME ZONE 'utc')::date, 'consume', 1, 'aaaaaaaa-0000-4000-8000-000000000001#1'),
		 ($2, 'dictation', (now() AT TIME ZONE 'utc')::date, 'consume', 1, 'b-ref'),
		 ($2, 'dictation', (now() AT TIME ZONE 'utc')::date, 'consume', 1, 'b-ref-2')`, aID, bID); err != nil {
		t.Fatalf("seed ledger: %v", err)
	}

	got, err := h.myAccountTool().Run(ctx, aID, nil)
	if err != nil {
		t.Fatalf("my_account: %v", err)
	}
	raw, _ := json.Marshal(got)
	out := string(raw)
	sum := got.(accountSummary)

	if sum.Plan != "pro" || sum.ValidUntil != aUntil.Format("2006-01-02") {
		t.Errorf("plan = %q valid until %q, want pro until %s", sum.Plan, sum.ValidUntil, aUntil.Format("2006-01-02"))
	}
	if sum.APIPlan != "api" || sum.APIPlanUntil != aAPIUntil.Format("2006-01-02") {
		t.Errorf("api plan = %q until %q, want api until %s", sum.APIPlan, sum.APIPlanUntil, aAPIUntil.Format("2006-01-02"))
	}
	if gotB, err := h.myAccountTool().Run(ctx, bID, nil); err != nil {
		t.Fatalf("my_account for b: %v", err)
	} else if b := gotB.(accountSummary); b.APIPlan != "" || b.APIPlanUntil != "" {
		t.Errorf("an account without an API plan reports %q until %q", b.APIPlan, b.APIPlanUntil)
	}
	if strings.Contains(out, bUntil.Format("2006-01-02")) {
		t.Errorf("the other account's expiry date is in the output: %s", out)
	}
	for _, a := range sum.Allowances {
		if a.Feature == string(plan.FeatureFit) && a.Used != 2 {
			t.Errorf("fit used = %d, want the caller's 2 and not the other account's 7", a.Used)
		}
	}
	if len(sum.Recent) != 1 {
		t.Errorf("recent activity has %d entries, want the caller's 1: %s", len(sum.Recent), out)
	}
	for _, leak := range []string{"my-account-a@example.test", "my-account-b@example.test", "@"} {
		if strings.Contains(out, leak) {
			t.Errorf("the output carries %q: %s", leak, out)
		}
	}
}

func seedSupportSession(t *testing.T, h *assistantHandlers, userID int64) uuid.UUID {
	t.Helper()
	sess, err := h.store.CreateSession(context.Background(), userID, assistant.PresetSupport, nil, nil)
	if err != nil {
		t.Fatalf("create support session: %v", err)
	}
	return sess.ID
}

func escalationInput(summary string) escalateInput {
	return escalateInput{Category: "billing", Summary: summary}
}

func TestSupportCreateHandsBackTheBlankSessionUntilAMessageIsSent(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := supportApp(t, pool, iss, &gateCountingModel{}, plan.DefaultConfig().Enforcing(), nil)
	_, token := assistantUser(t, pool, iss, "support-create-reuse@example.test", false)
	_, other := assistantUser(t, pool, iss, "support-create-reuse-other@example.test", false)

	first := newSupportSession(t, app, token)
	status, body := supportPost(t, app, "/api/v1/support/sessions", token, `{}`)
	if status != fiber.StatusOK || !strings.Contains(body, first) {
		t.Fatalf("second create = %d (%s), want 200 handing back %s", status, body, first)
	}
	if got := newSupportSession(t, app, other); got == first {
		t.Error("another account was handed this account's blank session")
	}

	if status, body := sendSupport(t, app, first, token); status != fiber.StatusOK {
		t.Fatalf("message = %d (%s), want 200", status, body)
	}
	if next := newSupportSession(t, app, token); next == first {
		t.Error("a session that already holds a message was handed back as blank")
	}
}

func TestMyAccountReportsTheSupportAllowanceFromTheLedger(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	iss := auth.NewIssuer("test-secret", time.Hour)
	cfg := plan.DefaultConfig().Enforcing()
	cfg.SupportMessagesPerDay = 7
	_, h := supportApp(t, pool, iss, &gateCountingModel{}, cfg, nil)
	aID, _ := assistantUser(t, pool, iss, "support-left-a@example.test", false)
	bID, _ := assistantUser(t, pool, iss, "support-left-b@example.test", false)

	read := func(user int64) supportAllowance {
		t.Helper()
		got, err := h.myAccountTool().Run(ctx, user, nil)
		if err != nil {
			t.Fatalf("my_account: %v", err)
		}
		return got.(accountSummary).SupportToday
	}
	if got := read(aID); got.Limit != 7 || got.Used != 0 || got.Remaining != 7 {
		t.Fatalf("fresh account = %+v, want limit 7, used 0, remaining 7", got)
	}
	for _, ref := range []string{"r1", "r2", "r3"} {
		if _, err := h.plans.ConsumeSupport(ctx, aID, ref); err != nil {
			t.Fatalf("ConsumeSupport: %v", err)
		}
	}
	if _, err := h.plans.ConsumeSupport(ctx, bID, "other"); err != nil {
		t.Fatal(err)
	}
	got := read(aID)
	if got.Used != 3 || got.Remaining != 4 {
		t.Errorf("after 3 messages = %+v, want used 3, remaining 4", got)
	}
	next := time.Now().UTC().Truncate(24 * time.Hour).Add(24 * time.Hour).Format(time.RFC3339)
	if got.ResetsAt != next {
		t.Errorf("resets_at = %q, want %q", got.ResetsAt, next)
	}
	if err := h.plans.Release(ctx, aID, plan.FeatureSupport, "r1"); err != nil {
		t.Fatal(err)
	}
	if got := read(aID); got.Used != 2 || got.Remaining != 5 {
		t.Errorf("after release = %+v, want used 2, remaining 5", got)
	}
}

func TestSupportRatingRecordsTheOwnersVerdictOnly(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app, _ := supportApp(t, pool, iss, &gateCountingModel{}, plan.DefaultConfig().Enforcing(), nil)
	_, owner := assistantSubscriber(t, pool, iss, "support-rate-owner@example.test", false)
	_, other := assistantSubscriber(t, pool, iss, "support-rate-other@example.test", false)
	id := newSupportSession(t, app, owner)
	rate := func(token, body string) int {
		status, _ := supportPost(t, app, "/api/v1/support/sessions/"+id+"/rating", token, body)
		return status
	}
	helpful := func() *bool {
		var v *bool
		if err := pool.QueryRow(context.Background(), `SELECT helpful FROM assistant_sessions WHERE id = $1`, id).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}

	if got := rate(owner, `{}`); got != fiber.StatusBadRequest {
		t.Errorf("rating without the field = %d, want 400", got)
	}
	if got := rate(other, `{"helpful":true}`); got != fiber.StatusNotFound {
		t.Errorf("another account's rating = %d, want 404", got)
	}
	if got := rate(owner, `{"helpful":true}`); got != fiber.StatusConflict {
		t.Errorf("rating before any reply = %d, want 409", got)
	}
	if v := helpful(); v != nil {
		t.Fatalf("helpful = %v after refused votes, want NULL", *v)
	}
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO assistant_messages (session_id, seq, role, content) VALUES ($1, 1, 'user', '{"text":"hi"}'), ($1, 2, 'assistant', '{"text":"hello"}')`, id); err != nil {
		t.Fatal(err)
	}
	if got := rate(owner, `{"helpful":true}`); got != fiber.StatusNoContent {
		t.Fatalf("owner rating = %d, want 204", got)
	}
	if got := rate(owner, `{"helpful":false}`); got != fiber.StatusNoContent {
		t.Fatalf("owner re-rating = %d, want 204", got)
	}
	if v := helpful(); v == nil || *v {
		t.Errorf("helpful = %v, want false (the later vote)", v)
	}

	chat := createAssistantSession(t, app, owner, assistant.PresetChat)
	if status, _ := supportPost(t, app, "/api/v1/support/sessions/"+chat+"/rating", owner, `{"helpful":true}`); status != fiber.StatusNotFound {
		t.Errorf("rating a chat session = %d, want 404", status)
	}
}
