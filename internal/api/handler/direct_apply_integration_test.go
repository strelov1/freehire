//go:build integration

// End-to-end HTTP test for applying on freehire against a real Postgres: an employer
// publishes a vacancy that takes applications here, a candidate reads its form with their
// prefill, applies, and the employer receives one email; the vacancy lands in the
// candidate's tracking, a second application is refused, and an unverified user is turned
// away. The CV renderer and the mail transport are fakes; everything else is real.
// Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/application/jobtracking"
	"github.com/strelov1/freehire/internal/candidate/answerbank"
	"github.com/strelov1/freehire/internal/identity/accounts"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/ingest/applyform"
	"github.com/strelov1/freehire/internal/ingest/directapply"
	"github.com/strelov1/freehire/internal/ingest/employer"
	"github.com/strelov1/freehire/internal/ingest/moderation"
	"github.com/strelov1/freehire/internal/platform/db"
)

// fakeApplicationCVs stands in for the CV renderer and the résumé store: the owner has an
// uploaded CV and one CV tailored for the job under test.
type fakeApplicationCVs struct {
	owner int64
	jobID *int64
	cvID  *string
}

func (f fakeApplicationCVs) Attachment(_ context.Context, userID, jobID int64, choice directapply.CVChoice) (directapply.Attachment, error) {
	if userID != f.owner {
		return directapply.Attachment{}, directapply.ErrNotFound
	}
	if !choice.Resume && (jobID != *f.jobID || choice.CVID.String() != *f.cvID) {
		return directapply.Attachment{}, directapply.ErrNotFound
	}
	return directapply.Attachment{Filename: "cv.pdf", ContentType: "application/pdf", Content: []byte("%PDF-1.7")}, nil
}

func (f fakeApplicationCVs) Options(_ context.Context, userID, jobID int64) (directapply.CVOptions, error) {
	o := directapply.CVOptions{HasResume: userID == f.owner}
	if userID == f.owner && jobID == *f.jobID {
		o.Tailored = &directapply.TailoredCV{ID: uuid.MustParse(*f.cvID), Title: "Tailored"}
	}
	return o, nil
}

type recordingApplicationMailer struct {
	sent []directapply.Mail
	fail bool
}

func (m *recordingApplicationMailer) SendApplication(_ context.Context, mail directapply.Mail) error {
	if m.fail {
		return errors.New("ses: throttled sending to hr@acme.test")
	}
	m.sent = append(m.sent, mail)
	return nil
}

func TestDirectApplyEndToEnd(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)

	seed := func(email string, verified bool) int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx, `INSERT INTO users (email, email_verified) VALUES ($1, $2) RETURNING id`, email, verified).Scan(&id); err != nil {
			t.Fatalf("seed %s: %v", email, err)
		}
		return id
	}
	employerID := seed("founder@acme.test", true)
	candidateID := seed("c@example.test", true)
	unverifiedID := seed("u@example.test", false)
	if _, err := pool.Exec(ctx, `INSERT INTO companies (slug, name, company_info) VALUES ('acme', 'Acme', '{"website":"acme.test"}')`); err != nil {
		t.Fatalf("seed company: %v", err)
	}

	accountsSvc := accounts.New(accounts.NewQueriesRepository(queries, pool), authHasher{})
	accountsSvc.WithCodes(accounts.NewQueriesCodeStore(queries, pool), nil)
	claimMailer := newFakeClaimMailer()
	employerRepo := employer.NewQueriesRepository(queries, pool)
	employerSvc := employer.New(employerRepo, accountsSvc, claimMailer, employerRepo,
		moderation.New(moderation.NewQueriesRepository(queries, pool, enrich.Version)))
	if _, err := employerSvc.Claim(ctx, employerID, "Acme", "founder@acme.test"); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if _, err := employerSvc.ConfirmClaim(ctx, employerID, claimMailer.codes["founder@acme.test"]); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	vacancy, _, err := employerSvc.CreateVacancy(ctx, employerID, employer.VacancyInput{Description: "Build things.", ApplyMode: employer.ApplyFreehire,
		Title: "Platform Engineer",
		Questions: []applyform.EmployerQuestion{
			{Label: "Do you hold an EU work permit?", Type: applyform.TypeBoolean, Required: true},
			{Label: "Anything else?", Type: applyform.TypeText},
		},
	})
	if err != nil {
		t.Fatalf("create vacancy: %v", err)
	}
	slug := vacancy.Fields().PublicSlug

	bank := answerbank.NewStore(answerbank.NewQueriesRepository(queries))
	if err := bank.Save(ctx, candidateID, "Do you hold an EU work permit?", "Yes", answerbank.AuthorCandidate); err != nil {
		t.Fatalf("seed bank: %v", err)
	}
	// The candidate's CV tailored for this vacancy (cvs.job_id), the one the dialog attaches.
	jobID := vacancy.Fields().ID
	var cvID string
	if err := pool.QueryRow(ctx, `INSERT INTO cvs (user_id, title, data, job_id) VALUES ($1, 'Tailored', '{}', $2) RETURNING id::text`, candidateID, jobID).Scan(&cvID); err != nil {
		t.Fatalf("seed cv: %v", err)
	}
	mailer := &recordingApplicationMailer{}
	tracking := jobtracking.New(jobtracking.NewQueriesRepository(queries, pool))
	h := &directApplyHandlers{svc: directapply.New(directapply.NewQueriesRepository(queries), employerSvc,
		fakeApplicationCVs{owner: candidateID, jobID: &jobID, cvID: &cvID}, mailer, directApplyTracker{tracking: tracking}, bank, 30)}

	iss := auth.NewIssuer("test-secret", time.Hour)
	candidateCookie, _ := iss.Issue(candidateID, testTokenVersion)
	unverifiedCookie, _ := iss.Issue(unverifiedID, testTokenVersion)
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	h.register(app.Group("/api/v1"), middleware{
		optionalCookie: auth.OptionalCookieAuth(iss, testVersions),
		cookie:         auth.RequireAuth(iss, testVersions),
	})

	do := func(t *testing.T, method, cookie, body string) (int, []byte) {
		t.Helper()
		var r *http.Request
		if body != "" {
			r = httptest.NewRequestWithContext(ctx, method, "/api/v1/jobs/"+slug+"/direct-apply", bytes.NewReader([]byte(body)))
			r.Header.Set("Content-Type", "application/json")
		} else {
			r = httptest.NewRequestWithContext(ctx, method, "/api/v1/jobs/"+slug+"/direct-apply", nil)
		}
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		}
		resp, err := app.Test(r, -1)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, b
	}
	application := `{"cv_id":"` + cvID + `","answers":{"q1":"true"},"note":"Hello"}`

	t.Run("a visitor sees the questions, a candidate also their prefill", func(t *testing.T) {
		status, body := do(t, fiber.MethodGet, "", "")
		if status != fiber.StatusOK {
			t.Fatalf("anonymous status = %d (%s)", status, body)
		}
		status, body = do(t, fiber.MethodGet, candidateCookie, "")
		var env struct {
			Data directApplyForm `json:"data"`
		}
		if err := json.Unmarshal(body, &env); err != nil || status != fiber.StatusOK {
			t.Fatalf("candidate status = %d, body %s, err %v", status, body, err)
		}
		f := env.Data
		if f.CompanyName != "Acme" || !f.Open || f.Applied || len(f.Questions) != 2 || f.Questions[0].Type != "boolean" {
			t.Fatalf("form = %+v", f)
		}
		if f.Prefill["q1"] != "true" {
			t.Errorf("prefill = %v, want the banked yes for q1", f.Prefill)
		}
		if f.CVs.Tailored == nil || f.CVs.Tailored.ID != cvID || !f.CVs.HasResume {
			t.Errorf("cvs = %+v, want the CV tailored for this vacancy and the uploaded one", f.CVs)
		}
	})

	t.Run("an API key carries no candidate's prefill", func(t *testing.T) {
		r := httptest.NewRequestWithContext(ctx, fiber.MethodGet, "/api/v1/jobs/"+slug+"/direct-apply", nil)
		r.Header.Set("Authorization", "Bearer "+candidateCookie)
		resp, err := app.Test(r, -1)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != fiber.StatusOK || bytes.Contains(b, []byte(`"q1":"true"`)) {
			t.Errorf("status = %d, body %s; want the form without the candidate's answers", resp.StatusCode, b)
		}
	})

	t.Run("an unverified user cannot apply", func(t *testing.T) {
		if status, body := do(t, fiber.MethodPost, unverifiedCookie, application); status != fiber.StatusForbidden {
			t.Errorf("status = %d (%s), want 403", status, body)
		}
	})

	t.Run("an application names exactly one CV", func(t *testing.T) {
		both := `{"resume":true,"cv_id":"` + cvID + `","answers":{"q1":"true"}}`
		if status, body := do(t, fiber.MethodPost, candidateCookie, both); status != fiber.StatusBadRequest {
			t.Errorf("both CVs: status = %d (%s), want 400", status, body)
		}
	})

	t.Run("a missing required answer names the question", func(t *testing.T) {
		status, body := do(t, fiber.MethodPost, candidateCookie, `{"cv_id":"`+cvID+`","answers":{}}`)
		if status != fiber.StatusBadRequest || !bytes.Contains(body, []byte("EU work permit")) {
			t.Errorf("status = %d, body %s; want a 400 naming the question", status, body)
		}
	})

	t.Run("a failed delivery is a 502 that does not leak the transport's error", func(t *testing.T) {
		mailer.fail = true
		status, body := do(t, fiber.MethodPost, candidateCookie, application)
		mailer.fail = false
		if status != fiber.StatusBadGateway || bytes.Contains(body, []byte("hr@acme.test")) {
			t.Errorf("status = %d, body %s; want a 502 without the employer's address", status, body)
		}
	})

	t.Run("the candidate applies and the employer receives it once", func(t *testing.T) {
		if status, body := do(t, fiber.MethodPost, candidateCookie, application); status != fiber.StatusCreated {
			t.Fatalf("status = %d (%s), want 201", status, body)
		}
		if len(mailer.sent) != 1 || mailer.sent[0].To != "founder@acme.test" || mailer.sent[0].CandidateEmail != "c@example.test" {
			t.Fatalf("mails = %+v", mailer.sent)
		}
		var applied bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM applications a JOIN jobs j ON j.id = a.job_id WHERE a.user_id = $1 AND j.public_slug = $2)`,
			candidateID, slug).Scan(&applied); err != nil || !applied {
			t.Errorf("tracking shows the application = %v, %v", applied, err)
		}
		if status, _ := do(t, fiber.MethodPost, candidateCookie, application); status != fiber.StatusConflict {
			t.Errorf("second application status = %d, want 409", status)
		}
		if len(mailer.sent) != 1 {
			t.Errorf("mails = %d after a refused second application, want 1", len(mailer.sent))
		}
		_, body := do(t, fiber.MethodGet, candidateCookie, "")
		if !bytes.Contains(body, []byte(`"applied":true`)) {
			t.Errorf("form after applying = %s, want applied", body)
		}
	})
}
