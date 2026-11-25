//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/ingest/employer"
	"github.com/strelov1/freehire/internal/platform/db"
)

// An employer lists the vacancies of its own active company, authored and collected, deletes
// only the ones it authored, and reads analytics for both.
func TestEmployerVacancyScope(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	user := func(email string) int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx, "INSERT INTO users (email) VALUES ($1) RETURNING id", email).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	owner, other := user("owner@scope.test"), user("owner@elsewhere.test")
	exec(`INSERT INTO company_accounts (user_id, company_slug, company_name, work_email, status)
		VALUES ($1, 'scope', 'Scope', 'owner@scope.test', 'active'), ($2, 'elsewhere', 'Elsewhere', 'owner@elsewhere.test', 'active')`, owner, other)
	exec(`INSERT INTO jobs (source, external_id, url, title, company, company_slug, public_slug, created_by, closed_at)
		VALUES ('employer', 'own', '', 'Own', 'Scope', 'scope', 'own-role', $1, NULL),
		       ('greenhouse', 'ats', 'https://ats.test/1', 'Collected', 'Scope', 'scope', 'ats-role', NULL, NULL),
		       ('greenhouse', 'ats-old', 'https://ats.test/2', 'Collected old', 'Scope', 'scope', 'ats-old-role', NULL, now()),
		       ('greenhouse', 'ats-dup', 'https://ats.test/3', 'Collected twin', 'Scope', 'scope', 'ats-dup-role', NULL, NULL),
		       ('greenhouse', 'else', 'https://else.test/1', 'Elsewhere role', 'Elsewhere', 'elsewhere', 'else-role', NULL, NULL)`, owner)
	exec(`UPDATE jobs SET duplicate_of_role = (SELECT id FROM jobs WHERE public_slug = 'ats-role') WHERE public_slug = 'ats-dup-role'`)

	// A fixed order: the vacancies were inserted in one statement and share a created_at.
	exec(`UPDATE jobs SET created_at = now() - interval '1 hour' WHERE public_slug = 'own-role'`)
	exec(`UPDATE jobs SET created_at = now() - interval '2 hours' WHERE public_slug = 'ats-role'`)

	issuer := auth.NewIssuer("test-secret", time.Hour)
	queries := db.New(pool)
	repo := employer.NewQueriesRepository(queries, pool)
	h := newEmployerHandlers(queries, employer.New(repo, nil, nil, repo, nil))
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	h.register(app.Group("/api/v1"), middleware{cookie: auth.RequireAuth(issuer, testVersions)},
		newContract(contractHandlers{employerHandlers: h}))
	call := func(method, path string, userID int64, want int) []byte {
		t.Helper()
		req := httptest.NewRequestWithContext(ctx, method, "/api/v1"+path, nil)
		token, err := issuer.Issue(userID, testTokenVersion)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Cookie", auth.CookieName+"="+token)
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		body, _ := io.ReadAll(res.Body)
		if res.StatusCode != want {
			t.Fatalf("%s %s returned %d, want %d: %s", method, path, res.StatusCode, want, body)
		}
		return body
	}
	slugs := func(userID int64) []string {
		t.Helper()
		var body struct {
			Data []struct {
				PublicSlug string `json:"public_slug"`
			} `json:"data"`
		}
		if err := json.Unmarshal(call(http.MethodGet, "/employer/jobs", userID, 200), &body); err != nil {
			t.Fatal(err)
		}
		out := make([]string, len(body.Data))
		for i, j := range body.Data {
			out[i] = j.PublicSlug
		}
		return out
	}
	equal := func(got []string, want ...string) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("list = %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("list = %v, want %v", got, want)
			}
		}
	}

	// Own company only, open collected ones with the authored ones, newest first; the closed
	// collected one and the duplicate are left out.
	equal(slugs(owner), "own-role", "ats-role")
	equal(slugs(other), "else-role")

	// A collected vacancy is read-only: no edit, close or delete.
	call(http.MethodDelete, "/employer/jobs/ats-role", owner, 404)
	call(http.MethodPost, "/employer/jobs/ats-role/close", owner, 404)
	// Nobody deletes another company's vacancy.
	call(http.MethodDelete, "/employer/jobs/own-role", other, 404)

	// Analytics and click statistics work for a collected vacancy of the caller's company only.
	call(http.MethodGet, "/employer/jobs/ats-role/analytics", owner, 200)
	call(http.MethodGet, "/employer/jobs/ats-role/outbound-clicks", owner, 200)
	call(http.MethodGet, "/employer/jobs/ats-role/analytics", other, 404)
	call(http.MethodGet, "/employer/jobs/ats-role/outbound-clicks", other, 404)

	// Deleting an authored vacancy is soft and final.
	call(http.MethodDelete, "/employer/jobs/own-role", owner, 200)
	equal(slugs(owner), "ats-role")
	if _, err := queries.GetJobBySlug(ctx, "own-role"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("public lookup of a deleted vacancy: err = %v, want ErrNoRows", err)
	}
	// The candidates' own history still reads it.
	if _, err := queries.GetJobBySlugForApplicant(ctx, "own-role"); err != nil {
		t.Fatalf("applicant lookup of a deleted vacancy: %v", err)
	}
	var reason string
	var queued bool
	if err := pool.QueryRow(ctx, `SELECT closed_reason, EXISTS (SELECT 1 FROM search_delete_outbox WHERE job_id = jobs.id)
		FROM jobs WHERE public_slug = 'own-role'`).Scan(&reason, &queued); err != nil {
		t.Fatalf("the row must stay for the applications that reference it: %v", err)
	}
	if reason != "employer_deleted" || !queued {
		t.Errorf("closed_reason = %q, queued for search removal = %v; want employer_deleted, true", reason, queued)
	}
	call(http.MethodPost, "/employer/jobs/own-role/reopen", owner, 404)
	call(http.MethodDelete, "/employer/jobs/own-role", owner, 404)
	call(http.MethodGet, "/employer/jobs/own-role/analytics", owner, 404)

	// A company that is not active sees nothing.
	exec("UPDATE company_accounts SET status = 'pending' WHERE user_id = $1", owner)
	call(http.MethodGet, "/employer/jobs", owner, 403)
	call(http.MethodGet, "/employer/jobs/ats-role/analytics", owner, 404)
}
