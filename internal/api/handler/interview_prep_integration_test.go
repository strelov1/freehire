//go:build integration

package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestPublicJobTools(t *testing.T) {
	pool := startPostgres(t)
	ctx := t.Context()
	queries := db.New(pool)
	bank := experience.NewStore(experience.NewQueriesRepository(queries))
	issuer := auth.NewIssuer("test-secret", time.Hour)

	account := func(email string) (int64, string) {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx, "INSERT INTO users (email) VALUES ($1) RETURNING id", email).Scan(&id); err != nil {
			t.Fatal(err)
		}
		token, err := issuer.Issue(id, testTokenVersion)
		if err != nil {
			t.Fatal(err)
		}
		return id, token
	}
	owner, ownerToken := account("prep-owner@example.test")
	other, otherToken := account("prep-other@example.test")
	_, emptyToken := account("prep-empty@example.test")
	for _, entry := range []struct {
		userID int64
		atom   experience.Atom
		author experience.Author
	}{
		{owner, experience.Atom{Claim: "Migrated payment services", Skills: []string{"go"}, Context: "Rolled out one service at a time"}, experience.AuthorCandidate},
		{owner, experience.Atom{Claim: "Invented Kubernetes rollout", Skills: []string{"go", "kubernetes"}}, experience.AuthorAgent},
		{owner, experience.Atom{Claim: "Designed a logo", Skills: []string{"figma"}}, experience.AuthorCandidate},
		{other, experience.Atom{Claim: "Maintained the Go compiler", Skills: []string{"go"}, Metrics: []string{"10% faster"}}, experience.AuthorCandidate},
	} {
		if _, err := bank.AddAtom(ctx, entry.userID, entry.atom, entry.author); err != nil {
			t.Fatal(err)
		}
	}

	for _, slug := range []string{"prep-open", "prep-private", "prep-closed", "prep-sparse"} {
		if _, err := queries.UpsertJob(ctx, db.UpsertJobParams{
			Source: "greenhouse", ExternalID: slug, PublicSlug: slug,
			Title: "Backend Engineer", Company: "Acme", Skills: []string{"go", "kubernetes"},
		}); err != nil {
			t.Fatal(err)
		}
	}
	for _, stmt := range []string{
		`UPDATE jobs SET requirements_derived = '[{"text":"Operate Kubernetes in production","priority":"required"}]' WHERE public_slug = 'prep-open'`,
		`UPDATE jobs SET is_private = true WHERE public_slug = 'prep-private'`,
		`UPDATE jobs SET closed_at = now() WHERE public_slug = 'prep-closed'`,
		`UPDATE jobs SET skills = '{}' WHERE public_slug = 'prep-sparse'`,
	} {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO applications (user_id, job_id, stage)
		SELECT $1, id, 'interview' FROM jobs WHERE public_slug = 'prep-closed'`, owner); err != nil {
		t.Fatal(err)
	}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	// Register the same routes as the server, with real signed cookies and owner-scoped DB reads.
	prep := &interviewPrepHandlers{jobs: &jobsHandlers{queries: queries}, bank: bank}
	contract := newContract(contractHandlers{jobsHandlers: prep.jobs, interviewPrepHandlers: prep})
	app.Get("/api/v1/jobs/:slug/skill-fit", contract)
	registerInterviewPrep(app.Group("/api/v1"), middleware{optionalCookie: auth.OptionalCookieAuth(issuer, queries)}, contract)

	get := func(slug, path, token string, want int) string {
		t.Helper()
		req := httptest.NewRequestWithContext(ctx, "GET", "/api/v1/jobs/"+slug+path, nil)
		if token != "" {
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		}
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		body, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != want {
			t.Fatalf("%s: got %d, want %d: %s", slug, res.StatusCode, want, body)
		}
		if strings.HasPrefix(path, "/interview-prep") && res.Header.Get("Cache-Control") != "private, no-store" {
			t.Errorf("unexpected Cache-Control: %q", res.Header.Get("Cache-Control"))
		}
		return string(body)
	}

	review := get("prep-open", "/skill-fit", "", 200)
	if !strings.Contains(review, `"skills":["go","kubernetes"]`) || !strings.Contains(review, "Operate Kubernetes in production") {
		t.Errorf("resume review did not expose the public job skills and requirements: %s", review)
	}
	for _, slug := range []string{"prep-private", "prep-closed", "prep-unknown"} {
		get(slug, "/skill-fit", ownerToken, 404)
	}

	request := func(slug, token string, want int) string {
		t.Helper()
		return get(slug, "/interview-prep?user_id=1", token, want)
	}
	guest := request("prep-open", "", 200)
	var payload struct {
		Data struct {
			Questions []struct{ Prompt, Topic string }
			Examples  json.RawMessage
		}
	}
	if err := json.Unmarshal([]byte(guest), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data.Questions) < 3 || len(payload.Data.Questions) > 5 ||
		payload.Data.Questions[0].Topic != "Operate Kubernetes in production" ||
		payload.Data.Questions[0].Prompt == "" || payload.Data.Examples != nil {
		t.Fatalf("unexpected guest sheet: %s", guest)
	}
	if invalid := request("prep-open", "invalid", 200); invalid != guest {
		t.Errorf("invalid cookie changed the public response: %s", invalid)
	}
	if own := request("prep-open", ownerToken, 200); !strings.Contains(own, "Rolled out one service at a time") ||
		!strings.Contains(own, "Migrated payment services") || strings.Contains(own, "Invented Kubernetes rollout") ||
		strings.Contains(own, "Designed a logo") || strings.Contains(own, "Maintained the Go compiler") {
		t.Errorf("unexpected owner evidence: %s", own)
	}
	if theirs := request("prep-open", otherToken, 200); !strings.Contains(theirs, "10% faster") ||
		strings.Contains(theirs, "Migrated payment services") {
		t.Errorf("unexpected second account evidence: %s", theirs)
	}
	if empty := request("prep-open", emptyToken, 200); !strings.Contains(empty, `"examples":[]`) {
		t.Errorf("missing empty state: %s", empty)
	}
	if sparse := request("prep-sparse", "", 200); !strings.Contains(sparse, "Backend Engineer") {
		t.Errorf("missing role-based question: %s", sparse)
	}
	request("prep-closed", ownerToken, 200)
	request("prep-closed", otherToken, 404)
	request("prep-closed", "", 404)
	request("prep-private", ownerToken, 404)
	request("prep-unknown", "", 404)
	if _, err := pool.Exec(ctx, "UPDATE users SET token_version = token_version + 1 WHERE id = $1", owner); err != nil {
		t.Fatal(err)
	}
	if revoked := request("prep-open", ownerToken, 200); revoked != guest {
		t.Errorf("revoked session returned more than the public sheet: %s", revoked)
	}
	request("prep-closed", ownerToken, 404)
}
