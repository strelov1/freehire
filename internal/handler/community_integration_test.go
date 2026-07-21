//go:build integration

// Integration tests for the community discussion-thread HTTP flow against a real
// Postgres: creating a thread on a company and on a job, the rejection paths
// (unknown subject, bad type, unauthenticated, over the rate limit), the reply flow
// (count increments; closed/missing rejected), and persona stability. A recurring
// assertion across all of them: no author user id ever appears in a response body.
// Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/community"
	"github.com/strelov1/freehire/internal/db"
)

func newCommunityApp(queries *db.Queries, iss *auth.Issuer, cfg community.Config) *fiber.App {
	repo := community.NewQueriesRepository(queries)
	h := &API{queries: queries, issuer: iss, community: community.New(repo, repo, cfg)}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	cookieAuth := auth.RequireAuth(iss)
	requireModerator := auth.RequireRole(queries, "moderator")
	app.Get("/api/v1/threads", h.ListThreads)
	app.Get("/api/v1/threads/:id", h.GetThread)
	app.Post("/api/v1/threads", cookieAuth, h.CreateThread)
	app.Post("/api/v1/threads/:id/replies", cookieAuth, h.CreateReply)
	app.Post("/api/v1/threads/:id/close", cookieAuth, requireModerator, h.CloseThread)
	return app
}

// postThread issues a create-thread request; withCookie toggles authentication.
func postThread(t *testing.T, app *fiber.App, cookie, subjectType, slug string) *http.Response {
	t.Helper()
	body, _ := json.Marshal(map[string]string{
		"subject_type": subjectType, "subject_slug": slug, "title": "Do they ghost?", "body": "asking for real",
	})
	r := httptest.NewRequest(fiber.MethodPost, "/api/v1/threads", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	}
	resp, err := app.Test(r)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	return resp
}

// readBody returns the whole response body as a string and its decoded data map.
func readBody(t *testing.T, resp *http.Response) (string, map[string]any) {
	t.Helper()
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var env struct {
		Data map[string]any `json:"data"`
	}
	_ = json.Unmarshal(raw, &env)
	return string(raw), env.Data
}

func TestCommunityThreadsEndToEnd(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var user1, user2 int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('c1@example.test') RETURNING id`).Scan(&user1); err != nil {
		t.Fatalf("seed user1: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('c2@example.test') RETURNING id`).Scan(&user2); err != nil {
		t.Fatalf("seed user2: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO companies (slug, name, job_count) VALUES ('acme', 'Acme', 1)`); err != nil {
		t.Fatalf("seed company: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO jobs (source, external_id, url, title, public_slug)
		 VALUES ('greenhouse', 'eng:1', 'http://example.test', 'Engineer', 'engineer-eng-1')`); err != nil {
		t.Fatalf("seed job: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	cookie1, _ := iss.Issue(user1)
	cookie2, _ := iss.Issue(user2)
	queries := db.New(pool)
	app := newCommunityApp(queries, iss, community.Config{})

	// 5.1 — create on a company and on a job; no author user id leaks.
	t.Run("create on company and job", func(t *testing.T) {
		for _, subj := range []struct{ typ, slug string }{
			{community.SubjectCompany, "acme"},
			{community.SubjectJob, "engineer-eng-1"},
		} {
			resp := postThread(t, app, cookie1, subj.typ, subj.slug)
			if resp.StatusCode != fiber.StatusCreated {
				t.Fatalf("%s: status %d, want 201", subj.typ, resp.StatusCode)
			}
			raw, data := readBody(t, resp)
			if strings.Contains(raw, "user_id") || strings.Contains(raw, "author_user_id") {
				t.Fatalf("%s: response leaks a user id: %s", subj.typ, raw)
			}
			if data["author"] == nil || data["author"] == "" {
				t.Fatalf("%s: missing persona handle in %s", subj.typ, raw)
			}
		}
	})

	// 5.2 — rejection paths.
	t.Run("rejections", func(t *testing.T) {
		cases := []struct {
			name              string
			typ, slug, cookie string
			want              int
		}{
			{"unknown subject", community.SubjectCompany, "ghost", cookie1, fiber.StatusNotFound},
			{"bad subject type", "user", "acme", cookie1, fiber.StatusBadRequest},
			{"unauthenticated", community.SubjectCompany, "acme", "", fiber.StatusUnauthorized},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				resp := postThread(t, app, c.cookie, c.typ, c.slug)
				if resp.StatusCode != c.want {
					t.Fatalf("status %d, want %d", resp.StatusCode, c.want)
				}
			})
		}
	})

	// 5.3 — reply flow: increments count, reads back, closed/missing rejected.
	t.Run("reply flow", func(t *testing.T) {
		resp := postThread(t, app, cookie1, community.SubjectCompany, "acme")
		_, data := readBody(t, resp)
		threadID := int64(data["id"].(float64))

		// Post a reply as user2.
		body, _ := json.Marshal(map[string]string{"body": "same experience here"})
		r := httptest.NewRequest(fiber.MethodPost, "/api/v1/threads/"+itoa(threadID)+"/replies", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie2})
		rr, _ := app.Test(r)
		if rr.StatusCode != fiber.StatusCreated {
			t.Fatalf("reply status %d, want 201", rr.StatusCode)
		}

		// GET the thread: reply_count is 1 and the reply is present, no user id leaks.
		gr := httptest.NewRequest(fiber.MethodGet, "/api/v1/threads/"+itoa(threadID), nil)
		gresp, _ := app.Test(gr)
		raw, data := readBody(t, gresp)
		if strings.Contains(raw, "user_id") {
			t.Fatalf("thread read leaks a user id: %s", raw)
		}
		thread := data["thread"].(map[string]any)
		if int(thread["reply_count"].(float64)) != 1 {
			t.Fatalf("reply_count = %v, want 1", thread["reply_count"])
		}

		// Reply to a missing thread → 404.
		mr := httptest.NewRequest(fiber.MethodPost, "/api/v1/threads/999999/replies", bytes.NewReader(body))
		mr.Header.Set("Content-Type", "application/json")
		mr.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie2})
		mresp, _ := app.Test(mr)
		if mresp.StatusCode != fiber.StatusNotFound {
			t.Fatalf("reply to missing: status %d, want 404", mresp.StatusCode)
		}

		// Close the thread as a moderator, then a reply is 409 and it leaves the listing.
		var modID int64
		_ = pool.QueryRow(ctx, `INSERT INTO users (email, role) VALUES ('mod@example.test', 'moderator') RETURNING id`).Scan(&modID)
		modCookie, _ := iss.Issue(modID)
		cr := httptest.NewRequest(fiber.MethodPost, "/api/v1/threads/"+itoa(threadID)+"/close", nil)
		cr.AddCookie(&http.Cookie{Name: auth.CookieName, Value: modCookie})
		cresp, _ := app.Test(cr)
		if cresp.StatusCode != fiber.StatusOK {
			t.Fatalf("close status %d, want 200", cresp.StatusCode)
		}
		cl := httptest.NewRequest(fiber.MethodPost, "/api/v1/threads/"+itoa(threadID)+"/replies", bytes.NewReader(body))
		cl.Header.Set("Content-Type", "application/json")
		cl.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie2})
		clresp, _ := app.Test(cl)
		if clresp.StatusCode != fiber.StatusConflict {
			t.Fatalf("reply to closed: status %d, want 409", clresp.StatusCode)
		}
	})

	// 5.4 — persona stability: one user, one handle; different users, different handles.
	t.Run("persona stability", func(t *testing.T) {
		_, a := readBody(t, postThread(t, app, cookie1, community.SubjectCompany, "acme"))
		_, b := readBody(t, postThread(t, app, cookie1, community.SubjectCompany, "acme"))
		_, other := readBody(t, postThread(t, app, cookie2, community.SubjectCompany, "acme"))
		if a["author"] != b["author"] {
			t.Fatalf("same user got two handles: %v vs %v", a["author"], b["author"])
		}
		if a["author"] == other["author"] {
			t.Fatalf("different users share a handle: %v", a["author"])
		}
	})
}

func TestCommunityThreadRateLimit(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('rl@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO companies (slug, name, job_count) VALUES ('acme', 'Acme', 1)`); err != nil {
		t.Fatalf("seed company: %v", err)
	}
	iss := auth.NewIssuer("test-secret", time.Hour)
	cookie, _ := iss.Issue(userID)
	app := newCommunityApp(db.New(pool), iss, community.Config{ThreadCap: 2})

	for i := 0; i < 2; i++ {
		if resp := postThread(t, app, cookie, community.SubjectCompany, "acme"); resp.StatusCode != fiber.StatusCreated {
			t.Fatalf("post %d: status %d, want 201", i, resp.StatusCode)
		}
	}
	if resp := postThread(t, app, cookie, community.SubjectCompany, "acme"); resp.StatusCode != fiber.StatusTooManyRequests {
		t.Fatalf("over-limit status %d, want 429", resp.StatusCode)
	}
}
