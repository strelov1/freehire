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
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/auth/oauth2server"
	"github.com/strelov1/freehire/internal/platform/db"
)

type introspectionFixture struct {
	keyHash, grantHash string
	scope              string
	version            int32
	failed             bool
	calls              int
}

func (f *introspectionFixture) ResolveAPIKeyIdentity(_ context.Context, hash string) (db.ResolveAPIKeyIdentityRow, error) {
	f.calls++
	if f.failed {
		return db.ResolveAPIKeyIdentityRow{}, errors.New("database unavailable")
	}
	if hash != f.keyHash {
		return db.ResolveAPIKeyIdentityRow{}, pgx.ErrNoRows
	}
	return db.ResolveAPIKeyIdentityRow{UserID: 42, Scope: f.scope, ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true}, ProUntil: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true}}, nil
}
func (f *introspectionFixture) ResolveOAuthGrantIdentity(_ context.Context, hash string) (db.ResolveOAuthGrantIdentityRow, error) {
	f.calls++
	if hash != f.grantHash {
		return db.ResolveOAuthGrantIdentityRow{}, pgx.ErrNoRows
	}
	return db.ResolveOAuthGrantIdentityRow{UserID: 42, ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true}}, nil
}
func (f *introspectionFixture) GetUserTokenVersion(context.Context, int64) (int32, error) {
	f.calls++
	if f.failed {
		return 0, errors.New("database unavailable")
	}
	return f.version, nil
}
func (f *introspectionFixture) GetUserAPIEntitlement(context.Context, int64) (db.GetUserAPIEntitlementRow, error) {
	return db.GetUserAPIEntitlementRow{}, nil
}

func TestAuthIntrospectionCredentialsAndTrust(t *testing.T) {
	key, keyHash, _, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	grant, grantHash, err := oauth2server.GenerateAccessToken()
	if err != nil {
		t.Fatal(err)
	}
	issuer := auth.NewIssuer(strings.Repeat("s", 32), time.Hour)
	session, err := issuer.Issue(42, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, token, cookie, scope, secret string
		version                            int32
		failed                             bool
		status                             int
		kind                               string
	}{
		{name: "api key", token: key, scope: auth.ScopeFull, secret: "secret", status: 200, kind: "api_key"},
		{name: "oauth", token: grant, secret: "secret", status: 200, kind: "oauth_grant"},
		{name: "narrow identity is not a permission grant", token: key, scope: auth.ScopeCV, secret: "secret", status: 200, kind: "api_key"},
		{name: "cookie", cookie: session, secret: "secret", version: 1, status: 200, kind: "session"},
		{name: "bearer session", token: session, secret: "secret", version: 1, status: 200, kind: "session"},
		{name: "cookie precedes bearer", cookie: session, token: key, secret: "secret", version: 1, status: 200, kind: "session"},
		{name: "revoked cookie falls back to bearer", cookie: session, token: grant, secret: "secret", version: 2, status: 200, kind: "oauth_grant"},
		{name: "revoked session", cookie: session, secret: "secret", version: 2, status: 401},
		{name: "missing credential", secret: "secret", status: 401},
		{name: "malformed credential", token: "invalid", secret: "secret", status: 401},
		{name: "missing infrastructure proof", token: key, scope: auth.ScopeFull, status: 403},
		{name: "wrong infrastructure proof", token: key, scope: auth.ScopeFull, secret: "wrong", status: 403},
		{name: "database outage", token: key, scope: auth.ScopeFull, secret: "secret", failed: true, status: 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &introspectionFixture{keyHash: keyHash, grantHash: grantHash, scope: tc.scope, version: tc.version, failed: tc.failed}
			app := fiber.New(fiber.Config{ErrorHandler: RenderError})
			app.Get("/internal/auth/introspect", newAuthIntrospection(store, issuer, "secret"))
			req := httptest.NewRequestWithContext(context.Background(), "GET", "/internal/auth/introspect", nil)
			req.Header.Set(authIntrospectionSecretHeader, tc.secret)
			req.Header.Set("X-FH-Auth-User", "999")
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}
			if tc.cookie != "" {
				req.Header.Set("Cookie", auth.CookieName+"="+tc.cookie)
			}
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.status {
				t.Fatalf("status=%d want %d", resp.StatusCode, tc.status)
			}
			if tc.status == 200 {
				var got introspectedIdentity
				if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
					t.Fatal(err)
				}
				if got.UserID != "42" || got.CredentialKind != tc.kind || got.ExpiresAt <= time.Now().Unix() {
					t.Fatalf("unexpected identity: %+v", got)
				}
				if tc.kind == "api_key" && got.Scope != tc.scope {
					t.Fatalf("scope=%q want%q", got.Scope, tc.scope)
				}
				if resp.Header.Get("Cache-Control") != "no-store" {
					t.Fatal("introspection response must not enter shared HTTP caches")
				}
			}
			if tc.secret != "secret" && store.calls != 0 {
				t.Fatal("unauthorized infrastructure reached storage")
			}
		})
	}
}
func TestAuthIntrospectionDisabled(t *testing.T) {
	app := fiber.New()
	app.Get("/identity", newAuthIntrospection(nil, nil, ""))
	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), "GET", "/identity", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatal(resp.StatusCode)
	}
}
