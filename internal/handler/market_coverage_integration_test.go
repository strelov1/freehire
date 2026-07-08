//go:build integration

// Integration test for the stateless market-coverage endpoint against a real
// Postgres: POST /api/v1/market/coverage is behind RequireAuthOrKey, so an
// anonymous request is 401 and a request bearing a valid API key is served. The
// facet backend is stubbed (the coverage math is unit-tested in verdict); this
// test verifies the route + auth wiring end to end. Run with:
// go test -tags=integration ./internal/handler/
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/search"
)

func TestMarketCoverageEndpoint(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)

	var userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('coverage@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	cookie, _ := iss.Issue(userID)
	fake := &recordingFacetCounter{res: search.FacetResult{
		Total:  500,
		Facets: map[string]map[string]int64{"skills": {"go": 300}},
	}}
	h := &API{pool: pool, queries: queries, issuer: iss, facets: fake}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	keyAuth := auth.RequireAuthOrKey(iss, h.queries)
	app.Post("/api/v1/me/api-keys", auth.RequireAuth(iss), h.CreateAPIKey)
	app.Post("/api/v1/market/coverage", keyAuth, h.MarketCoverage)

	const path = "/api/v1/market/coverage?category=backend"

	// Anonymous → 401.
	anon := httptest.NewRequest(fiber.MethodPost, path, bytes.NewBufferString(`{"skills":["go"]}`))
	anon.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(anon)
	if err != nil {
		t.Fatalf("anon request: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("anon status = %d, want 401", resp.StatusCode)
	}

	// Mint an API key via the cookie session.
	createReq := httptest.NewRequest(fiber.MethodPost, "/api/v1/me/api-keys", bytes.NewBufferString(`{"name":"ci"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	createResp, err := app.Test(createReq)
	if err != nil || createResp.StatusCode != fiber.StatusCreated {
		t.Fatalf("create key: err=%v status=%d", err, createResp.StatusCode)
	}
	var created struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	_ = json.NewDecoder(createResp.Body).Decode(&created)
	if created.Data.Token == "" {
		t.Fatal("create key returned no plaintext token")
	}

	// Bearer key → 200 with the coverage payload.
	keyed := httptest.NewRequest(fiber.MethodPost, path, bytes.NewBufferString(`{"skills":["go"]}`))
	keyed.Header.Set("Content-Type", "application/json")
	keyed.Header.Set("Authorization", "Bearer "+created.Data.Token)
	keyedResp, err := app.Test(keyed)
	if err != nil {
		t.Fatalf("keyed request: %v", err)
	}
	if keyedResp.StatusCode != fiber.StatusOK {
		t.Fatalf("keyed status = %d, want 200", keyedResp.StatusCode)
	}
	var body struct {
		Data struct {
			Total           int64 `json:"total"`
			CoveragePercent int   `json:"coverage_percent"`
		} `json:"data"`
	}
	_ = json.NewDecoder(keyedResp.Body).Decode(&body)
	if body.Data.Total != 500 {
		t.Errorf("total = %d, want 500", body.Data.Total)
	}

	// The market filter carried the query facet through to the role query.
	if !filterHas(fake.callFilter(0), `enrichment.category = "backend"`) {
		t.Errorf("role query missing category facet: %#v", fake.callFilter(0))
	}
}
