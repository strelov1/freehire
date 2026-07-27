package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

// protectedApp mounts a single route behind RequireAuth that echoes the user id
// the middleware resolved, so tests can assert both access control and that the
// identity propagates into the handler.
func protectedApp(iss *Issuer) *fiber.App {
	app := fiber.New()
	app.Get("/me", RequireAuth(iss, anyVersion{1}), func(c *fiber.Ctx) error {
		id, ok := UserID(c)
		if !ok {
			return fiber.NewError(fiber.StatusInternalServerError, "user id missing from context")
		}
		return c.JSON(fiber.Map{"id": id})
	})
	return app
}

func TestRequireAuth_ValidTokenGrantsAccessAndPropagatesID(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	token, err := iss.Issue(7, 1)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	req := httptest.NewRequest(fiber.MethodGet, "/me", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: token})

	resp, err := protectedApp(iss).Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.ID != 7 {
		t.Errorf("handler saw user id %d, want 7", body.ID)
	}
}

func TestRequireAuth_RejectsUnauthorized(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	expired := NewIssuer("secret", -time.Minute)
	expiredToken, _ := expired.Issue(7, 1)

	cases := []struct {
		name  string
		token string // empty = no cookie set
	}{
		{"missing cookie", ""},
		{"malformed token", "not-a-jwt"},
		{"expired token", expiredToken},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(fiber.MethodGet, "/me", nil)
			if tc.token != "" {
				req.AddCookie(&http.Cookie{Name: CookieName, Value: tc.token})
			}
			resp, err := protectedApp(iss).Test(req)
			if err != nil {
				t.Fatalf("Test: %v", err)
			}
			if resp.StatusCode != fiber.StatusUnauthorized {
				t.Errorf("status = %d, want 401", resp.StatusCode)
			}
		})
	}
}
