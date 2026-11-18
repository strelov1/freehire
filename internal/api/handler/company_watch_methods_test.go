package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestCompanyWatch_ImportReservesItsMethodsBeforeTheSlugRoute(t *testing.T) {
	for _, method := range []string{fiber.MethodOptions, fiber.MethodGet, fiber.MethodHead, fiber.MethodPut, fiber.MethodDelete, fiber.MethodPatch} {
		t.Run(method, func(t *testing.T) {
			app := watchApp(&watchQueries{}, 7)
			request := httptest.NewRequestWithContext(context.Background(), method, "/api/v1/me/watchlist/import", strings.NewReader(`{"names":[]}`))
			request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
			response, err := app.Test(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != fiber.StatusMethodNotAllowed {
				t.Errorf("status=%d, want405", response.StatusCode)
			}
			if allow := response.Header.Get(fiber.HeaderAllow); allow != "POST" {
				t.Errorf("Allow=%q, want POST only", allow)
			}
			if method == fiber.MethodHead {
				body, err := io.ReadAll(response.Body)
				if err != nil {
					t.Fatal(err)
				}
				if len(body) != 0 {
					t.Errorf("HEAD body=%q, want empty", body)
				}
				return
			}
			var body map[string]string
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["error"] != "Method Not Allowed" {
				t.Errorf("error body=%v, want normal method refusal", body)
			}
		})
	}
}

func TestCompanyWatch_ImportPostKeepsItsCookieGate(t *testing.T) {
	status, body := doWatch(t, watchApp(&watchQueries{}, 0), fiber.MethodPost, "/me/watchlist/import", `{"names":["Acme"]}`)
	if status != fiber.StatusUnauthorized || body["error"] != "unauthorized" {
		t.Errorf("status=%d body=%v, want cookie refusal", status, body)
	}
}
