//go:build integration

// Integration tests for the Connected-devices HTTP flow against a real
// Postgres: listing shows only the caller's own grants, and revoking is
// owner-scoped and immediately disables the grant. Run with:
// go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

func seedOAuthGrant(t *testing.T, queries *db.Queries, userID int64, clientID, tokenHash string) int64 {
	t.Helper()
	version, err := queries.GetUserTokenVersion(context.Background(), userID)
	if err != nil {
		t.Fatalf("GetUserTokenVersion: %v", err)
	}
	expiresAt := time.Now().Add(30 * 24 * time.Hour)
	grant, err := queries.CreateOAuthGrant(context.Background(), db.CreateOAuthGrantParams{
		UserID: userID, ClientID: clientID, AccessTokenHash: tokenHash,
		IssuedTokenVersion: version, ExpiresAt: pgconv.Timestamptz(&expiresAt),
	})
	if err != nil {
		t.Fatalf("CreateOAuthGrant: %v", err)
	}
	return grant.ID
}

func TestOAuthGrantsEndToEnd(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &authHandlers{queries: queries}

	owner := seedOAuthUser(t, pool, "grants-owner@example.test")
	other := seedOAuthUser(t, pool, "grants-other@example.test")
	seedOAuthTestClient(t, queries, "client-grants-1", []string{"http://127.0.0.1:9999/callback"})
	grantID := seedOAuthGrant(t, queries, owner, "client-grants-1", "grant-hash-1")

	ownerCookie, err := iss.Issue(owner, testTokenVersion)
	if err != nil {
		t.Fatalf("Issue(owner): %v", err)
	}
	otherCookie, err := iss.Issue(other, testTokenVersion)
	if err != nil {
		t.Fatalf("Issue(other): %v", err)
	}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/me/oauth-grants", auth.RequireAuth(iss, testVersions), h.ListOAuthGrants)
	app.Delete("/api/v1/me/oauth-grants/:id", auth.RequireAuth(iss, testVersions), h.RevokeOAuthGrant)

	cookieReq := func(method, path, cookie string) *http.Request {
		r := httptest.NewRequestWithContext(context.Background(), method, path, nil)
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		return r
	}

	t.Run("owner lists their own grant", func(t *testing.T) {
		resp, err := app.Test(cookieReq(fiber.MethodGet, "/api/v1/me/oauth-grants", ownerCookie))
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		var out struct {
			Data []oauthGrantResponse `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(out.Data) != 1 || out.Data[0].ClientName != "Test MCP Client" {
			t.Errorf("data = %+v", out.Data)
		}
	})

	t.Run("another user sees no grants and cannot revoke this one", func(t *testing.T) {
		listResp, err := app.Test(cookieReq(fiber.MethodGet, "/api/v1/me/oauth-grants", otherCookie))
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		defer listResp.Body.Close()
		var out struct {
			Data []oauthGrantResponse `json:"data"`
		}
		if err := json.NewDecoder(listResp.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(out.Data) != 0 {
			t.Errorf("other user's list = %+v, want empty", out.Data)
		}

		delResp, err := app.Test(cookieReq(fiber.MethodDelete, fmt.Sprintf("/api/v1/me/oauth-grants/%d", grantID), otherCookie))
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		defer delResp.Body.Close()
		if delResp.StatusCode != fiber.StatusNotFound {
			t.Errorf("status = %d, want 404", delResp.StatusCode)
		}
	})

	t.Run("owner revokes their own grant and it stops authenticating", func(t *testing.T) {
		resp, err := app.Test(cookieReq(fiber.MethodDelete, fmt.Sprintf("/api/v1/me/oauth-grants/%d", grantID), ownerCookie))
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusNoContent {
			t.Fatalf("status = %d, want 204", resp.StatusCode)
		}

		if _, err := queries.AuthenticateOAuthGrant(context.Background(), "grant-hash-1"); err == nil {
			t.Error("revoked grant still authenticates")
		}
	})
}
