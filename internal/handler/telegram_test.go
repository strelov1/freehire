package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/telegramnotify"
)

// webhookApp mounts the webhook on an enabled handler with the given secret. No
// DB is needed: the secret guard runs before any query.
func webhookApp(secret string) *fiber.App {
	h := &telegramHandlers{
		telegramLinks:         telegramnotify.NewLinkTokens("test-secret", 10*time.Minute),
		telegramBot:           telegramnotify.NewClient("bottoken"),
		telegramWebhookSecret: secret,
	}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/api/v1/telegram/webhook", h.TelegramWebhook)
	return app
}

func TestTelegramWebhook_emptySecretFailsClosed(t *testing.T) {
	app := webhookApp("")

	// A request with no secret header must NOT pass when the configured secret is
	// empty — a naive ConstantTimeCompare("", "") would let a forged update in. The
	// endpoint is not served at all in that state, so the refusal is a 404: an
	// unsecurable webhook is unrepresentable, not merely guarded.
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/telegram/webhook", nil)
	res, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestTelegramWebhook_secretMismatchForbidden(t *testing.T) {
	app := webhookApp("hook-secret")

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/telegram/webhook", nil)
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "wrong")
	res, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
}
