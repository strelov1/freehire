//go:build integration

// Integration tests for the subscription HTTP flow against a real Postgres: a
// signed-in user subscribes one of their saved searches to a channel, lists,
// pauses, and unsubscribes, with owner-scoping and the duplicate/invalid-channel
// guards. Cookie-only (RequireAuth). Run with: go test -tags=integration ./internal/handler/
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
	"github.com/strelov1/freehire/internal/subscription"
)

func TestSubscriptionsEndToEnd(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var ownerID, otherID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('subs@example.test') RETURNING id`).Scan(&ownerID); err != nil {
		t.Fatalf("seed owner: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('other-subs@example.test') RETURNING id`).Scan(&otherID); err != nil {
		t.Fatalf("seed other: %v", err)
	}
	var ownerSS, otherSS int64
	if err := pool.QueryRow(ctx, `INSERT INTO saved_searches (user_id, name, query) VALUES ($1,'Go','q=go') RETURNING id`, ownerID).Scan(&ownerSS); err != nil {
		t.Fatalf("seed owner saved search: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO saved_searches (user_id, name, query) VALUES ($1,'Rust','q=rust') RETURNING id`, otherID).Scan(&otherSS); err != nil {
		t.Fatalf("seed other saved search: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	ownerCookie, _ := iss.Issue(ownerID)
	otherCookie, _ := iss.Issue(otherID)
	queries := db.New(pool)
	h := &API{pool: pool, queries: queries, issuer: iss, subscription: subscription.New(subscription.NewQueriesRepository(queries))}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	guard := auth.RequireAuth(iss)
	app.Post("/api/v1/me/subscriptions", guard, h.CreateSubscription)
	app.Get("/api/v1/me/subscriptions", guard, h.ListSubscriptions)
	app.Patch("/api/v1/me/subscriptions/:id", guard, h.SetSubscriptionActive)
	app.Delete("/api/v1/me/subscriptions/:id", guard, h.DeleteSubscription)

	const path = "/api/v1/me/subscriptions"
	req := func(method, p, cookie, body string) *http.Request {
		var r io.Reader
		if body != "" {
			r = bytes.NewBufferString(body)
		}
		rq := httptest.NewRequest(method, p, r)
		if body != "" {
			rq.Header.Set("Content-Type", "application/json")
		}
		if cookie != "" {
			rq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		}
		return rq
	}
	do := func(rq *http.Request) (*http.Response, map[string]any) {
		res, err := app.Test(rq, -1)
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		var env map[string]any
		b, _ := io.ReadAll(res.Body)
		if len(b) > 0 {
			_ = json.Unmarshal(b, &env)
		}
		return res, env
	}

	// Unauthenticated → 401.
	if res, _ := do(req(http.MethodGet, path, "", "")); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauth list status = %d, want 401", res.StatusCode)
	}

	// Create on the owner's saved search → 201, defaults to telegram + active.
	res, env := do(req(http.MethodPost, path, ownerCookie, fmt.Sprintf(`{"saved_search_id":%d}`, ownerSS)))
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", res.StatusCode)
	}
	created := env["data"].(map[string]any)
	subID := int64(created["id"].(float64))
	if created["channel"] != "telegram" || created["active"] != true {
		t.Errorf("created = %+v, want telegram/active", created)
	}

	// Duplicate (same saved search + channel) → 409.
	if res, _ := do(req(http.MethodPost, path, ownerCookie, fmt.Sprintf(`{"saved_search_id":%d}`, ownerSS))); res.StatusCode != http.StatusConflict {
		t.Errorf("duplicate status = %d, want 409", res.StatusCode)
	}

	// Unsupported channel → 400.
	if res, _ := do(req(http.MethodPost, path, ownerCookie, fmt.Sprintf(`{"saved_search_id":%d,"channel":"carrier-pigeon"}`, ownerSS))); res.StatusCode != http.StatusBadRequest {
		t.Errorf("bad-channel status = %d, want 400", res.StatusCode)
	}

	// Cannot subscribe to another user's saved search → 404.
	if res, _ := do(req(http.MethodPost, path, ownerCookie, fmt.Sprintf(`{"saved_search_id":%d}`, otherSS))); res.StatusCode != http.StatusNotFound {
		t.Errorf("cross-user saved-search status = %d, want 404", res.StatusCode)
	}

	// List returns the owner's one subscription with its saved-search name.
	_, env = do(req(http.MethodGet, path, ownerCookie, ""))
	data := env["data"].([]any)
	if len(data) != 1 || data[0].(map[string]any)["saved_search_name"] != "Go" {
		t.Errorf("owner list = %+v, want one item named Go", data)
	}
	// Another user sees none.
	_, env = do(req(http.MethodGet, path, otherCookie, ""))
	if len(env["data"].([]any)) != 0 {
		t.Errorf("other-user list len = %d, want 0", len(env["data"].([]any)))
	}

	// Pause it → active false.
	res, env = do(req(http.MethodPatch, fmt.Sprintf("%s/%d", path, subID), ownerCookie, `{"active":false}`))
	if res.StatusCode != http.StatusOK || env["data"].(map[string]any)["active"] != false {
		t.Errorf("pause status=%d data=%+v, want 200/active=false", res.StatusCode, env["data"])
	}
	// Cross-user toggle → 404.
	if res, _ := do(req(http.MethodPatch, fmt.Sprintf("%s/%d", path, subID), otherCookie, `{"active":true}`)); res.StatusCode != http.StatusNotFound {
		t.Errorf("cross-user toggle status = %d, want 404", res.StatusCode)
	}

	// Cross-user delete → 404; owner delete → 204; re-delete → 404.
	if res, _ := do(req(http.MethodDelete, fmt.Sprintf("%s/%d", path, subID), otherCookie, "")); res.StatusCode != http.StatusNotFound {
		t.Errorf("cross-user delete status = %d, want 404", res.StatusCode)
	}
	if res, _ := do(req(http.MethodDelete, fmt.Sprintf("%s/%d", path, subID), ownerCookie, "")); res.StatusCode != http.StatusNoContent {
		t.Errorf("owner delete status = %d, want 204", res.StatusCode)
	}
	if res, _ := do(req(http.MethodDelete, fmt.Sprintf("%s/%d", path, subID), ownerCookie, "")); res.StatusCode != http.StatusNotFound {
		t.Errorf("re-delete status = %d, want 404", res.StatusCode)
	}
}
