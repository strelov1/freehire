package auth

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestBearerToken(t *testing.T) {
	cases := []struct {
		name, header, want string
	}{
		{"standard", "Bearer fhk_abc", "fhk_abc"},
		{"case-insensitive scheme", "bearer fhk_abc", "fhk_abc"},
		{"surrounding whitespace is trimmed", "Bearer   fhk_abc ", "fhk_abc"},
		{"scheme with no token", "Bearer", ""},
		{"scheme and space only", "Bearer ", ""},
		{"wrong scheme", "Basic dXNlcjpwYXNz", ""},
		{"no header", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := fiber.New()
			var got string
			app.Get("/", func(c *fiber.Ctx) error {
				got = bearerToken(c)
				return c.SendStatus(fiber.StatusOK)
			})
			req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/", nil)
			if tc.header != "" {
				req.Header.Set(fiber.HeaderAuthorization, tc.header)
			}
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("Test: %v", err)
			}
			defer resp.Body.Close()
			if got != tc.want {
				t.Errorf("bearerToken(%q) = %q, want %q", tc.header, got, tc.want)
			}
		})
	}
}
