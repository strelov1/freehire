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

// fakeKeyAuth authenticates exactly one token hash to a user id; any other hash
// errors, standing in for an unknown, revoked, or expired key (the real DB layer
// distinguishes those; the middleware treats every error as "not authenticated").
type fakeKeyAuth struct {
	validHash string
	userID    int64
}

func (f fakeKeyAuth) AuthenticateAPIKey(_ context.Context, tokenHash string) (int64, error) {
	if tokenHash == f.validHash {
		return f.userID, nil
	}
	return 0, errors.New("no such key")
}

// dualAuthApp mounts a route behind RequireAuthOrKey that echoes the resolved user
// id, so tests assert both access control and that identity propagates into the
// handler via the shared c.Locals.
func dualAuthApp(iss *Issuer, keys APIKeyAuthenticator) *fiber.App {
	app := fiber.New()
	app.Get("/me", RequireAuthOrKey(iss, keys), func(c *fiber.Ctx) error {
		id, ok := UserID(c)
		if !ok {
			return fiber.NewError(fiber.StatusInternalServerError, "user id missing from context")
		}
		return c.JSON(fiber.Map{"id": id, "via_key": ViaAPIKey(c)})
	})
	return app
}

func decodeID(t *testing.T, resp *http.Response) int64 {
	t.Helper()
	var body struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body.ID
}

func TestRequireAuthOrKey_ValidKeyAuthenticates(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	const token = "fhk_test-key"
	keys := fakeKeyAuth{validHash: HashAPIKey(token), userID: 9}

	req := httptest.NewRequest(fiber.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := dualAuthApp(iss, keys).Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if id := decodeID(t, resp); id != 9 {
		t.Errorf("handler saw user id %d, want 9", id)
	}
}

func decodeViaKey(t *testing.T, resp *http.Response) bool {
	t.Helper()
	var body struct {
		ViaKey bool `json:"via_key"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body.ViaKey
}

func TestRequireAuthOrKey_FlagsKeyAuth(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	const token = "fhk_test-key"
	keys := fakeKeyAuth{validHash: HashAPIKey(token), userID: 9}

	req := httptest.NewRequest(fiber.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := dualAuthApp(iss, keys).Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if !decodeViaKey(t, resp) {
		t.Error("ViaAPIKey should be true for key auth")
	}
}

func TestRequireAuthOrKey_CookieIsNotViaKey(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	token, _ := iss.Issue(7)

	req := httptest.NewRequest(fiber.MethodGet, "/me", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: token})
	resp, err := dualAuthApp(iss, fakeKeyAuth{}).Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if decodeViaKey(t, resp) {
		t.Error("ViaAPIKey should be false for cookie auth")
	}
}

func TestRequireAuthOrKey_ValidCookieAuthenticates(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	keys := fakeKeyAuth{} // no valid key; the cookie must carry the identity
	token, _ := iss.Issue(7)

	req := httptest.NewRequest(fiber.MethodGet, "/me", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: token})

	resp, err := dualAuthApp(iss, keys).Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if id := decodeID(t, resp); id != 7 {
		t.Errorf("handler saw user id %d, want 7", id)
	}
}

func TestRequireAuthOrKey_CookieTakesPrecedenceOverKey(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	const token = "fhk_test-key"
	keys := fakeKeyAuth{validHash: HashAPIKey(token), userID: 9}
	cookie, _ := iss.Issue(7)

	req := httptest.NewRequest(fiber.MethodGet, "/me", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: cookie})
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := dualAuthApp(iss, keys).Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if id := decodeID(t, resp); id != 7 {
		t.Errorf("handler saw user id %d, want 7 (a valid cookie should win)", id)
	}
}

func TestRequireAuthOrKey_InvalidCookieFallsThroughToKey(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	const token = "fhk_test-key"
	keys := fakeKeyAuth{validHash: HashAPIKey(token), userID: 9}

	req := httptest.NewRequest(fiber.MethodGet, "/me", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: "not-a-jwt"})
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := dualAuthApp(iss, keys).Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if id := decodeID(t, resp); id != 9 {
		t.Errorf("handler saw user id %d, want 9 (key authenticates after a bad cookie)", id)
	}
}

func TestRequireAuthOrKey_RejectsUnauthorized(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	const token = "fhk_valid"
	keys := fakeKeyAuth{validHash: HashAPIKey(token), userID: 9}

	cases := []struct {
		name   string
		cookie string // empty = no cookie
		bearer string // empty = no Authorization header
	}{
		{"no credentials", "", ""},
		{"unknown key", "", "fhk_unknown"},
		{"garbage bearer", "", "not-even-prefixed"},
		{"malformed cookie only", "not-a-jwt", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(fiber.MethodGet, "/me", nil)
			if tc.cookie != "" {
				req.AddCookie(&http.Cookie{Name: CookieName, Value: tc.cookie})
			}
			if tc.bearer != "" {
				req.Header.Set("Authorization", "Bearer "+tc.bearer)
			}
			resp, err := dualAuthApp(iss, keys).Test(req)
			if err != nil {
				t.Fatalf("Test: %v", err)
			}
			if resp.StatusCode != fiber.StatusUnauthorized {
				t.Errorf("status = %d, want 401", resp.StatusCode)
			}
		})
	}
}
