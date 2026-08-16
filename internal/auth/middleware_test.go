package auth

import (
	"context"
	"encoding/json"
	"errors"
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
		return c.JSON(fiber.Map{"id": id, "via_cookie": ViaCookie(c)})
	})
	return app
}

func TestRequireAuth_ValidTokenGrantsAccessAndPropagatesID(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	token, err := iss.Issue(7, 1)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/me", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: token})

	resp, err := protectedApp(iss).Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
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

// The extension gates read_current_page on this flag (see the
// confine-browse-preset-to-extension change): ViaCookie must be true for the
// website's own carrier so the tool stays confined to the extension's Bearer JWT.
func TestRequireAuth_FlagsCookieAuth(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	token, _ := iss.Issue(7, 1)

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/me", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: token})

	resp, err := protectedApp(iss).Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	var body struct {
		ViaCookie bool `json:"via_cookie"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.ViaCookie {
		t.Error("ViaCookie should be true for RequireAuth's cookie path")
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
			req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/me", nil)
			if tc.token != "" {
				req.AddCookie(&http.Cookie{Name: CookieName, Value: tc.token})
			}
			resp, err := protectedApp(iss).Test(req)
			if err != nil {
				t.Fatalf("Test: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != fiber.StatusUnauthorized {
				t.Errorf("status = %d, want 401", resp.StatusCode)
			}
		})
	}
}

type errVersions struct{ err error }

func (e errVersions) GetUserTokenVersion(_ context.Context, _ int64) (int32, error) {
	return 0, e.err
}

func TestRequireAuth_DBInfraErrorReturns503(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	token, _ := iss.Issue(7, 1)

	app := fiber.New()
	app.Get("/me", RequireAuth(iss, errVersions{err: errors.New("db connection lost")}), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/me", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: token})

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 on DB infra error", resp.StatusCode)
	}
}

func TestOptionalCookieAuth_DBInfraErrorDegradesToGuest(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	token, _ := iss.Issue(7, 1)

	app := fiber.New()
	app.Get("/public", OptionalCookieAuth(iss, errVersions{err: errors.New("db connection lost")}), func(c *fiber.Ctx) error {
		_, ok := UserID(c)
		return c.JSON(fiber.Map{"authed": ok})
	})

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/public", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: token})

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body struct {
		Authed bool `json:"authed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Authed {
		t.Error("expected guest degradation (authed=false) on DB infra error for OptionalCookieAuth")
	}
}

func TestOptionalAuth_DBInfraErrorDegradesCookieToGuest(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	token, _ := iss.Issue(7, 1)

	app := fiber.New()
	app.Get("/feed", OptionalAuth(iss, errVersions{err: errors.New("db connection lost")}, fakeKeyAuth{}), func(c *fiber.Ctx) error {
		_, ok := UserID(c)
		return c.JSON(fiber.Map{"authed": ok})
	})

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/feed", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: token})

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body struct {
		Authed bool `json:"authed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Authed {
		t.Error("expected guest degradation (authed=false) on cookie DB infra error for OptionalAuth")
	}
}

func TestOptionalAuth_DBInfraErrorOnAPIKeyReturns503(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)

	app := fiber.New()
	app.Get("/feed", OptionalAuth(iss, anyVersion{1}, errKeyAuth{err: errors.New("db timeout")}), func(c *fiber.Ctx) error {
		_, ok := UserID(c)
		return c.JSON(fiber.Map{"authed": ok})
	})

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/feed", nil)
	req.Header.Set("Authorization", "Bearer fhk_somekey")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 on API key DB infra error for OptionalAuth", resp.StatusCode)
	}
}

func TestRequireAuthOrKey_DBInfraErrorOnCookieReturns503(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	token, _ := iss.Issue(7, 1)

	app := fiber.New()
	app.Get("/protected", RequireAuthOrKey(iss, errVersions{err: errors.New("db error")}, fakeKeyAuth{}), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/protected", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: token})

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 on cookie DB infra error for RequireAuthOrKey", resp.StatusCode)
	}
}

func TestRequireAuthWS_DBInfraErrorReturns503(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	token, _ := iss.Issue(7, 1)

	app := fiber.New()
	app.Get("/ws", RequireAuthWS(iss, errVersions{err: errors.New("db error")}, fakeKeyAuth{}), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/ws", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 on DB infra error for RequireAuthWS", resp.StatusCode)
	}
}

func TestRequireAuth_DBTimeout(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	token, _ := iss.Issue(7, 1)

	app := fiber.New()
	app.Get("/protected", RequireAuth(iss, errVersions{err: context.DeadlineExceeded}), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/protected", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: token})

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 on context timeout for RequireAuth", resp.StatusCode)
	}
}

type errRoleLoader struct {
	err error
}

func (e errRoleLoader) GetUserRole(_ context.Context, _ int64) (string, error) {
	return "", e.err
}

func TestRequireRole_DBDown(t *testing.T) {
	app := fiber.New()
	app.Get("/admin", func(c *fiber.Ctx) error {
		c.Locals(LocalsUserID, int64(42))
		return c.Next()
	}, RequireRole(errRoleLoader{err: errors.New("db outage")}, "admin"), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/admin", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 on role loader DB outage for RequireRole", resp.StatusCode)
	}
}
