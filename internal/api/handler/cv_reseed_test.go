package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// TestCVRegister_ReseedIsCookieOnly pins the gate against the real register().
// Cookie-only is the enforcement: whole-document replace must not be reachable with a
// CLI/API key the way PATCH is.
func TestCVRegister_ReseedIsCookieOnly(t *testing.T) {
	app := fiber.New()
	api := app.Group("/api/v1")
	(&cvHandlers{}).register(api, middleware{
		paidAI: markPaidAI,
		key:    namedGate("key"),
		cvKey:  namedGate("cvKey"),
		cookie: namedGate("cookie"),
	}, cvContract(&cvHandlers{}))
	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/me/cvs/"+uuid.New().String()+"/reseed", nil))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if got := string(body); got != "cookie" {
		t.Errorf("POST reseed gated by %q, want cookie", got)
	}
}
