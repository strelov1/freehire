//go:build integration

// End-to-end HTTP tests for the verified-employer surface against a real Postgres: claim
// (201), confirm (auto-activate on a matching domain / stay pending on a mismatch),
// create/edit/close a vacancy (scoped to the owning account, a colliding URL from a
// different employer refused), the moderator review queue (role-gated, approve/reject),
// and the admin revoke action (role-gated, distinct from moderator).
// Run with: go test -tags=integration ./internal/api/handler/
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

	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/identity/accounts"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/ingest/employer"
	"github.com/strelov1/freehire/internal/ingest/moderation"
	"github.com/strelov1/freehire/internal/platform/db"
)

// fakeClaimMailer captures the mailed code instead of touching SES, keyed by recipient so
// two employers claiming in the same test do not clobber each other's code.
type fakeClaimMailer struct{ codes map[string]string }

func newFakeClaimMailer() *fakeClaimMailer { return &fakeClaimMailer{codes: map[string]string{}} }

func (m *fakeClaimMailer) SendClaimVerificationCode(_ context.Context, email, code string) error {
	m.codes[email] = code
	return nil
}

func TestEmployerEndToEnd(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)

	var modID, adminID, userAID, userBID, userCID int64
	seed := func(email, role string) int64 {
		t.Helper()
		var id int64
		q := "INSERT INTO users (email) VALUES ($1) RETURNING id"
		if role != "" {
			q = "INSERT INTO users (email, role) VALUES ($1, '" + role + "') RETURNING id"
		}
		if err := pool.QueryRow(ctx, q, email).Scan(&id); err != nil {
			t.Fatalf("seed %s: %v", email, err)
		}
		return id
	}
	modID = seed("mod@example.test", "moderator")
	adminID = seed("admin@example.test", "admin")
	userAID = seed("founder@acme.test", "")
	userBID = seed("founder@bravo.test", "")
	userCID = seed("founder@charlie.test", "")

	if _, err := pool.Exec(ctx,
		`INSERT INTO companies (slug, name, company_info) VALUES ('acme', 'Acme', '{"website":"acme.test"}')`); err != nil {
		t.Fatalf("seed company acme: %v", err)
	}
	// bravo has no known website, so its claim cannot auto-activate.

	iss := auth.NewIssuer("test-secret", time.Hour)
	modCookie, _ := iss.Issue(modID, testTokenVersion)
	adminCookie, _ := iss.Issue(adminID, testTokenVersion)
	userACookie, _ := iss.Issue(userAID, testTokenVersion)
	userBCookie, _ := iss.Issue(userBID, testTokenVersion)
	userCCookie, _ := iss.Issue(userCID, testTokenVersion)

	accountsSvc := accounts.New(accounts.NewQueriesRepository(queries, pool), authHasher{})
	mailer := newFakeClaimMailer()
	// The account-specific mailer (nil here) is never read by the generic IssueCode/
	// ConfirmCode this test exercises — only the CodeStore is; see accounts.Service.IssueCode.
	accountsSvc.WithCodes(accounts.NewQueriesCodeStore(queries, pool), nil)
	moderationSvc := moderation.New(moderation.NewQueriesRepository(queries, pool, enrich.Version))
	employerRepo := employer.NewQueriesRepository(queries, pool)
	employerSvc := employer.New(employerRepo, accountsSvc, mailer, employerRepo, moderationSvc)
	h := newEmployerHandlers(queries, employerSvc)

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	mw := middleware{
		cookie:    auth.RequireAuth(iss, testVersions),
		moderator: auth.RequireRole(queries, "moderator"),
	}
	h.register(app.Group("/api/v1"), mw)

	req := func(method, path, cookie, body string) *http.Request {
		var r *http.Request
		if body != "" {
			r = httptest.NewRequestWithContext(context.Background(), method, path, bytes.NewReader([]byte(body)))
			r.Header.Set("Content-Type", "application/json")
		} else {
			r = httptest.NewRequestWithContext(context.Background(), method, path, nil)
		}
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		}
		return r
	}
	do := func(t *testing.T, r *http.Request) *http.Response {
		t.Helper()
		resp, err := app.Test(r)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		return resp
	}
	decodeData := func(t *testing.T, resp *http.Response, out any) {
		t.Helper()
		env := struct {
			Data json.RawMessage `json:"data"`
		}{}
		body, _ := io.ReadAll(resp.Body)
		if err := json.Unmarshal(body, &env); err != nil {
			t.Fatalf("decode envelope: %v (body %s)", err, body)
		}
		if out != nil {
			if err := json.Unmarshal(env.Data, out); err != nil {
				t.Fatalf("decode data: %v (body %s)", err, body)
			}
		}
	}

	t.Run("employer A claims Acme and auto-activates on a matching domain", func(t *testing.T) {
		resp := do(t, req(fiber.MethodPost, "/api/v1/employer/claim", userACookie,
			`{"company_name":"Acme","work_email":"founder@acme.test"}`))
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusCreated {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("claim status = %d, want 201 (body %s)", resp.StatusCode, b)
		}
		var acc struct {
			CompanySlug string `json:"company_slug"`
			Status      string `json:"status"`
		}
		decodeData(t, resp, &acc)
		if acc.CompanySlug != "acme" || acc.Status != "pending" {
			t.Fatalf("claim response = %+v, want acme/pending", acc)
		}

		code := mailer.codes["founder@acme.test"]
		if code == "" {
			t.Fatal("no code was mailed to founder@acme.test")
		}
		resp = do(t, req(fiber.MethodPost, "/api/v1/employer/claim/confirm", userACookie, `{"code":"`+code+`"}`))
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("confirm status = %d, want 200 (body %s)", resp.StatusCode, b)
		}
		decodeData(t, resp, &acc)
		if acc.Status != "active" {
			t.Fatalf("status after confirm = %q, want active (matching domain)", acc.Status)
		}
	})

	t.Run("employer B claims Bravo and stays pending (unknown website)", func(t *testing.T) {
		resp := do(t, req(fiber.MethodPost, "/api/v1/employer/claim", userBCookie,
			`{"company_name":"Bravo","work_email":"founder@bravo.test"}`))
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusCreated {
			t.Fatalf("claim status = %d, want 201", resp.StatusCode)
		}
		code := mailer.codes["founder@bravo.test"]
		resp = do(t, req(fiber.MethodPost, "/api/v1/employer/claim/confirm", userBCookie, `{"code":"`+code+`"}`))
		defer resp.Body.Close()
		var acc struct {
			Status string `json:"status"`
		}
		decodeData(t, resp, &acc)
		if acc.Status != "pending" {
			t.Fatalf("status = %q, want pending (unknown website)", acc.Status)
		}
	})

	t.Run("a pending account cannot create a vacancy (403)", func(t *testing.T) {
		resp := do(t, req(fiber.MethodPost, "/api/v1/employer/jobs", userBCookie,
			`{"url":"https://bravo.test/jobs/1","title":"Go Engineer"}`))
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusForbidden {
			t.Errorf("status = %d, want 403", resp.StatusCode)
		}
	})

	t.Run("a pending account can still read its own status (200, not 403)", func(t *testing.T) {
		resp := do(t, req(fiber.MethodGet, "/api/v1/employer/company", userBCookie, ""))
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 200 (body %s)", resp.StatusCode, b)
		}
		var acc struct {
			Status string `json:"status"`
		}
		decodeData(t, resp, &acc)
		if acc.Status != "pending" {
			t.Errorf("status = %q, want pending", acc.Status)
		}
	})

	t.Run("a claim whose code was never entered is hidden, unapprovable, and resendable", func(t *testing.T) {
		resp := do(t, req(fiber.MethodPost, "/api/v1/employer/claim", userCCookie,
			`{"company_name":"Charlie","work_email":"founder@charlie.test"}`))
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusCreated {
			t.Fatalf("claim status = %d, want 201", resp.StatusCode)
		}

		resp = do(t, req(fiber.MethodGet, "/api/v1/employer/claims", modCookie, ""))
		defer resp.Body.Close()
		var rows []struct {
			CompanySlug string `json:"company_slug"`
		}
		decodeData(t, resp, &rows)
		for _, r := range rows {
			if r.CompanySlug == "charlie" {
				t.Fatalf("queue = %+v, an unconfirmed claim must not be in it", rows)
			}
		}

		resp = do(t, req(fiber.MethodPost, "/api/v1/employer/claims/"+itoa(userCID)+"/approve", modCookie, ""))
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusConflict {
			t.Fatalf("approve status = %d, want 409 for an unconfirmed claim", resp.StatusCode)
		}

		resp = do(t, req(fiber.MethodPost, "/api/v1/employer/claim/resend", userCCookie, ""))
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusTooManyRequests {
			t.Fatalf("immediate resend status = %d, want 429 inside the cooldown", resp.StatusCode)
		}

		// Age the first code past both the cooldown and its own expiry.
		if _, err := pool.Exec(ctx,
			`UPDATE user_email_codes SET created_at = now() - interval '1 hour' WHERE user_id = $1`, userCID); err != nil {
			t.Fatalf("age the first code: %v", err)
		}
		resp = do(t, req(fiber.MethodPost, "/api/v1/employer/claim/resend", userCCookie, ""))
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("resend status = %d, want 200 (body %s)", resp.StatusCode, b)
		}
		// The aged first code is past its expiry too, so this confirm only succeeds on a
		// freshly minted one.
		code := mailer.codes["founder@charlie.test"]

		resp = do(t, req(fiber.MethodPost, "/api/v1/employer/claim/confirm", userCCookie, `{"code":"`+code+`"}`))
		defer resp.Body.Close()
		var acc struct {
			Status         string `json:"status"`
			EmailConfirmed bool   `json:"email_confirmed"`
		}
		decodeData(t, resp, &acc)
		if acc.Status != "pending" || !acc.EmailConfirmed {
			t.Fatalf("after confirm = %+v, want pending with email_confirmed", acc)
		}

		resp = do(t, req(fiber.MethodPost, "/api/v1/employer/claim/resend", userCCookie, ""))
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusConflict {
			t.Errorf("resend after confirm status = %d, want 409", resp.StatusCode)
		}
	})

	t.Run("non-moderator cannot see the pending-claims queue (403)", func(t *testing.T) {
		resp := do(t, req(fiber.MethodGet, "/api/v1/employer/claims", userACookie, ""))
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusForbidden {
			t.Errorf("status = %d, want 403", resp.StatusCode)
		}
	})

	t.Run("a user with no employer account at all gets 404", func(t *testing.T) {
		resp := do(t, req(fiber.MethodGet, "/api/v1/employer/company", modCookie, ""))
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusNotFound {
			t.Errorf("status = %d, want 404", resp.StatusCode)
		}
	})

	t.Run("moderator approves Bravo's pending claim, seeding its blank website", func(t *testing.T) {
		resp := do(t, req(fiber.MethodGet, "/api/v1/employer/claims", modCookie, ""))
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("queue status = %d, want 200", resp.StatusCode)
		}
		var rows []struct {
			CompanySlug string `json:"company_slug"`
			Status      string `json:"status"`
		}
		decodeData(t, resp, &rows)
		found := false
		for _, r := range rows {
			if r.CompanySlug == "bravo" && r.Status == "pending" {
				found = true
			}
		}
		if !found {
			t.Fatalf("pending queue = %+v, want bravo/pending in it", rows)
		}

		resp = do(t, req(fiber.MethodPost, "/api/v1/employer/claims/"+itoa(userBID)+"/approve", modCookie, ""))
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("approve status = %d, want 200 (body %s)", resp.StatusCode, b)
		}
		var acc struct {
			Status string `json:"status"`
		}
		decodeData(t, resp, &acc)
		if acc.Status != "active" {
			t.Fatalf("status after approval = %q, want active", acc.Status)
		}

		var website string
		if err := pool.QueryRow(ctx, `SELECT company_info->>'website' FROM companies WHERE slug = 'bravo'`).Scan(&website); err != nil {
			t.Fatalf("read bravo: %v", err)
		}
		if website != "bravo.test" {
			t.Errorf("bravo website = %q, want bravo.test (seeded by the approval)", website)
		}
	})

	var acmeSlug string

	t.Run("employer A publishes a vacancy for their own company", func(t *testing.T) {
		resp := do(t, req(fiber.MethodPost, "/api/v1/employer/jobs", userACookie,
			`{"url":"https://acme.test/jobs/1","title":"Go Engineer","description":"We use Go."}`))
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusCreated {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("create status = %d, want 201 (body %s)", resp.StatusCode, b)
		}
		var jobResp struct {
			PublicSlug string `json:"public_slug"`
			Company    string `json:"company"`
		}
		decodeData(t, resp, &jobResp)
		if jobResp.Company != "Acme" {
			t.Errorf("company = %q, want Acme (the account's locked name, not request content)", jobResp.Company)
		}
		acmeSlug = jobResp.PublicSlug
	})

	t.Run("employer A lists their own vacancies; employer B's list stays empty", func(t *testing.T) {
		resp := do(t, req(fiber.MethodGet, "/api/v1/employer/jobs", userACookie, ""))
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		var listA []struct {
			PublicSlug string `json:"public_slug"`
		}
		decodeData(t, resp, &listA)
		if len(listA) != 1 || listA[0].PublicSlug != acmeSlug {
			t.Fatalf("A's list = %+v, want exactly [%s]", listA, acmeSlug)
		}

		resp = do(t, req(fiber.MethodGet, "/api/v1/employer/jobs", userBCookie, ""))
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		var listB []struct {
			PublicSlug string `json:"public_slug"`
		}
		decodeData(t, resp, &listB)
		if len(listB) != 0 {
			t.Errorf("B's list = %+v, want empty — B has published nothing", listB)
		}
	})

	t.Run("employer B cannot edit or close employer A's vacancy (404)", func(t *testing.T) {
		resp := do(t, req(fiber.MethodPatch, "/api/v1/employer/jobs/"+acmeSlug, userBCookie, `{"title":"Hijacked"}`))
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusNotFound {
			t.Errorf("update status = %d, want 404", resp.StatusCode)
		}
		resp = do(t, req(fiber.MethodPost, "/api/v1/employer/jobs/"+acmeSlug+"/close", userBCookie, ""))
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusNotFound {
			t.Errorf("close status = %d, want 404", resp.StatusCode)
		}
	})

	t.Run("employer A edits and closes their own vacancy", func(t *testing.T) {
		resp := do(t, req(fiber.MethodPatch, "/api/v1/employer/jobs/"+acmeSlug, userACookie, `{"title":"Senior Go Engineer"}`))
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("update status = %d, want 200 (body %s)", resp.StatusCode, b)
		}
		var jobResp struct {
			Title string `json:"title"`
		}
		decodeData(t, resp, &jobResp)
		if jobResp.Title != "Senior Go Engineer" {
			t.Errorf("title = %q, want the edit applied", jobResp.Title)
		}

		resp = do(t, req(fiber.MethodPost, "/api/v1/employer/jobs/"+acmeSlug+"/close", userACookie, ""))
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("close status = %d, want 200", resp.StatusCode)
		}
		var closedReason string
		if err := pool.QueryRow(ctx, `SELECT closed_reason FROM jobs WHERE public_slug = $1`, acmeSlug).Scan(&closedReason); err != nil {
			t.Fatalf("read job: %v", err)
		}
		if closedReason != "employer_closed" {
			t.Errorf("closed_reason = %q, want employer_closed", closedReason)
		}
	})

	t.Run("employer A edits their own company's curated profile", func(t *testing.T) {
		resp := do(t, req(fiber.MethodPatch, "/api/v1/employer/company", userACookie, `{"tagline":"We build things"}`))
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 200 (body %s)", resp.StatusCode, b)
		}
		var company struct {
			Tagline string `json:"tagline"`
		}
		decodeData(t, resp, &company)
		if company.Tagline != "We build things" {
			t.Errorf("tagline = %q, want the edit applied", company.Tagline)
		}
	})

	t.Run("employer A publishes a vacancy taking applications on freehire", func(t *testing.T) {
		resp := do(t, req(fiber.MethodPost, "/api/v1/employer/jobs", userACookie,
			`{"apply_mode":"freehire","title":"Platform Engineer","questions":[`+
				`{"label":"EU work permit?","type":"boolean","required":true},`+
				`{"label":"Stack","type":"select","options":["Go","Rust"]}]}`))
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusCreated {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 201 (body %s)", resp.StatusCode, b)
		}
		var view struct {
			PublicSlug  string `json:"public_slug"`
			URL         string `json:"url"`
			DirectApply bool   `json:"direct_apply"`
		}
		decodeData(t, resp, &view)
		if !view.DirectApply || view.URL != "" {
			t.Fatalf("view = %+v, want direct_apply with no url", view)
		}

		resp = do(t, req(fiber.MethodPatch, "/api/v1/employer/jobs/"+view.PublicSlug, userACookie,
			`{"questions":[{"label":"Notice period?","type":"text"}]}`))
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("question edit status = %d, want 200 (body %s)", resp.StatusCode, b)
		}
		var label string
		if err := pool.QueryRow(ctx, `SELECT f.payload->'fields'->0->>'label' FROM apply_forms f JOIN jobs j ON j.id = f.job_id WHERE j.public_slug = $1`,
			view.PublicSlug).Scan(&label); err != nil {
			t.Fatalf("read form: %v", err)
		}
		if label != "Notice period?" {
			t.Errorf("stored first question = %q, want the edit", label)
		}

		resp = do(t, req(fiber.MethodPatch, "/api/v1/employer/jobs/"+view.PublicSlug, userACookie, `{"apply_mode":"external"}`))
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusBadRequest {
			t.Errorf("mode change status = %d, want 400", resp.StatusCode)
		}
		resp = do(t, req(fiber.MethodPost, "/api/v1/employer/jobs", userACookie,
			`{"apply_mode":"freehire","title":"T","questions":[{"label":"Q","type":"select","options":["only"]}]}`))
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusBadRequest {
			t.Errorf("malformed question status = %d, want 400", resp.StatusCode)
		}
	})

	t.Run("moderator cannot revoke (403); admin can", func(t *testing.T) {
		resp := do(t, req(fiber.MethodPost, "/api/v1/employer/claims/"+itoa(userAID)+"/revoke", modCookie, ""))
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusForbidden {
			t.Errorf("moderator revoke status = %d, want 403", resp.StatusCode)
		}

		resp = do(t, req(fiber.MethodPost, "/api/v1/employer/claims/"+itoa(userAID)+"/revoke", adminCookie, ""))
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("admin revoke status = %d, want 200 (body %s)", resp.StatusCode, b)
		}
		var acc struct {
			Status string `json:"status"`
		}
		decodeData(t, resp, &acc)
		if acc.Status != "revoked" {
			t.Fatalf("status = %q, want revoked", acc.Status)
		}

		resp = do(t, req(fiber.MethodPost, "/api/v1/employer/jobs", userACookie, `{"url":"https://acme.test/jobs/2","title":"Another Role"}`))
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusForbidden {
			t.Errorf("post-revoke create status = %d, want 403", resp.StatusCode)
		}
	})
}
