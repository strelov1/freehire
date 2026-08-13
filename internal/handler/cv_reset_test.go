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

// TestCVRegister_ResetFromResumeIsCookieOnly pins the gate against the real register().
// Cookie-only is the enforcement: whole-document replace must not be reachable with a
// CLI/API key the way PATCH is.
func TestCVRegister_ResetFromResumeIsCookieOnly(t *testing.T) {
	app := fiber.New()
	api := app.Group("/api/v1")
	(&cvHandlers{}).register(api, middleware{
		key:    namedGate("key"),
		cvKey:  namedGate("cvKey"),
		cookie: namedGate("cookie"),
	})
	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/me/cvs/"+uuid.New().String()+"/reset-from-resume", nil))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if got := string(body); got != "cookie" {
		t.Errorf("POST reset-from-resume gated by %q, want cookie", got)
	}
}
