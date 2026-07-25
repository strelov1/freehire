package handler

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/telegramnotify"
)

// The webhook's secret is compared in constant time against the configured value. With an
// unset secret both sides are empty, and an equality check on two empty strings succeeds —
// so a misconfigured deployment would accept forged Bot API updates from anyone. The
// endpoint must refuse the request rather than trust an empty expectation.
func TestTelegramWebhook_RefusesWhenTheSecretIsUnset(t *testing.T) {
	h := &API{
		telegramLinks:         telegramnotify.NewLinkTokens("jwt-secret", time.Minute),
		telegramBot:           telegramnotify.NewClient("bot-token"),
		telegramWebhookSecret: "", // misconfigured deployment
	}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/api/v1/telegram/webhook", h.TelegramWebhook)

	resp, err := app.Test(httptest.NewRequest(fiber.MethodPost, "/api/v1/telegram/webhook", nil))
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == fiber.StatusOK {
		t.Fatal("an unauthenticated update was accepted while the secret was unset (fail-open)")
	}
}
