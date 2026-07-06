//go:build integration

// Integration tests for the dismiss/undismiss wire contract. A real Postgres
// exercises the DB-backed paths the unit tests cannot: the dismiss upsert
// response shape and idempotency, the "undismiss with no interaction row" 200
// zero-state, the API-key auth path, and the unauth/404 gates. Run with:
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

func TestDismissUndismissEndpoints(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('dismisser@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO jobs (source, external_id, url, title, public_slug)
		 VALUES ('test', 'dismiss-1', 'http://example.test', 'Go Dev', 'go-dev-acme-t35nijto')`); err != nil {
		t.Fatalf("seed job: %v", err)
	}

	// Seed an API key for this user to exercise the Bearer path.
	token, hash, prefix, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO api_keys (user_id, name, token_hash, token_prefix) VALUES ($1, 'ci', $2, $3)`,
		userID, hash, prefix); err != nil {
		t.Fatalf("seed api key: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	cookie, err := iss.Issue(userID)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	queries := db.New(pool)
	h := &API{pool: pool, queries: queries, issuer: iss, tracking: jobtracking.New(jobtracking.NewQueriesRepository(queries))}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	keyAuth := auth.RequireAuthOrKey(iss, queries)
	app.Post("/api/v1/jobs/:slug/dismiss", keyAuth, h.DismissJob)
	app.Delete("/api/v1/jobs/:slug/dismiss", keyAuth, h.UndismissJob)

	type interaction struct {
		JobID       int64   `json:"job_id"`
		ViewedAt    *string `json:"viewed_at"`
		SavedAt     *string `json:"saved_at"`
		DismissedAt *string `json:"dismissed_at"`
	}
	const path = "/api/v1/jobs/go-dev-acme-t35nijto/dismiss"

	do := func(t *testing.T, req *http.Request) interaction {
		t.Helper()
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 200 (body %s)", resp.StatusCode, body)
		}
		var body struct {
			Data interaction `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return body.Data
	}
	cookieReq := func(method, reqPath string) *http.Request {
		r := httptest.NewRequest(method, reqPath, nil)
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		return r
	}

	t.Run("undismiss with no interaction row is a 200 zero-state", func(t *testing.T) {
		got := do(t, cookieReq(fiber.MethodDelete, path))
		if got.DismissedAt != nil || got.ViewedAt != nil || got.SavedAt != nil {
			t.Errorf("zero-state = %+v, want all timestamps null", got)
		}
		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM user_jobs").Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		if n != 0 {
			t.Errorf("rows = %d, want 0 (undismiss must not create a row)", n)
		}
	})

	t.Run("dismiss sets dismissed_at and is idempotent", func(t *testing.T) {
		first := do(t, cookieReq(fiber.MethodPost, path))
		if first.DismissedAt == nil || first.ViewedAt == nil {
			t.Errorf("dismiss = %+v, want dismissed_at and viewed_at set", first)
		}
		second := do(t, cookieReq(fiber.MethodPost, path))
		if second.DismissedAt == nil {
			t.Error("second dismiss lost dismissed_at")
		}
		var n int
		if err := pool.QueryRow(ctx,
			"SELECT count(*) FROM user_jobs WHERE user_id=$1", userID).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		if n != 1 {
			t.Errorf("rows = %d, want 1 (idempotent upsert, no duplicate)", n)
		}
	})

	t.Run("undismiss clears dismissed_at, keeps history", func(t *testing.T) {
		got := do(t, cookieReq(fiber.MethodDelete, path))
		if got.DismissedAt != nil {
			t.Error("undismiss left dismissed_at set")
		}
		if got.ViewedAt == nil {
			t.Error("undismiss lost viewed_at")
		}
	})

	t.Run("dismiss authenticated by an API key", func(t *testing.T) {
		req := httptest.NewRequest(fiber.MethodPost, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		got := do(t, req)
		if got.DismissedAt == nil {
			t.Error("key-authenticated dismiss did not set dismissed_at")
		}
	})

	t.Run("dismiss without credentials is 401", func(t *testing.T) {
		resp, err := app.Test(httptest.NewRequest(fiber.MethodPost, path, nil))
		if err != nil {
			t.Fatalf("Test: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != fiber.StatusUnauthorized {
			t.Errorf("status = %d, want 401", resp.StatusCode)
		}
	})

	t.Run("dismiss on an unknown slug is 404", func(t *testing.T) {
		resp, err := app.Test(cookieReq(fiber.MethodPost, "/api/v1/jobs/no-such-job/dismiss"))
		if err != nil {
			t.Fatalf("Test: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != fiber.StatusNotFound {
			t.Errorf("status = %d, want 404", resp.StatusCode)
		}
	})
}

// TestExcludedJobIDs covers the swipe deck's exclusion query: it returns every
// job the user has interacted with — saved, dismissed, AND merely-viewed — so a
// card is shown at most once, and respects the cap. The deck feeds these into an
// `id NOT IN [...]` search filter.
func TestExcludedJobIDs(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)

	var userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('excl@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	seedJob := func(ext string) int64 {
		var id int64
		if err := pool.QueryRow(ctx,
			`INSERT INTO jobs (source, external_id, url, title, public_slug)
			 VALUES ('test', $1, 'http://example.test', 'Job', $1) RETURNING id`, ext).Scan(&id); err != nil {
			t.Fatalf("seed job %s: %v", ext, err)
		}
		return id
	}
	savedID, dismissedID, viewedID := seedJob("saved"), seedJob("dismissed"), seedJob("viewed")

	if _, err := queries.SaveJob(ctx, db.SaveJobParams{UserID: userID, JobID: savedID}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := queries.DismissJob(ctx, db.DismissJobParams{UserID: userID, JobID: dismissedID}); err != nil {
		t.Fatalf("dismiss: %v", err)
	}
	if _, err := queries.RecordJobView(ctx, db.RecordJobViewParams{UserID: userID, JobID: viewedID}); err != nil {
		t.Fatalf("view: %v", err)
	}

	ids, err := queries.ExcludedJobIDs(ctx, db.ExcludedJobIDsParams{UserID: userID, Limit: 1000})
	if err != nil {
		t.Fatalf("ExcludedJobIDs: %v", err)
	}
	got := map[int64]bool{}
	for _, id := range ids {
		got[id] = true
	}
	if !got[savedID] || !got[dismissedID] || !got[viewedID] {
		t.Errorf("excluded = %v, want it to contain saved (%d), dismissed (%d), and viewed (%d)",
			ids, savedID, dismissedID, viewedID)
	}
	if len(ids) != 3 {
		t.Errorf("len(excluded) = %d, want 3", len(ids))
	}
}
