//go:build integration

// Integration test for the mobile OAuth code exchange against a real Postgres:
// a minted one-time code exchanges once for a session (setting the auth cookie)
// and the resolved user, and a reused code is rejected. Run with:
// go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/accounts"
	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/auth/oauth"
	"github.com/strelov1/freehire/internal/db"
)

func TestOAuthExchangeEndToEnd(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('oauth@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	queries := db.New(pool)
	codes := oauth.NewCodeStore(time.Minute)
	h := &API{
		pool:       pool,
		queries:    queries,
		issuer:     auth.NewIssuer("test-secret", time.Hour),
		oauthCodes: codes,
		accounts:   accounts.New(accounts.NewQueriesRepository(queries, pool), authHasher{}),
	}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/api/v1/auth/oauth/exchange", h.OAuthExchange)

	code, err := codes.Mint(userID)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}

	exchange := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(fiber.MethodPost, "/api/v1/auth/oauth/exchange", strings.NewReader(`{"code":"`+code+`"}`))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Test: %v", err)
		}
		rec := httptest.NewRecorder()
		rec.Code = resp.StatusCode
		for k, vs := range resp.Header {
			for _, v := range vs {
				rec.Header().Add(k, v)
			}
		}
		if b, _ := io.ReadAll(resp.Body); len(b) > 0 {
			rec.Body.Write(b)
		}
		return rec
	}

	first := exchange()
	if first.Code != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", first.Code)
	}
	if sc := strings.Join(first.Header().Values("Set-Cookie"), "\n"); !strings.Contains(sc, auth.CookieName+"=") {
		t.Errorf("exchange did not set the session cookie: %q", sc)
	}
	var out struct {
		Data struct {
			Email string `json:"email"`
		} `json:"data"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Data.Email != "oauth@example.test" {
		t.Errorf("email = %q, want oauth@example.test", out.Data.Email)
	}

	// Single-use: the same code can't be redeemed twice.
	if second := exchange(); second.Code != fiber.StatusUnauthorized {
		t.Errorf("reused code status = %d, want 401", second.Code)
	}
}
