//go:build integration

// Integration tests for the saved-search HTTP flow against a real Postgres: a
// signed-in user creates / lists / updates / deletes named filter snapshots, the
// per-user cap and name rules are enforced, and every operation is owner-scoped so
// one user can never touch another's row. Cookie-only (RequireAuth). Run with:
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
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/savedsearch"
)

func TestSavedSearchesEndToEnd(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var ownerID, otherID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('searches@example.test') RETURNING id`).Scan(&ownerID); err != nil {
		t.Fatalf("seed owner: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('other-searches@example.test') RETURNING id`).Scan(&otherID); err != nil {
		t.Fatalf("seed other user: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	ownerCookie, _ := iss.Issue(ownerID)
	otherCookie, _ := iss.Issue(otherID)
	queries := db.New(pool)
	h := &API{pool: pool, queries: queries, issuer: iss, savedSearch: savedsearch.New(savedsearch.NewQueriesRepository(queries))}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	guard := auth.RequireAuth(iss)
	app.Post("/api/v1/me/searches", guard, h.CreateSavedSearch)
	app.Get("/api/v1/me/searches", guard, h.ListSavedSearches)
	app.Patch("/api/v1/me/searches/:id", guard, h.UpdateSavedSearch)
	app.Delete("/api/v1/me/searches/:id", guard, h.DeleteSavedSearch)

	const path = "/api/v1/me/searches"

	req := func(method, p, cookie string, body string) *http.Request {
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

	// Unauthenticated → 401, nothing stored.
	if res, _ := do(req(http.MethodGet, path, "", "")); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated list status = %d, want 401", res.StatusCode)
	}

	// Create (empty query is the valid "show all" snapshot).
	res, env := do(req(http.MethodPost, path, ownerCookie, `{"name":"Remote Go","query":"q=go&work_mode=remote"}`))
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", res.StatusCode)
	}
	created := env["data"].(map[string]any)
	id := int64(created["id"].(float64))
	if created["name"] != "Remote Go" || created["query"] != "q=go&work_mode=remote" {
		t.Errorf("created body = %+v, fields not carried", created)
	}
	if res, _ := do(req(http.MethodPost, path, ownerCookie, `{"name":"All jobs","query":""}`)); res.StatusCode != http.StatusCreated {
		t.Fatalf("create with empty query status = %d, want 201", res.StatusCode)
	}

	// List returns the owner's two sets only.
	_, env = do(req(http.MethodGet, path, ownerCookie, ""))
	if got := len(env["data"].([]any)); got != 2 {
		t.Errorf("owner list len = %d, want 2", got)
	}
	// Another user sees none of them.
	_, env = do(req(http.MethodGet, path, otherCookie, ""))
	if got := len(env["data"].([]any)); got != 0 {
		t.Errorf("other-user list len = %d, want 0", got)
	}

	// Duplicate name → 409.
	if res, _ := do(req(http.MethodPost, path, ownerCookie, `{"name":"Remote Go","query":"q=rust"}`)); res.StatusCode != http.StatusConflict {
		t.Errorf("duplicate-name status = %d, want 409", res.StatusCode)
	}

	// Blank and over-long names → 400.
	if res, _ := do(req(http.MethodPost, path, ownerCookie, `{"name":"   ","query":""}`)); res.StatusCode != http.StatusBadRequest {
		t.Errorf("blank-name status = %d, want 400", res.StatusCode)
	}
	longName := strings.Repeat("x", 101)
	if res, _ := do(req(http.MethodPost, path, ownerCookie, fmt.Sprintf(`{"name":%q,"query":""}`, longName))); res.StatusCode != http.StatusBadRequest {
		t.Errorf("over-long-name status = %d, want 400", res.StatusCode)
	}

	// Update: overwrite the query (partial) and bump updated_at.
	res, env = do(req(http.MethodPatch, fmt.Sprintf("%s/%d", path, id), ownerCookie, `{"query":"q=rust"}`))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("update status = %d, want 200", res.StatusCode)
	}
	if env["data"].(map[string]any)["query"] != "q=rust" {
		t.Errorf("updated query = %v, want q=rust", env["data"].(map[string]any)["query"])
	}

	// A different user cannot update the owner's set → 404.
	if res, _ := do(req(http.MethodPatch, fmt.Sprintf("%s/%d", path, id), otherCookie, `{"query":"x=1"}`)); res.StatusCode != http.StatusNotFound {
		t.Errorf("cross-user update status = %d, want 404", res.StatusCode)
	}
	// ...and cannot delete it either → 404.
	if res, _ := do(req(http.MethodDelete, fmt.Sprintf("%s/%d", path, id), otherCookie, "")); res.StatusCode != http.StatusNotFound {
		t.Errorf("cross-user delete status = %d, want 404", res.StatusCode)
	}

	// Owner deletes own set → 204, then it is gone.
	if res, _ := do(req(http.MethodDelete, fmt.Sprintf("%s/%d", path, id), ownerCookie, "")); res.StatusCode != http.StatusNoContent {
		t.Errorf("delete status = %d, want 204", res.StatusCode)
	}
	if res, _ := do(req(http.MethodDelete, fmt.Sprintf("%s/%d", path, id), ownerCookie, "")); res.StatusCode != http.StatusNotFound {
		t.Errorf("re-delete status = %d, want 404", res.StatusCode)
	}
}

func TestSavedSearchesCapEnforced(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('cap@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	// Seed 50 saved searches directly (the cap) so the 51st create is rejected.
	for i := 0; i < 50; i++ {
		if _, err := pool.Exec(ctx, `INSERT INTO saved_searches (user_id, name, query) VALUES ($1, $2, '')`, userID, fmt.Sprintf("set-%d", i)); err != nil {
			t.Fatalf("seed saved search %d: %v", i, err)
		}
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	cookie, _ := iss.Issue(userID)
	queries := db.New(pool)
	h := &API{pool: pool, queries: queries, issuer: iss, savedSearch: savedsearch.New(savedsearch.NewQueriesRepository(queries))}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/api/v1/me/searches", auth.RequireAuth(iss), h.CreateSavedSearch)

	rq := httptest.NewRequest(http.MethodPost, "/api/v1/me/searches", bytes.NewBufferString(`{"name":"one too many","query":""}`))
	rq.Header.Set("Content-Type", "application/json")
	rq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	res, err := app.Test(rq, -1)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if res.StatusCode != http.StatusConflict {
		t.Errorf("create past cap status = %d, want 409", res.StatusCode)
	}
}
