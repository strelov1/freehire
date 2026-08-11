package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/strelov1/freehire/internal/accounts"
	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/auth/oauth"
)

func TestListProvidersV2StructuredAndOmitsWebApple(t *testing.T) {
	h := &authHandlers{oauth: fakeRegistry{"google": &fakeProvider{name: "google"}, "apple": &fakeProvider{name: "apple"}}, mobileCallbacks: map[string]string{"ios": "https://freehire.me/auth/callback"}}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v2/auth/providers", h.ListProvidersV2)
	resp := get(t, app, "/api/v2/auth/providers")
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var body struct {
		Data struct {
			Schema    int `json:"schema_version"`
			Providers []struct {
				ID        string   `json:"id"`
				Flow      string   `json:"flow"`
				Platforms []string `json:"platforms"`
				Available bool     `json:"available"`
			} `json:"providers"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Schema != 2 || len(body.Data.Providers) != 1 || body.Data.Providers[0].ID != "google" || body.Data.Providers[0].Flow != "browser_oauth" {
		t.Fatalf("body=%+v", body)
	}
}

func TestRegisterV2DisabledAndNoStoreHeaders(t *testing.T) {
	disabled := fiber.New()
	(&authHandlers{authV2Enabled: false}).registerV2(disabled.Group("/api/v2"), middleware{})
	resp, err := disabled.Test(httptest.NewRequest("GET", "/api/v2/auth/providers", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 404 {
		t.Fatalf("disabled status=%d", resp.StatusCode)
	}
	h := &authHandlers{authV2Enabled: true, oauth: fakeRegistry{}}
	enabled := fiber.New(fiber.Config{ErrorHandler: RenderError})
	h.registerV2(enabled.Group("/api/v2"), middleware{optionalCookie: func(c *fiber.Ctx) error { return c.Next() }})
	resp, err = enabled.Test(httptest.NewRequest("GET", "/api/v2/auth/providers", nil))
	if err != nil {
		t.Fatal(err)
	}
	if got := resp.Header.Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control=%q", got)
	}
}

func TestV2CodedErrorEnvelope(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/coded", func(*fiber.Ctx) error {
		return authError(428, "recent_auth_required", "recent authentication required")
	})
	resp, err := app.Test(httptest.NewRequest("GET", "/coded", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body map[string]string
	if err = json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 428 || body["code"] != "recent_auth_required" {
		t.Fatalf("status=%d body=%v", resp.StatusCode, body)
	}
}

func TestV2OAuthStateCookieHasSeparateNamespace(t *testing.T) {
	if oauthV2StateCookieName == oauth.StateCookieName {
		t.Fatal("v2 OAuth must not overwrite an in-flight v1 state cookie")
	}
}

func TestRecentAuthRotatesLegacySessionBeforeBinding(t *testing.T) {
	const secret = "legacy-session-test-secret"
	issuer := auth.NewIssuer(secret, time.Hour)
	now := time.Now()
	legacy, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "42", "tv": 1, "iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
	}).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	h := &authHandlers{issuer: issuer}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/rotate", func(c *fiber.Ctx) error {
		if _, err := h.currentSessionHashOrRotate(c, 42, 1); err != nil {
			return err
		}
		return c.SendStatus(fiber.StatusNoContent)
	})
	req := httptest.NewRequest(fiber.MethodGet, "/rotate", nil)
	req.Header.Set(fiber.HeaderCookie, auth.CookieName+"="+legacy)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var rotated string
	for _, cookie := range resp.Cookies() {
		if cookie.Name == auth.CookieName {
			rotated = cookie.Value
		}
	}
	if rotated == "" || rotated == legacy {
		t.Fatal("legacy session was not rotated")
	}
	if _, err = issuer.SessionFingerprint(rotated); err != nil {
		t.Fatalf("rotated session cannot bind recent auth: %v", err)
	}
}

type passwordReauthFailureRepo struct {
	fakeRepo
	err error
}

func (r passwordReauthFailureRepo) UserByID(context.Context, int64) (accounts.User, error) {
	return accounts.User{ID: 7, Email: "reauth@example.test", HasPassword: true}, nil
}

func (r passwordReauthFailureRepo) UserByEmail(context.Context, string) (accounts.User, string, bool, error) {
	return accounts.User{}, "", false, r.err
}

func TestPasswordReauthPreservesInfrastructureFailure(t *testing.T) {
	h := &authHandlers{accounts: accounts.New(passwordReauthFailureRepo{err: errors.New("pgx: connection refused timeout")}, authHasher{})}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/reauth", func(c *fiber.Ctx) error {
		c.Locals(auth.LocalsUserID, int64(7))
		return c.Next()
	}, h.PasswordReauthV2)
	req := httptest.NewRequest(fiber.MethodPost, "/reauth", strings.NewReader(`{"password":"correct horse battery staple"}`))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Fatalf("status=%d, want 503", resp.StatusCode)
	}
}

var _ oauth.Provider = (*fakeProvider)(nil)
