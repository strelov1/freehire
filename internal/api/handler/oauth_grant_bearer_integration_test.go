//go:build integration

// Integration test proving an MCP OAuth grant (freehire#3114) authenticates
// exactly where a full-scope API key already does — the REST surface every
// non-browser client (the CLI, freehire-mcp) actually calls, not merely the
// signed-in MCP server at /api/v1/mcp/account. Run with:
// go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/application/jobtracking"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/auth/oauth2server"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

func TestOAuthGrant_AuthenticatesLikeAFullScopeAPIKey(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)

	userID := seedOAuthUser(t, pool, "oauth-bearer@example.test")
	if _, err := pool.Exec(ctx,
		`INSERT INTO jobs (source, external_id, url, title, public_slug)
		 VALUES ('test', 'oauth-bearer-1', 'http://example.test', 'Go Dev', 'go-dev-oauth-bearer')`); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	seedOAuthTestClient(t, queries, "client-bearer-1", []string{"http://127.0.0.1:9999/cb"})

	version, err := queries.GetUserTokenVersion(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserTokenVersion: %v", err)
	}
	const liveToken = "fhm_test-bearer-token"
	expiresAt := time.Now().Add(30 * 24 * time.Hour)
	if _, err := queries.CreateOAuthGrant(ctx, db.CreateOAuthGrantParams{
		UserID: userID, ClientID: "client-bearer-1", AccessTokenHash: oauth2server.HashToken(liveToken),
		IssuedTokenVersion: version, ExpiresAt: pgconv.Timestamptz(&expiresAt),
	}); err != nil {
		t.Fatalf("CreateOAuthGrant: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	th := &trackingHandlers{tracking: jobtracking.New(jobtracking.NewQueriesRepository(queries, pool))}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	keyAuth := auth.RequireAuthOrKey(iss, testVersions, apiKeys{queries})
	app.Post("/api/v1/jobs/:slug/apply", keyAuth, th.MarkApplied)
	app.Get("/api/v1/auth/me", auth.RequireAuthOrScopedKey(iss, testVersions, apiKeys{queries}, auth.ScopeCV), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	t.Run("a live grant authenticates a REST endpoint via Bearer", func(t *testing.T) {
		req := httptest.NewRequestWithContext(ctx, fiber.MethodPost, "/api/v1/jobs/go-dev-oauth-bearer/apply", nil)
		req.Header.Set("Authorization", "Bearer "+liveToken)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("apply: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("apply via oauth grant status = %d, want 200 (body %s)", resp.StatusCode, body)
		}
		var n int
		if err := pool.QueryRow(ctx,
			"SELECT count(*) FROM applications WHERE user_id = $1 AND applied_at IS NOT NULL", userID).Scan(&n); err != nil {
			t.Fatalf("count applied: %v", err)
		}
		if n != 1 {
			t.Errorf("applied rows for grant owner = %d, want 1", n)
		}
	})

	t.Run("a grant is full-scope, so it also passes a ScopeCV-gated route", func(t *testing.T) {
		req := httptest.NewRequestWithContext(ctx, fiber.MethodGet, "/api/v1/auth/me", nil)
		req.Header.Set("Authorization", "Bearer "+liveToken)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("me: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			t.Errorf("status = %d, want 200", resp.StatusCode)
		}
	})

	t.Run("an unknown bearer token is still rejected", func(t *testing.T) {
		req := httptest.NewRequestWithContext(ctx, fiber.MethodPost, "/api/v1/jobs/go-dev-oauth-bearer/apply", nil)
		req.Header.Set("Authorization", "Bearer fhm_no-such-token-at-all")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("apply: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusUnauthorized {
			t.Errorf("status = %d, want 401", resp.StatusCode)
		}
	})

	t.Run("a revoked grant (password reset deleted it) is rejected", func(t *testing.T) {
		if _, err := queries.BumpUserTokenVersion(ctx, userID); err != nil {
			t.Fatalf("BumpUserTokenVersion: %v", err)
		}
		req := httptest.NewRequestWithContext(ctx, fiber.MethodPost, "/api/v1/jobs/go-dev-oauth-bearer/apply", nil)
		req.Header.Set("Authorization", "Bearer "+liveToken)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("apply: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusUnauthorized {
			t.Errorf("status = %d, want 401", resp.StatusCode)
		}
	})
}
