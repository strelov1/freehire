package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// The suggestion endpoint takes the same gate as the rest of the session API. The
// extension reaches the assistant with a Bearer credential, and a cookie-only rule
// here would leave the strip missing in the side panel for no gain — the endpoint
// reads a conversation the caller already owns and spends one cheap model call.
func TestAssistantFollowUpsRoute_TakesTheSessionApisGate(t *testing.T) {
	app := fiber.New()
	api := app.Group("/api/v1")
	(&assistantHandlers{}).register(api, middleware{
		key:    namedGate("key"),
		cvKey:  namedGate("cvKey"),
		cookie: namedGate("cookie"),
	})

	resp, err := app.Test(httptest.NewRequest(http.MethodPost,
		"/api/v1/assistant/sessions/00000000-0000-0000-0000-000000000000/followups", nil))
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if got := string(body); got != "key" {
		t.Errorf("the follow-ups route is gated by %q, want %q", got, "key")
	}
}

// A GET is the one method every prefetcher, crawler and browser feels free to issue
// twice, and this one spends a model call. It is a POST, and nothing else.
func TestAssistantFollowUpsRoute_IsNotReachableByGet(t *testing.T) {
	app := fiber.New()
	api := app.Group("/api/v1")
	(&assistantHandlers{}).register(api, middleware{
		key:    namedGate("key"),
		cvKey:  namedGate("cvKey"),
		cookie: namedGate("cookie"),
	})

	resp, err := app.Test(httptest.NewRequest(http.MethodGet,
		"/api/v1/assistant/sessions/00000000-0000-0000-0000-000000000000/followups", nil))
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != fiber.StatusNotFound && resp.StatusCode != fiber.StatusMethodNotAllowed {
		t.Errorf("GET status = %d, want the route not to answer a GET", resp.StatusCode)
	}
}
