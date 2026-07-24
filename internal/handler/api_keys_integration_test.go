//go:build integration

// Integration tests for the API-key HTTP flow against a real Postgres: create
// returns the plaintext token once, the token authenticates a per-user endpoint
// via Authorization: Bearer, listing never leaks the secret, and revoking is
// owner-scoped and immediately disables the key. Run with:
// go test -tags=integration ./internal/handler/
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobtracking"
)

func TestAPIKeysEndToEnd(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var ownerID, otherID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('keys@example.test') RETURNING id`).Scan(&ownerID); err != nil {
		t.Fatalf("seed owner: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('other@example.test') RETURNING id`).Scan(&otherID); err != nil {
		t.Fatalf("seed other user: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO jobs (source, external_id, url, title, public_slug)
		 VALUES ('test', 'apikey-1', 'http://example.test', 'Go Dev', 'go-dev-acme-t35nijto')`); err != nil {
		t.Fatalf("seed job: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	ownerCookie, _ := iss.Issue(ownerID)
	otherCookie, _ := iss.Issue(otherID)
	queries := db.New(pool)
	h := &API{pool: pool, queries: queries, issuer: iss, tracking: jobtracking.New(jobtracking.NewQueriesRepository(queries, pool))}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	keyAuth := auth.RequireAuthOrKey(iss, h.queries)
	app.Post("/api/v1/me/api-keys", auth.RequireAuth(iss), h.CreateAPIKey)
	app.Get("/api/v1/me/api-keys", auth.RequireAuth(iss), h.ListAPIKeys)
	app.Delete("/api/v1/me/api-keys/:id", auth.RequireAuth(iss), h.RevokeAPIKey)
	app.Post("/api/v1/jobs/:slug/apply", keyAuth, h.MarkApplied)

	const applyPath = "/api/v1/jobs/go-dev-acme-t35nijto/apply"

	cookieReq := func(method, path, cookie string, body []byte) *http.Request {
		var r *http.Request
		if body != nil {
			r = httptest.NewRequest(method, path, bytes.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
		} else {
			r = httptest.NewRequest(method, path, nil)
		}
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		return r
	}

	// Create a key (cookie session).
	createResp, err := app.Test(cookieReq(fiber.MethodPost, "/api/v1/me/api-keys", ownerCookie, []byte(`{"name":"ci"}`)))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if createResp.StatusCode != fiber.StatusCreated {
		body, _ := io.ReadAll(createResp.Body)
		t.Fatalf("create status = %d, want 201 (body %s)", createResp.StatusCode, body)
	}
	var created struct {
		Data struct {
			ID          int64  `json:"id"`
			Token       string `json:"token"`
			TokenPrefix string `json:"token_prefix"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.Data.Token == "" {
		t.Fatal("create did not return the plaintext token")
	}
	if created.Data.TokenPrefix == "" || created.Data.TokenPrefix == created.Data.Token {
		t.Errorf("token_prefix = %q, want a short non-secret prefix", created.Data.TokenPrefix)
	}
	keyID, token := created.Data.ID, created.Data.Token

	bearerReq := func(method, path string) *http.Request {
		r := httptest.NewRequest(method, path, nil)
		r.Header.Set("Authorization", "Bearer "+token)
		return r
	}

	t.Run("the key authenticates a per-user endpoint via Bearer", func(t *testing.T) {
		resp, err := app.Test(bearerReq(fiber.MethodPost, applyPath))
		if err != nil {
			t.Fatalf("apply: %v", err)
		}
		if resp.StatusCode != fiber.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("apply via key status = %d, want 200 (body %s)", resp.StatusCode, body)
		}
		var n int
		if err := pool.QueryRow(ctx,
			"SELECT count(*) FROM user_jobs WHERE user_id = $1 AND applied_at IS NOT NULL", ownerID).Scan(&n); err != nil {
			t.Fatalf("count applied: %v", err)
		}
		if n != 1 {
			t.Errorf("applied rows for key owner = %d, want 1", n)
		}
	})

	t.Run("list returns the key without the secret", func(t *testing.T) {
		resp, err := app.Test(cookieReq(fiber.MethodGet, "/api/v1/me/api-keys", ownerCookie, nil))
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		body, _ := io.ReadAll(resp.Body)
		if bytes.Contains(body, []byte(token)) {
			t.Error("list response leaked the plaintext token")
		}
		if bytes.Contains(body, []byte("token_hash")) {
			t.Error("list response leaked token_hash")
		}
		if !bytes.Contains(body, []byte(`"name":"ci"`)) {
			t.Errorf("list response missing the created key: %s", body)
		}
		if !bytes.Contains(body, []byte(`"meta"`)) {
			t.Errorf("list response missing the meta envelope (list convention): %s", body)
		}
	})

	t.Run("revoke is owner-scoped and disables the key", func(t *testing.T) {
		path := fmt.Sprintf("/api/v1/me/api-keys/%d", keyID)

		other, err := app.Test(cookieReq(fiber.MethodDelete, path, otherCookie, nil))
		if err != nil {
			t.Fatalf("revoke(other): %v", err)
		}
		if other.StatusCode != fiber.StatusNotFound {
			t.Errorf("other-user revoke status = %d, want 404", other.StatusCode)
		}

		owner, err := app.Test(cookieReq(fiber.MethodDelete, path, ownerCookie, nil))
		if err != nil {
			t.Fatalf("revoke(owner): %v", err)
		}
		if owner.StatusCode != fiber.StatusNoContent {
			t.Errorf("owner revoke status = %d, want 204", owner.StatusCode)
		}

		after, err := app.Test(bearerReq(fiber.MethodPost, applyPath))
		if err != nil {
			t.Fatalf("apply after revoke: %v", err)
		}
		if after.StatusCode != fiber.StatusUnauthorized {
			t.Errorf("revoked key apply status = %d, want 401", after.StatusCode)
		}
	})
}
