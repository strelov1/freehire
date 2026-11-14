//go:build integration

// End-to-end HTTP tests for the applicant inbox against a real Postgres: listing an
// employer's delivered applications with their stage counts, serving the kept CV, moving an
// application (the candidate's tracking follows and the candidate is emailed), and the
// scoping that keeps one employer out of another's applications.
// Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/application/appevent"
	"github.com/strelov1/freehire/internal/application/jobtracking"
	"github.com/strelov1/freehire/internal/identity/accounts"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/ingest/directapply"
	"github.com/strelov1/freehire/internal/ingest/employer"
	"github.com/strelov1/freehire/internal/ingest/moderation"
	"github.com/strelov1/freehire/internal/platform/db"
)

type recordingStatusMailer struct{ sent []directapply.StatusMail }

func (m *recordingStatusMailer) SendStatusUpdate(_ context.Context, mail directapply.StatusMail) error {
	m.sent = append(m.sent, mail)
	return nil
}

func TestEmployerApplicationsEndToEnd(t *testing.T) {
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
	employerID, otherID, candidateID := seed("hr@acme.test"), seed("hr@globex.test"), seed("c@example.test")
	for _, a := range []struct {
		id         int64
		slug, mail string
	}{{employerID, "acme", "hr@acme.test"}, {otherID, "globex", "hr@globex.test"}} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO company_accounts (user_id, company_slug, company_name, work_email, status, verified_at, email_confirmed_at)
			VALUES ($1, $2, initcap($2), $3, 'active', now(), now())`, a.id, a.slug, a.mail); err != nil {
			t.Fatalf("seed account %s: %v", a.slug, err)
		}
	}
	var jobID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO jobs (source, external_id, url, title, company, company_slug, public_slug, created_by)
		VALUES ('employer', 'freehire:acme-1', '', 'Platform Engineer', 'Acme', 'acme', 'acme-platform', $1) RETURNING id`,
		employerID).Scan(&jobID); err != nil {
		t.Fatalf("seed job: %v", err)
	}

	repo := directapply.NewQueriesRepository(queries)
	appID, recorded, err := repo.Record(ctx, directapply.Record{
		JobID: jobID, UserID: candidateID, CV: directapply.CVChoice{Resume: true}, Note: "Hello",
		Answers: []directapply.Answer{{ID: "q1", Label: "EU work permit?", Type: "boolean", Answer: "true"}},
		CVFile:  directapply.Attachment{Filename: "cv.pdf", ContentType: "application/pdf", Content: []byte("%PDF-1.7 cv")},
	}, 30)
	if err != nil || !recorded {
		t.Fatalf("Record = %v, %v", recorded, err)
	}
	if err := repo.MarkSent(ctx, appID); err != nil {
		t.Fatalf("MarkSent: %v", err)
	}
	tracking := jobtracking.New(jobtracking.NewQueriesRepository(queries, pool))
	if _, err := tracking.MarkApplied(ctx, candidateID, "acme-platform", appevent.SourceUser); err != nil {
		t.Fatalf("MarkApplied: %v", err)
	}

	accountsSvc := accounts.New(accounts.NewQueriesRepository(queries, pool), authHasher{})
	employerRepo := employer.NewQueriesRepository(queries, pool)
	moderationSvc := moderation.New(moderation.NewQueriesRepository(queries, pool, enrich.Version))
	employerSvc := employer.New(employerRepo, accountsSvc, nil, employerRepo, moderationSvc)
	notifier := &recordingStatusMailer{}
	h := newEmployerHandlers(queries, employerSvc)
	h.inbox = directapply.NewInbox(repo, employerSvc, notifier, directapply.NewApplicationMoves(queries, pool))

	iss := auth.NewIssuer("test-secret", time.Hour)
	employerCookie, _ := iss.Issue(employerID, testTokenVersion)
	otherCookie, _ := iss.Issue(otherID, testTokenVersion)
	candidateCookie, _ := iss.Issue(candidateID, testTokenVersion)

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	mw := middleware{cookie: auth.RequireAuth(iss, testVersions)}
	h.register(app.Group("/api/v1"), mw, newContract(contractHandlers{employerHandlers: h}))

	// reply is a finished response: its status, headers and the whole body.
	type reply struct {
		StatusCode int
		Header     http.Header
	}
	do := func(t *testing.T, method, path, cookie, body string) (reply, []byte) {
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
		return reply{StatusCode: resp.StatusCode, Header: resp.Header}, b
	}
	idPath := "/api/v1/employer/applications/" + strconv.FormatInt(appID, 10)

	t.Run("the employer lists the delivered application with stage counts", func(t *testing.T) {
		resp, body := do(t, fiber.MethodGet, "/api/v1/employer/applications?job=acme-platform", employerCookie, "")
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d (body %s)", resp.StatusCode, body)
		}
		var page struct {
			Data []struct {
				ID             int64  `json:"id"`
				CandidateEmail string `json:"candidate_email"`
				Stage          string `json:"stage"`
				HasCV          bool   `json:"has_cv"`
				Answers        []struct {
					Label  string `json:"label"`
					Answer string `json:"answer"`
				} `json:"answers"`
			} `json:"data"`
			Meta struct {
				Total       int64            `json:"total"`
				StageCounts map[string]int64 `json:"stage_counts"`
			} `json:"meta"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			t.Fatalf("decode: %v (%s)", err, body)
		}
		if len(page.Data) != 1 || page.Data[0].ID != appID || page.Data[0].CandidateEmail != "c@example.test" ||
			page.Data[0].Stage != "new" || !page.Data[0].HasCV || len(page.Data[0].Answers) != 1 {
			t.Errorf("data = %+v", page.Data)
		}
		if page.Meta.Total != 1 || page.Meta.StageCounts["new"] != 1 {
			t.Errorf("meta = %+v", page.Meta)
		}
	})

	t.Run("an unknown stage filter is refused, not widened", func(t *testing.T) {
		if resp, body := do(t, fiber.MethodGet, "/api/v1/employer/applications?stage=ghosted", employerCookie, ""); resp.StatusCode != fiber.StatusBadRequest {
			t.Errorf("status = %d (body %s), want 400", resp.StatusCode, body)
		}
	})

	t.Run("the employer opens the CV as it was sent", func(t *testing.T) {
		resp, body := do(t, fiber.MethodGet, idPath+"/cv", employerCookie, "")
		if resp.StatusCode != fiber.StatusOK || string(body) != "%PDF-1.7 cv" {
			t.Fatalf("status = %d, body %q", resp.StatusCode, body)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "application/pdf" {
			t.Errorf("content type = %q", ct)
		}
		if cd := resp.Header.Get("Content-Disposition"); !strings.HasPrefix(cd, "inline") || !strings.Contains(cd, "cv.pdf") {
			t.Errorf("content disposition = %q", cd)
		}
		if cc := resp.Header.Get("Cache-Control"); cc != "private, no-store" {
			t.Errorf("cache control = %q", cc)
		}
	})

	t.Run("another employer cannot see, open or move it", func(t *testing.T) {
		_, body := do(t, fiber.MethodGet, "/api/v1/employer/applications", otherCookie, "")
		if !strings.Contains(string(body), `"data":[]`) {
			t.Errorf("other employer's list = %s, want empty", body)
		}
		if resp, _ := do(t, fiber.MethodGet, idPath+"/cv", otherCookie, ""); resp.StatusCode != fiber.StatusNotFound {
			t.Errorf("other employer's CV status = %d, want 404", resp.StatusCode)
		}
		if resp, _ := do(t, fiber.MethodPost, idPath+"/stage", otherCookie, `{"stage":"rejected"}`); resp.StatusCode != fiber.StatusNotFound {
			t.Errorf("other employer's move status = %d, want 404", resp.StatusCode)
		}
	})

	t.Run("a user without an employer account is refused", func(t *testing.T) {
		if resp, _ := do(t, fiber.MethodGet, "/api/v1/employer/applications", candidateCookie, ""); resp.StatusCode != fiber.StatusNotFound {
			t.Errorf("status = %d, want 404 (no employer account)", resp.StatusCode)
		}
	})

	t.Run("a message with a stage that sends no letter is refused", func(t *testing.T) {
		resp, body := do(t, fiber.MethodPost, idPath+"/stage", employerCookie, `{"stage":"reviewing","message":"hi"}`)
		if resp.StatusCode != fiber.StatusBadRequest || !strings.Contains(string(body), "interview") {
			t.Errorf("status = %d (body %s), want 400 naming the stages that send a letter", resp.StatusCode, body)
		}
	})

	t.Run("an interview invitation moves the candidate's tracking and emails them", func(t *testing.T) {
		resp, body := do(t, fiber.MethodPost, idPath+"/stage", employerCookie, `{"stage":"interview","message":"Are you free on Tuesday?"}`)
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d (body %s)", resp.StatusCode, body)
		}
		var moved struct {
			Data struct {
				Application struct {
					Stage          string `json:"stage"`
					StageChangedAt string `json:"stage_changed_at"`
				} `json:"application"`
				CandidateNotified bool `json:"candidate_notified"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &moved); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if moved.Data.Application.Stage != "interview" || moved.Data.Application.StageChangedAt == "" || !moved.Data.CandidateNotified {
			t.Errorf("moved = %+v", moved.Data)
		}
		if len(notifier.sent) != 1 || notifier.sent[0].To != "c@example.test" || notifier.sent[0].ReplyTo != "hr@acme.test" ||
			notifier.sent[0].Message != "Are you free on Tuesday?" {
			t.Errorf("letters = %+v", notifier.sent)
		}
		var stage string
		if err := pool.QueryRow(ctx, `SELECT stage FROM applications WHERE user_id = $1 AND job_id = $2`, candidateID, jobID).Scan(&stage); err != nil || stage != "interview" {
			t.Errorf("candidate's stage = %q, %v; want interview", stage, err)
		}
	})

	t.Run("an unparseable id is a bad request", func(t *testing.T) {
		if resp, _ := do(t, fiber.MethodPost, "/api/v1/employer/applications/abc/stage", employerCookie, `{"stage":"new"}`); resp.StatusCode != fiber.StatusBadRequest {
			t.Errorf("status = %d, want 400", resp.StatusCode)
		}
	})
}

func TestEmployerApplicationsWithoutMail(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	h := newEmployerHandlers(queries, nil)
	iss := auth.NewIssuer("test-secret", time.Hour)
	cookie, _ := iss.Issue(1, testTokenVersion)
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	h.register(app.Group("/api/v1"), middleware{cookie: auth.RequireAuth(iss, testVersions)}, newContract(contractHandlers{employerHandlers: h}))
	r := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/api/v1/employer/applications", nil)
	r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	resp, err := app.Test(r)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusNotImplemented {
		t.Errorf("status = %d, want 501", resp.StatusCode)
	}
}
