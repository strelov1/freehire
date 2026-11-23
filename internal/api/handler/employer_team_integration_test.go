//go:build integration

// End-to-end HTTP tests for the company team against a real Postgres: the holder invites a
// colleague, the colleague previews and follows the link, then acts for the company — its
// vacancies and the applications to them — until removed.
// Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/identity/accounts"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/ingest/directapply"
	"github.com/strelov1/freehire/internal/ingest/employer"
	"github.com/strelov1/freehire/internal/ingest/moderation"
	"github.com/strelov1/freehire/internal/platform/db"
)

type recordingInviteMailer struct{ sent []employer.TeamInvitationMail }

func (m *recordingInviteMailer) SendTeamInvitation(_ context.Context, inv employer.TeamInvitationMail) error {
	m.sent = append(m.sent, inv)
	return nil
}

func TestEmployerTeamEndToEnd(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)

	seed := func(email string) int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx, `INSERT INTO users (email, email_verified) VALUES ($1, true) RETURNING id`, email).Scan(&id); err != nil {
			t.Fatalf("seed %s: %v", email, err)
		}
		return id
	}
	holderID, colleagueID, candidateID := seed("hr@acme.test"), seed("dev@acme.test"), seed("c@example.test")
	if _, err := pool.Exec(ctx, `
		INSERT INTO company_accounts (user_id, company_slug, company_name, work_email, status, verified_at, email_confirmed_at)
		VALUES ($1, 'acme', 'Acme', 'hr@acme.test', 'active', now(), now())`, holderID); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	var jobID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO jobs (source, external_id, url, title, company, company_slug, public_slug, created_by)
		VALUES ('employer', 'freehire:acme-1', '', 'Platform Engineer', 'Acme', 'acme', 'acme-platform', $1) RETURNING id`,
		holderID).Scan(&jobID); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	repo := directapply.NewQueriesRepository(queries)
	appID, _, err := repo.Record(ctx, directapply.Record{
		JobID: jobID, UserID: candidateID, CV: directapply.CVChoice{Resume: true},
		CVFile: directapply.Attachment{Filename: "cv.pdf", ContentType: "application/pdf", Content: []byte("%PDF")},
	}, 30)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := repo.MarkSent(ctx, appID); err != nil {
		t.Fatalf("MarkSent: %v", err)
	}

	accountsSvc := accounts.New(accounts.NewQueriesRepository(queries, pool), authHasher{})
	employerRepo := employer.NewQueriesRepository(queries, pool)
	moderationSvc := moderation.New(moderation.NewQueriesRepository(queries, pool, enrich.Version))
	invites := &recordingInviteMailer{}
	employerSvc := employer.New(employerRepo, accountsSvc, nil, employerRepo, moderationSvc).
		WithTeam(employerRepo, invites, "https://freehire.test/my/employer/join")
	h := newEmployerHandlers(queries, employerSvc)
	h.inbox = directapply.NewInbox(repo, employerSvc, &recordingStatusMailer{}, directapply.NewApplicationMoves(queries, pool))

	iss := auth.NewIssuer("test-secret", time.Hour)
	holder, _ := iss.Issue(holderID, testTokenVersion)
	colleague, _ := iss.Issue(colleagueID, testTokenVersion)
	candidate, _ := iss.Issue(candidateID, testTokenVersion)

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	h.register(app.Group("/api/v1"), middleware{cookie: auth.RequireAuth(iss, testVersions)}, newContract(contractHandlers{employerHandlers: h}))

	do := func(t *testing.T, method, path, cookie, body string) (int, []byte) {
		t.Helper()
		r := httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		resp, err := app.Test(r)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, b
	}

	// The holder invites a colleague at their work address.
	status, body := do(t, fiber.MethodPost, "/api/v1/employer/team/invitations", holder, `{"email":"dev@acme.test"}`)
	if status != fiber.StatusCreated || len(invites.sent) != 1 {
		t.Fatalf("invite status = %d (body %s), letters %d", status, body, len(invites.sent))
	}
	link, err := url.Parse(invites.sent[0].Link)
	if err != nil {
		t.Fatalf("parse link: %v", err)
	}
	token := link.Query().Get("token")
	tokenBody := `{"token":"` + token + `"}`

	if status, body := do(t, fiber.MethodPost, "/api/v1/employer/team/invitation-preview", colleague, tokenBody); status != fiber.StatusOK ||
		!strings.Contains(string(body), `"company_name":"Acme"`) || !strings.Contains(string(body), `"email":"dev@acme.test"`) {
		t.Fatalf("preview = %d %s", status, body)
	}
	if status, _ := do(t, fiber.MethodPost, "/api/v1/employer/team/invitation-preview", colleague, `{"token":"nope"}`); status != fiber.StatusNotFound {
		t.Errorf("preview of a dead link = %d, want 404", status)
	}

	// A mismatched or unverified account cannot consume the token.
	if status, body := do(t, fiber.MethodPost, "/api/v1/employer/team/join", candidate, tokenBody); status != fiber.StatusForbidden ||
		!strings.Contains(string(body), "confirm and use the invited email to join") {
		t.Fatalf("joining with another email = %d %s", status, body)
	}
	if _, err := pool.Exec(ctx, "UPDATE users SET email_verified = false WHERE id = $1", colleagueID); err != nil {
		t.Fatal(err)
	}
	if status, body := do(t, fiber.MethodPost, "/api/v1/employer/team/join", colleague, tokenBody); status != fiber.StatusForbidden {
		t.Fatalf("joining with unverified address = %d %s", status, body)
	}
	if _, err := pool.Exec(ctx, "UPDATE users SET email_verified = true WHERE id = $1", colleagueID); err != nil {
		t.Fatal(err)
	}
	status, body = do(t, fiber.MethodPost, "/api/v1/employer/team/join", colleague, tokenBody)
	if status != fiber.StatusOK || !strings.Contains(string(body), `"member":true`) || !strings.Contains(string(body), `"role":"recruiter"`) || !strings.Contains(string(body), `"work_email":"dev@acme.test"`) {
		t.Fatalf("join = %d %s", status, body)
	}
	if status, _ := do(t, fiber.MethodPost, "/api/v1/employer/team/join", colleague, tokenBody); status != fiber.StatusConflict {
		t.Errorf("joining again = %d, want 409", status)
	}

	// The colleague now acts for the company: its vacancies and the applications to them.
	if status, body := do(t, fiber.MethodGet, "/api/v1/employer/jobs", colleague, ""); status != fiber.StatusOK || !strings.Contains(string(body), "acme-platform") {
		t.Errorf("colleague's vacancies = %d %s", status, body)
	}
	if status, body := do(t, fiber.MethodGet, "/api/v1/employer/applications", colleague, ""); status != fiber.StatusOK || !strings.Contains(string(body), `"id":`+strconv.FormatInt(appID, 10)) {
		t.Errorf("colleague's applications = %d %s", status, body)
	}

	// The holder sees both in the team; the holder cannot be removed.
	status, body = do(t, fiber.MethodGet, "/api/v1/employer/team", holder, "")
	var team struct {
		Data struct {
			Members []struct {
				UserID int64 `json:"user_id"`
				Holder bool  `json:"holder"`
			} `json:"members"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &team); err != nil || status != fiber.StatusOK || len(team.Data.Members) != 2 {
		t.Fatalf("team = %d %s (%v)", status, body, err)
	}
	for _, check := range []struct{ method, path, body string }{
		{fiber.MethodDelete, "/api/v1/employer/team/members/" + strconv.FormatInt(holderID, 10), ""},
		{fiber.MethodPost, "/api/v1/employer/team/invitations", `{"email":"third@acme.test"}`},
		{fiber.MethodPatch, "/api/v1/employer/company", `{"tagline":"restricted"}`},
		{fiber.MethodPost, "/api/v1/employer/jobs/acme-platform/close", ""},
	} {
		if status, body := do(t, check.method, check.path, colleague, check.body); status != fiber.StatusForbidden {
			t.Errorf("recruiter %s %s = %d %s", check.method, check.path, status, body)
		}
	}
	appPath := "/api/v1/employer/applications/" + strconv.FormatInt(appID, 10)
	if status, body := do(t, fiber.MethodPatch, appPath+"/workspace", colleague, `{"employer_notes":"private recruiter note","mark_viewed":true}`); status != fiber.StatusOK || !strings.Contains(string(body), "private recruiter note") {
		t.Errorf("recruiter saves workspace = %d %s", status, body)
	}
	if status, body := do(t, fiber.MethodGet, appPath+"/cv", colleague, ""); status != fiber.StatusOK || string(body) != "%PDF" {
		t.Errorf("recruiter reads original CV = %d %s", status, body)
	}
	if status, body := do(t, fiber.MethodGet, "/api/v1/employer/activity", colleague, ""); status != fiber.StatusOK ||
		!strings.Contains(string(body), "notes_changed") || strings.Contains(string(body), "member_joined") {
		t.Errorf("recruiter audit = %d %s", status, body)
	}

	// Removed, the colleague no longer reaches the company.
	if status, body := do(t, fiber.MethodDelete, "/api/v1/employer/team/members/"+strconv.FormatInt(colleagueID, 10), holder, ""); status != fiber.StatusOK {
		t.Fatalf("remove = %d %s", status, body)
	}
	if status, _ := do(t, fiber.MethodGet, "/api/v1/employer/applications", colleague, ""); status != fiber.StatusNotFound {
		t.Errorf("a removed colleague's applications = %d, want 404 (no account)", status)
	}
	// The link was single-use: now that the colleague acts for no company, it does not let
	// them back in.
	if status, body := do(t, fiber.MethodPost, "/api/v1/employer/team/join", colleague, tokenBody); status != fiber.StatusNotFound {
		t.Errorf("rejoining with the used link = %d %s, want 404", status, body)
	}
	if status, _ := do(t, fiber.MethodGet, "/api/v1/employer/applications", colleague, ""); status != fiber.StatusNotFound {
		t.Errorf("applications after the refused rejoin = %d, want 404", status)
	}
	if status, body := do(t, fiber.MethodGet, "/api/v1/employer/activity", holder, ""); status != fiber.StatusOK ||
		!strings.Contains(string(body), "member_removed") || !strings.Contains(string(body), "member_joined") ||
		!strings.Contains(string(body), "invited") || strings.Contains(string(body), "private recruiter note") {
		t.Errorf("holder audit = %d %s", status, body)
	}

}
