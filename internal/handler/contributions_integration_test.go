//go:build integration

// Integration tests for the link-contribution HTTP flow against a real Postgres: an
// unauthenticated submit is 401, an unsupported host is 422, a novel supported link is 201
// and credits a point (surfaced on /auth/me), a resubmit is 409, and a link already in the
// catalogue is 409. Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/accounts"
	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/contribution"
	"github.com/strelov1/freehire/internal/db"
)

func TestContributionsEndToEnd(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('contrib@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	// A board already in the catalogue (a job on greenhouse board "beta"), to exercise the
	// "board already tracked" reject.
	if _, err := pool.Exec(ctx,
		`INSERT INTO jobs (source, external_id, url, title, public_slug)
		 VALUES ('greenhouse', 'beta:2', 'http://example.test', 'Taken', 'taken-beta-2')`); err != nil {
		t.Fatalf("seed job: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	cookie, _ := iss.Issue(userID)
	queries := db.New(pool)
	h := &API{
		pool:         pool,
		queries:      queries,
		issuer:       iss,
		contribution: contribution.New(contribution.NewQueriesRepository(queries, pool)),
		accounts:     accounts.New(accounts.NewQueriesRepository(queries, pool), authHasher{}),
	}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	keyAuth := auth.RequireAuthOrKey(iss, queries)
	app.Post("/api/v1/me/contributions", keyAuth, h.CreateContribution)
	app.Get("/api/v1/me/contributions", keyAuth, h.ListMyContributions)
	app.Get("/api/v1/auth/me", keyAuth, h.Me)

	submit := func(t *testing.T, url string, withCookie bool) *http.Response {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"url": url})
		r := httptest.NewRequest(fiber.MethodPost, "/api/v1/me/contributions", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if withCookie {
			r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		}
		resp, err := app.Test(r)
		if err != nil {
			t.Fatalf("submit: %v", err)
		}
		return resp
	}

	t.Run("anonymous submit is 401", func(t *testing.T) {
		if resp := submit(t, "https://job-boards.greenhouse.io/acme/jobs/456", false); resp.StatusCode != fiber.StatusUnauthorized {
			t.Errorf("status = %d, want 401", resp.StatusCode)
		}
	})

	t.Run("unsupported host is 422", func(t *testing.T) {
		if resp := submit(t, "https://example.com/careers/1", true); resp.StatusCode != fiber.StatusUnprocessableEntity {
			t.Errorf("status = %d, want 422", resp.StatusCode)
		}
	})

	t.Run("board already tracked is 409", func(t *testing.T) {
		// The "beta" board has a job → contributing any beta link (vacancy or listing) is rejected.
		if resp := submit(t, "https://job-boards.greenhouse.io/beta/jobs/2?utm=x", true); resp.StatusCode != fiber.StatusConflict {
			t.Errorf("status = %d, want 409", resp.StatusCode)
		}
	})

	t.Run("novel board (via vacancy URL) is 201 and credits a point", func(t *testing.T) {
		resp := submit(t, "https://jobs.ashbyhq.com/acme/a741b4e8-8799-4539-b1c2-78d69ff625e7?utm_source=tg", true)
		if resp.StatusCode != fiber.StatusCreated {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 201 (body %s)", resp.StatusCode, body)
		}
		var created struct {
			Data contributionResponse `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if created.Data.Source != "ashby" || created.Data.Board != "acme" {
			t.Errorf("created = %+v, want ashby/acme", created.Data)
		}

		// /auth/me reflects the awarded point.
		meReq := httptest.NewRequest(fiber.MethodGet, "/api/v1/auth/me", nil)
		meReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		meResp, err := app.Test(meReq)
		if err != nil {
			t.Fatalf("me: %v", err)
		}
		var me struct {
			Data struct {
				Points int `json:"points"`
			} `json:"data"`
		}
		if err := json.NewDecoder(meResp.Body).Decode(&me); err != nil {
			t.Fatalf("decode me: %v", err)
		}
		if me.Data.Points != 1 {
			t.Errorf("points = %d, want 1", me.Data.Points)
		}
	})

	t.Run("a second vacancy on the same board is 409", func(t *testing.T) {
		// A DIFFERENT vacancy on the same ashby "acme" board → same board → duplicate, no point.
		if resp := submit(t, "https://jobs.ashbyhq.com/acme/a1c86055-bca7-43de-8542-38c94347c693", true); resp.StatusCode != fiber.StatusConflict {
			t.Errorf("status = %d, want 409", resp.StatusCode)
		}
		// The bare board-listing URL for the same board is also a duplicate.
		if resp := submit(t, "https://jobs.ashbyhq.com/acme", true); resp.StatusCode != fiber.StatusConflict {
			t.Errorf("board-listing resubmit status = %d, want 409", resp.StatusCode)
		}
		// Still exactly one point — the rejected resubmits credited nothing.
		var points int
		if err := pool.QueryRow(ctx, `SELECT points FROM users WHERE id = $1`, userID).Scan(&points); err != nil {
			t.Fatalf("read points: %v", err)
		}
		if points != 1 {
			t.Errorf("points after duplicates = %d, want still 1", points)
		}
	})

	t.Run("my contributions lists the caller's own", func(t *testing.T) {
		r := httptest.NewRequest(fiber.MethodGet, "/api/v1/me/contributions", nil)
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		resp, err := app.Test(r)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		var list struct {
			Data []contributionResponse `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(list.Data) != 1 || list.Data[0].Board != "acme" || list.Data[0].Source != "ashby" {
			t.Errorf("list = %+v, want the one recorded ashby/acme board", list.Data)
		}
	})
}
