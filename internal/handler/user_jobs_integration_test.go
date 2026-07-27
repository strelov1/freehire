//go:build integration

// Integration tests for the save/unsave wire contract. The auth gates are unit
// tested; here a real Postgres exercises the DB-backed paths the unit tests
// cannot: the save upsert response shape and the "unsave with no interaction
// row" case, which must be a 200 zero-state, never an error. Run with:
// go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"encoding/json"
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

func TestSaveUnsaveEndpoints(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('saver@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO jobs (source, external_id, url, title, public_slug)
		 VALUES ('test', 'save-1', 'http://example.test', 'Go Dev', 'go-dev-acme-t35nijto')`); err != nil {
		t.Fatalf("seed job: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(userID, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	queries := db.New(pool)
	h := &trackingHandlers{tracking: jobtracking.New(jobtracking.NewQueriesRepository(queries, pool))}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/api/v1/jobs/:slug/save", auth.RequireAuth(iss, testVersions), h.SaveJob)
	app.Delete("/api/v1/jobs/:slug/save", auth.RequireAuth(iss, testVersions), h.UnsaveJob)

	type interaction struct {
		JobID     int64   `json:"job_id"`
		ViewedAt  *string `json:"viewed_at"`
		SavedAt   *string `json:"saved_at"`
		AppliedAt *string `json:"applied_at"`
	}
	do := func(t *testing.T, method, path string) interaction {
		t.Helper()
		req := httptest.NewRequest(method, path, nil)
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("%s %s: status = %d, want 200 (body %s)", method, path, resp.StatusCode, body)
		}
		var body struct {
			Data interaction `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return body.Data
	}

	t.Run("unsave with no interaction row is a 200 zero-state", func(t *testing.T) {
		got := do(t, fiber.MethodDelete, "/api/v1/jobs/go-dev-acme-t35nijto/save")
		if got.SavedAt != nil || got.ViewedAt != nil || got.AppliedAt != nil {
			t.Errorf("zero-state = %+v, want all timestamps null", got)
		}
		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM user_jobs").Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		if n != 0 {
			t.Errorf("rows = %d, want 0 (unsave must not create a row)", n)
		}
	})

	t.Run("save then unsave round-trips the saved state", func(t *testing.T) {
		saved := do(t, fiber.MethodPost, "/api/v1/jobs/go-dev-acme-t35nijto/save")
		if saved.SavedAt == nil || saved.ViewedAt == nil {
			t.Errorf("save = %+v, want saved_at and viewed_at set", saved)
		}
		unsaved := do(t, fiber.MethodDelete, "/api/v1/jobs/go-dev-acme-t35nijto/save")
		if unsaved.SavedAt != nil {
			t.Error("unsave left saved_at set")
		}
		if unsaved.ViewedAt == nil {
			t.Error("unsave lost viewed_at")
		}
	})

	t.Run("save on an unknown slug is 404", func(t *testing.T) {
		req := httptest.NewRequest(fiber.MethodPost, "/api/v1/jobs/no-such-job/save", nil)
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Test: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != fiber.StatusNotFound {
			t.Errorf("status = %d, want 404", resp.StatusCode)
		}
	})
}
