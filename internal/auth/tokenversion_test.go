package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
)

// fakeVersions reports one account's current session generation, standing in for the
// users.token_version column. A zero user id errors, standing in for a deleted account.
type fakeVersions struct {
	userID  int64
	version int32
}

func (f fakeVersions) GetUserTokenVersion(_ context.Context, id int64) (int32, error) {
	if id != f.userID {
		return 0, pgx.ErrNoRows
	}
	return f.version, nil
}

// legacyToken mints a correctly signed token with no version claim — exactly what every
// session issued before this change looks like.
func legacyToken(t *testing.T, secret string, userID int64) string {
	t.Helper()
	claims := jwt.RegisteredClaims{
		Subject:   "7",
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign legacy token: %v", err)
	}
	return token
}

func TestIssuer_CarriesTokenVersion(t *testing.T) {
	iss := NewIssuer("test-secret", time.Hour)

	token, err := iss.Issue(42, 3)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	id, version, err := iss.Parse(token)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if id != 42 {
		t.Errorf("user id = %d, want 42", id)
	}
	if version != 3 {
		t.Errorf("token version = %d, want 3", version)
	}
}

func TestIssuer_RejectsTokenWithoutVersion(t *testing.T) {
	iss := NewIssuer("test-secret", time.Hour)

	if _, _, err := iss.Parse(legacyToken(t, "test-secret", 7)); err == nil {
		t.Error("Parse should reject a correctly signed token that carries no version claim")
	}
}

// versionedApp mounts a protected route behind RequireAuth with a version loader.
func versionedApp(iss *Issuer, versions TokenVersionLoader) *fiber.App {
	app := fiber.New()
	app.Get("/protected", RequireAuth(iss, versions), func(c *fiber.Ctx) error {
		id, _ := UserID(c)
		return c.JSON(fiber.Map{"id": id})
	})
	return app
}

func cookieRequest(token string) *http.Request {
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/protected", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: token})
	return req
}

func TestRequireAuth_AcceptsCurrentVersion(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	token, err := iss.Issue(7, 4)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	resp, err := versionedApp(iss, fakeVersions{userID: 7, version: 4}).Test(cookieRequest(token))
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("status = %d, want 200 for a current token", resp.StatusCode)
	}
}

func TestRequireAuth_RejectsRevokedVersion(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	token, err := iss.Issue(7, 4)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	// The account signed out everywhere after this token was minted.
	resp, err := versionedApp(iss, fakeVersions{userID: 7, version: 5}).Test(cookieRequest(token))
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for a revoked token", resp.StatusCode)
	}
}

func TestRequireAuth_RejectsUnknownAccount(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	token, err := iss.Issue(7, 1)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	// The version load fails (deleted account): fail closed, like RequireRole.
	resp, err := versionedApp(iss, fakeVersions{userID: 99, version: 1}).Test(cookieRequest(token))
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401 when the account's version cannot be loaded", resp.StatusCode)
	}
}

func TestRequireAuth_RejectsLegacyToken(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)

	resp, err := versionedApp(iss, fakeVersions{userID: 7, version: 1}).
		Test(cookieRequest(legacyToken(t, "secret", 7)))
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401 — a pre-versioning token must not survive the deploy", resp.StatusCode)
	}
}
