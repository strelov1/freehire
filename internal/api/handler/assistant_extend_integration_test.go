//go:build integration

// Integration tests for extending a tailoring session: it buys another ceiling's worth of
// turns out of the day's tailoring allowance, is refused when that allowance is spent, and
// is offered only for a session that HAS a ceiling. Run with:
// go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

// startedTailorSession creates a tailoring session and pays for its first ceiling, which
// is the state the tailoring bootstrap leaves behind in production.
func startedTailorSession(t *testing.T, pool *pgxpool.Pool, cfg plan.Config, userID int64) string {
	t.Helper()
	sess, err := assistant.NewStore(db.New(pool)).CreateSession(
		context.Background(), userID, assistant.PresetTailor, nil, nil)
	if err != nil {
		t.Fatalf("create tailoring session: %v", err)
	}
	if _, err := plan.NewStore(db.New(pool), pool, cfg).StartSession(
		context.Background(), userID, sess.ID.String()); err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	return sess.ID.String()
}

func TestExtendingASessionBuysMoreTurns(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	cfg := plan.DefaultConfig().Enforcing()
	cfg.TailorTurnsPerSession = 1
	app := meteredAssistantApp(t, pool, iss,
		&turnModel{replies: []*llms.ContentChoice{{Content: "one"}, {Content: "two"}}}, cfg)

	userID, token := assistantUser(t, pool, iss, "session-extend-http@example.test", false)
	sessionID := startedTailorSession(t, pool, cfg, userID)

	// Spend the one turn this session's first ceiling allows, then meet the ceiling.
	if status, _ := postTurn(t, app, sessionID, token, "first"); status != fiber.StatusOK {
		t.Fatalf("first turn status = %d, want 200", status)
	}
	if status, _ := postTurn(t, app, sessionID, token, "second"); status != fiber.StatusPaymentRequired {
		t.Fatalf("second turn status = %d, want 402 at the ceiling", status)
	}

	if status := postExtend(t, app, sessionID, token); status != fiber.StatusOK {
		t.Fatalf("extend status = %d, want 200", status)
	}
	if got := usedToday(t, pool, userID, plan.FeatureTailor); got != 2 {
		t.Errorf("tailoring allowance used = %d, want 2 (the start plus the extension)", got)
	}
	if status, _ := postTurn(t, app, sessionID, token, "third"); status != fiber.StatusOK {
		t.Fatalf("turn after extending status = %d, want 200", status)
	}
}

func TestExtendingIsRefusedWithNoAllowanceLeft(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	// One session a day: starting this one spends it, so there is nothing to extend with.
	cfg := plan.DefaultConfig().Enforcing().WithFreeDaily(plan.FeatureTailor, 1)
	app := meteredAssistantApp(t, pool, iss, &turnModel{}, cfg)

	userID, token := assistantUser(t, pool, iss, "session-extend-broke-http@example.test", false)
	sessionID := startedTailorSession(t, pool, cfg, userID)

	if status := postExtend(t, app, sessionID, token); status != fiber.StatusPaymentRequired {
		t.Fatalf("extend status = %d, want 402", status)
	}
}

func TestOnlyATailoringSessionCanBeExtended(t *testing.T) {
	pool := startPostgres(t)
	iss := auth.NewIssuer("test-secret", time.Hour)
	app := meteredAssistantApp(t, pool, iss, &turnModel{}, plan.DefaultConfig().Enforcing())

	_, token := assistantUser(t, pool, iss, "session-extend-chat@example.test", false)
	sessionID := createAssistantSession(t, app, token, assistant.PresetChat)

	// A chat session is bounded by the daily assistant allowance, and a day cannot be
	// topped up — offering to extend one would sell something that does not exist.
	if status := postExtend(t, app, sessionID, token); status != fiber.StatusConflict {
		t.Errorf("extending a chat session = %d, want 409", status)
	}
}

// postExtend asks for another ceiling's worth of turns and returns the status.
func postExtend(t *testing.T, app *fiber.App, sessionID, token string) int {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost,
		"/api/v1/assistant/sessions/"+sessionID+"/extend", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatalf("extend: %v", err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}
