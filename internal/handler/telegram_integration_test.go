//go:build integration

// Integration tests for the Telegram linking + webhook flow against a real
// Postgres: a signed-in user requests a deep link, the inbound /start webhook
// (secret-guarded) captures the chat_id, and the status/unlink endpoints reflect
// it. The bot client points at a stub server so no real Telegram call is made.
// Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/telegramnotify"
)

func TestTelegramLinkAndWebhook(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('tg@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	// Stub Bot API so the webhook's confirmation reply makes no real network call.
	var replies int
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		replies++
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer stub.Close()

	iss := auth.NewIssuer("test-secret", time.Hour)
	cookie, _ := iss.Issue(userID)
	queries := db.New(pool)
	h := &API{
		pool:                  pool,
		queries:               queries,
		issuer:                iss,
		telegramLinks:         telegramnotify.NewLinkTokens("test-secret", 10*time.Minute),
		telegramBot:           telegramnotify.NewClientWithBase("bottoken", stub.URL),
		telegramBotUsername:   "freehirebot",
		telegramWebhookSecret: "hook-secret",
	}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	guard := auth.RequireAuth(iss)
	app.Post("/api/v1/me/telegram/link", guard, h.LinkTelegram)
	app.Get("/api/v1/me/telegram", guard, h.TelegramLinkStatus)
	app.Delete("/api/v1/me/telegram", guard, h.UnlinkTelegram)
	app.Post("/api/v1/telegram/webhook", h.TelegramWebhook)

	authed := func(method, p, body string) *http.Request {
		var r io.Reader
		if body != "" {
			r = bytes.NewBufferString(body)
		}
		rq := httptest.NewRequest(method, p, r)
		if body != "" {
			rq.Header.Set("Content-Type", "application/json")
		}
		rq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		return rq
	}
	send := func(rq *http.Request) (*http.Response, map[string]any) {
		res, err := app.Test(rq, -1)
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		var env map[string]any
		b, _ := io.ReadAll(res.Body)
		if len(b) > 0 {
			_ = json.Unmarshal(b, &env)
		}
		return res, env
	}

	// Link issues a deep link; the token in it resolves back to the user.
	res, env := send(authed(http.MethodPost, "/api/v1/me/telegram/link", ""))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("link status = %d, want 200", res.StatusCode)
	}
	url := env["data"].(map[string]any)["url"].(string)
	if !strings.HasPrefix(url, "https://t.me/freehirebot?start=") {
		t.Fatalf("deep link = %q, want t.me/freehirebot?start=...", url)
	}
	token := strings.TrimPrefix(url, "https://t.me/freehirebot?start=")
	if uid, err := h.telegramLinks.Parse(token); err != nil || uid != userID {
		t.Fatalf("deep-link token resolves to %d (%v), want %d", uid, err, userID)
	}

	// Status before linking.
	_, env = send(authed(http.MethodGet, "/api/v1/me/telegram", ""))
	if d := env["data"].(map[string]any); d["enabled"] != true || d["linked"] != false {
		t.Errorf("status before link = %+v, want enabled/not-linked", d)
	}

	webhook := func(secret, body string) *http.Response {
		rq := httptest.NewRequest(http.MethodPost, "/api/v1/telegram/webhook", bytes.NewBufferString(body))
		rq.Header.Set("Content-Type", "application/json")
		if secret != "" {
			rq.Header.Set("X-Telegram-Bot-Api-Secret-Token", secret)
		}
		res, err := app.Test(rq, -1)
		if err != nil {
			t.Fatalf("webhook app.Test: %v", err)
		}
		return res
	}
	startBody := fmt.Sprintf(`{"message":{"chat":{"id":98765},"text":"/start %s"}}`, token)

	// Forged update without the secret → 403, nothing linked.
	if res := webhook("", startBody); res.StatusCode != http.StatusForbidden {
		t.Fatalf("webhook without secret = %d, want 403", res.StatusCode)
	}
	if _, err := queries.GetTelegramLink(ctx, userID); err == nil {
		t.Fatal("link created despite missing secret")
	}

	// Valid secret + /start <token> → 200, chat linked, bot replied.
	if res := webhook("hook-secret", startBody); res.StatusCode != http.StatusOK {
		t.Fatalf("webhook with secret = %d, want 200", res.StatusCode)
	}
	link, err := queries.GetTelegramLink(ctx, userID)
	if err != nil || link.ChatID != 98765 {
		t.Fatalf("after webhook: chat=%d err=%v, want 98765", link.ChatID, err)
	}
	if replies == 0 {
		t.Error("bot sent no confirmation reply")
	}

	// Status after linking.
	_, env = send(authed(http.MethodGet, "/api/v1/me/telegram", ""))
	if d := env["data"].(map[string]any); d["linked"] != true || int64(d["chat_id"].(float64)) != 98765 {
		t.Errorf("status after link = %+v, want linked/chat 98765", d)
	}

	// A bogus token is acknowledged (200) but links nothing new: the only link
	// stays the real one (chat 98765), and no extra row appears.
	if res := webhook("hook-secret", `{"message":{"chat":{"id":424242},"text":"/start garbage"}}`); res.StatusCode != http.StatusOK {
		t.Errorf("webhook bad token = %d, want 200", res.StatusCode)
	}
	var linkCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM telegram_links`).Scan(&linkCount); err != nil {
		t.Fatal(err)
	}
	if linkCount != 1 {
		t.Errorf("telegram_links rows = %d, want 1 (bogus token created none)", linkCount)
	}

	// Unlink → 204, status reflects it.
	if res, _ := send(authed(http.MethodDelete, "/api/v1/me/telegram", "")); res.StatusCode != http.StatusNoContent {
		t.Errorf("unlink status = %d, want 204", res.StatusCode)
	}
	_, env = send(authed(http.MethodGet, "/api/v1/me/telegram", ""))
	if env["data"].(map[string]any)["linked"] != false {
		t.Error("status after unlink still linked")
	}
}

func TestTelegramDisabledWhenUnconfigured(t *testing.T) {
	pool := startPostgres(t)
	var userID int64
	if err := pool.QueryRow(context.Background(), `INSERT INTO users (email) VALUES ('off@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	iss := auth.NewIssuer("test-secret", time.Hour)
	cookie, _ := iss.Issue(userID)
	// No telegram* fields set → feature disabled.
	h := &API{pool: pool, queries: db.New(pool), issuer: iss}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/api/v1/me/telegram/link", auth.RequireAuth(iss), h.LinkTelegram)
	app.Post("/api/v1/telegram/webhook", h.TelegramWebhook)

	rq := httptest.NewRequest(http.MethodPost, "/api/v1/me/telegram/link", nil)
	rq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	if res, _ := app.Test(rq, -1); res.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("link when disabled = %d, want 503", res.StatusCode)
	}
	rq = httptest.NewRequest(http.MethodPost, "/api/v1/telegram/webhook", bytes.NewBufferString(`{}`))
	rq.Header.Set("Content-Type", "application/json")
	if res, err := app.Test(rq, -1); err != nil || res.StatusCode != http.StatusNotFound {
		t.Errorf("webhook when disabled = %d (%v), want 404", res.StatusCode, err)
	}
}
