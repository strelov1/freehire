//go:build integration

// Integration test for GET /api/v1/me/credits: a signed-in caller reads their AI-credits
// balance (fresh user reports the full monthly grant) without consuming any. Run with:
// go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/credits"
	"github.com/strelov1/freehire/internal/db"
)

func TestGetMyCredits(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)

	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('creditview@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	iss := auth.NewIssuer("test-secret", time.Hour)
	tok, _ := iss.Issue(userID)
	h := &API{pool: pool, queries: queries, issuer: iss,
		credits: credits.NewStore(queries, pool, credits.Config{MonthlyGrant: 20, CostMatch: 1, CostTailor: 3, ContributionReward: 5})}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/me/credits", auth.RequireAuthOrKey(iss, queries), h.GetMyCredits)

	req := httptest.NewRequest(fiber.MethodGet, "/api/v1/me/credits", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: tok})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		Data struct {
			Remaining int    `json:"remaining"`
			ResetsAt  string `json:"resets_at"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data.Remaining != 20 {
		t.Errorf("remaining = %d, want 20 (fresh monthly grant)", body.Data.Remaining)
	}
	if body.Data.ResetsAt == "" {
		t.Error("resets_at should be set")
	}
}
