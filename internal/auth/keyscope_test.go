package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
)

// fakeScopedKeys authenticates one token hash to a user id with a given scope, standing
// in for the api_keys row the DB layer returns. An unknown hash is pgx.ErrNoRows, as the
// real query reports it — the middleware distinguishes that from a lookup outage.
type fakeScopedKeys struct {
	validHash string
	userID    int64
	scope     string
}

func (f fakeScopedKeys) AuthenticateAPIKey(_ context.Context, tokenHash string) (APIKeyIdentity, error) {
	if tokenHash == f.validHash {
		return APIKeyIdentity{UserID: f.userID, Scope: f.scope}, nil
	}
	return APIKeyIdentity{}, pgx.ErrNoRows
}

// anyVersion accepts whatever version a token carries, so the scope tests exercise scope
// alone; revocation has its own tests in tokenversion_test.go.
type anyVersion struct{ version int32 }

func (a anyVersion) GetUserTokenVersion(_ context.Context, _ int64) (int32, error) {
	return a.version, nil
}

// scopeApp mounts one route behind the given middleware, echoing the resolved identity so
// a test can assert both the status and what the handler saw.
func scopeApp(mw fiber.Handler) *fiber.App {
	app := fiber.New()
	app.Get("/probe", mw, func(c *fiber.Ctx) error {
		id, _ := UserID(c)
		return c.JSON(fiber.Map{"id": id, "scope": KeyScope(c)})
	})
	return app
}

func keyRequest(token string) *http.Request {
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/probe", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

func TestRequireAuthOrKey_RejectsNarrowScope(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	const token = "fhk_tailoring"
	keys := fakeScopedKeys{validHash: HashAPIKey(token), userID: 4, scope: ScopeCV}

	resp, err := scopeApp(RequireAuthOrKey(iss, anyVersion{1}, keys)).Test(keyRequest(token))
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusForbidden {
		t.Errorf("status = %d, want 403 — a cv-scoped key must not reach a full-scope route", resp.StatusCode)
	}
}

func TestRequireAuthOrKey_AcceptsFullScope(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	const token = "fhk_cli"
	keys := fakeScopedKeys{validHash: HashAPIKey(token), userID: 4, scope: ScopeFull}

	resp, err := scopeApp(RequireAuthOrKey(iss, anyVersion{1}, keys)).Test(keyRequest(token))
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("status = %d, want 200 for a full-scope key", resp.StatusCode)
	}
}

func TestRequireAuthOrScopedKey_AdmitsBothScopes(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	for _, scope := range []string{ScopeFull, ScopeCV} {
		t.Run(scope, func(t *testing.T) {
			const token = "fhk_any"
			keys := fakeScopedKeys{validHash: HashAPIKey(token), userID: 4, scope: scope}

			resp, err := scopeApp(RequireAuthOrScopedKey(iss, anyVersion{1}, keys, ScopeCV)).Test(keyRequest(token))
			if err != nil {
				t.Fatalf("Test: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != fiber.StatusOK {
				t.Errorf("status = %d, want 200 — the CV surface admits %s keys", resp.StatusCode, scope)
			}
		})
	}
}

func TestRequireAuthOrScopedKey_StillRejectsUnknownKey(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	keys := fakeScopedKeys{validHash: HashAPIKey("fhk_real"), userID: 4, scope: ScopeCV}

	resp, err := scopeApp(RequireAuthOrScopedKey(iss, anyVersion{1}, keys, ScopeCV)).Test(keyRequest("fhk_forged"))
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401 — an unknown key is unauthenticated, not under-scoped", resp.StatusCode)
	}
}

func TestKeyScope_EmptyForCookieAuth(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	token, err := iss.Issue(7, 1)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/probe", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: token})
	resp, err := scopeApp(RequireAuthOrScopedKey(iss, anyVersion{1}, fakeScopedKeys{}, ScopeCV)).Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200 for a cookie session", resp.StatusCode)
	}
}
