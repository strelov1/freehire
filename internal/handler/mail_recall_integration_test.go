//go:build integration

// Integration tests for POST /me/tracking/:slug/mail-recall — the button that sweeps the
// caller's mailbox for one application's mail.
//
// The assertions that matter are the ones about what did NOT happen: nothing is linked,
// no stage moves, and no employer_reply reaches the ledger. Run with:
// go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/llm"
	"github.com/strelov1/freehire/internal/mailrecall"
)

// recallModel answers whatever the test told it to, and counts calls so a test can assert
// the model was never reached.
type recallModel struct {
	reply string
	err   error
	calls int
}

func (m *recallModel) GenerateContent(context.Context, []llms.MessageContent, ...llms.CallOption) (*llms.ContentResponse, error) {
	m.calls++
	if m.err != nil {
		return nil, m.err
	}
	return &llms.ContentResponse{Choices: []*llms.ContentChoice{{Content: m.reply}}}, nil
}

func (*recallModel) Call(context.Context, string, ...llms.CallOption) (string, error) { return "", nil }

type recallBody struct {
	Data struct {
		Scanned     int `json:"scanned"`
		Invitations int `json:"invitations"`
		Suggested   []struct {
			ID      int64  `json:"id"`
			Subject string `json:"subject"`
		} `json:"suggested"`
	} `json:"data"`
	Error string `json:"error"`
}

// seedRecallJob inserts a posting and returns its id and public slug.
func seedRecallJob(t *testing.T, pool *pgxpool.Pool, ext, title, company string) (int64, string) {
	t.Helper()
	var id int64
	slug := ext + "-slug"
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, company, public_slug)
		 VALUES ('greenhouse', $1, 'https://example.test/' || $1, $2, $3, $4) RETURNING id`,
		ext, title, company, slug).Scan(&id); err != nil {
		t.Fatalf("seed job %s: %v", ext, err)
	}
	return id, slug
}

// recallFixture is one caller with one application and a mailbox around it.
type recallFixture struct {
	pool     *pgxpool.Pool
	userID   int64
	slug     string
	jobID    int64
	unlinked int64
	linked   int64
}

func seedRecallFixture(t *testing.T, pool *pgxpool.Pool) recallFixture {
	t.Helper()
	ctx := context.Background()
	q := db.New(pool)

	var userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('recall-http@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	jobID, slug := seedRecallJob(t, pool, "recall-http-job", "Backend Engineer", "Derq")
	if _, err := q.MarkJobApplied(ctx, db.MarkJobAppliedParams{
		UserID: userID, JobID: jobID, EventSource: "user",
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	var appID int64
	if err := pool.QueryRow(ctx,
		`SELECT id FROM applications WHERE user_id = $1 AND job_id = $2`, userID, jobID).Scan(&appID); err != nil {
		t.Fatalf("read application: %v", err)
	}

	at := time.Now().Add(-24 * time.Hour)
	seed := func(ext, subject string) int64 {
		var id int64
		if err := pool.QueryRow(ctx,
			`INSERT INTO emails (user_id, source, external_id, subject, body_text, received_at)
			 VALUES ($1, 'gmail', $2, $3, 'We received your application.', $4) RETURNING id`,
			userID, ext, subject, at).Scan(&id); err != nil {
			t.Fatalf("seed email %s: %v", ext, err)
		}
		return id
	}
	unlinked := seed("recall-http-unlinked", "Thanks for applying to Derq")
	linked := seed("recall-http-linked", "Already ours")
	if _, err := pool.Exec(ctx,
		`UPDATE emails SET job_id = $2, application_id = $3 WHERE id = $1`, linked, jobID, appID); err != nil {
		t.Fatalf("link email: %v", err)
	}

	return recallFixture{pool: pool, userID: userID, slug: slug, jobID: jobID, unlinked: unlinked, linked: linked}
}

// recallApp mounts the route over handlers wired to the given model.
func recallApp(t *testing.T, pool *pgxpool.Pool, model llms.Model) (*fiber.App, *auth.Issuer) {
	t.Helper()
	queries := db.New(pool)
	h := newInboxHandlers(queries, pool, nil, nil, "", false, "inbox.freehire.test")
	if model != nil {
		client := llm.NewWithModel(model)
		// A real binding, not a zero one: it drives the whole userLLM path — resolve the
		// caller's credential, fall open to the service one, tag the call — which a zero
		// binding short-circuits before any of it runs.
		h = h.withRecall(mailrecall.New(mailrecall.NewDBStore(queries), client),
			llmBinding{client: client})
	}

	iss := auth.NewIssuer("test-secret-that-is-long-enough-0001", time.Hour)
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	// RequireAuthOrKey, not RequireAuth: production mounts this on mw.key, so a full-scope
	// API key can press the button, and testing the cookie-only gate would leave the path
	// that makes rate limiting matter unexercised.
	app.Post("/api/v1/me/tracking/:slug/mail-recall",
		auth.RequireAuthOrKey(iss, testVersions, apiKeys{queries}), mailRecallLimiter(), h.RecallApplicationMail)

	return app, iss
}

func postRecall(t *testing.T, app *fiber.App, iss *auth.Issuer, userID int64, slug string) (int, recallBody) {
	t.Helper()
	cookie, _ := iss.Issue(userID, testTokenVersion)
	r := httptest.NewRequest(fiber.MethodPost, "/api/v1/me/tracking/"+slug+"/mail-recall", nil)
	r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	resp, err := app.Test(r, -1)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	var body recallBody
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp.StatusCode, body
}

func verdictsJSON(t *testing.T, ids ...int64) string {
	t.Helper()
	type verdict struct {
		EmailID    int64   `json:"email_id"`
		Belongs    bool    `json:"belongs"`
		Confidence float64 `json:"confidence"`
	}
	out := struct {
		Verdicts []verdict `json:"verdicts"`
	}{}
	for _, id := range ids {
		out.Verdicts = append(out.Verdicts, verdict{EmailID: id, Belongs: true, Confidence: 0.95})
	}
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal verdicts: %v", err)
	}
	return string(raw)
}

// The happy path, and everything it must leave alone. A proposal is a suggestion: the
// message stays unlinked, the stage does not move, and the ledger records no reply —
// because a reply is recorded on a LINK, and this endpoint cannot make one.
func TestRecallApplicationMail_ProposesWithoutLinking(t *testing.T) {
	pool := startPostgres(t)
	fx := seedRecallFixture(t, pool)
	model := &recallModel{reply: verdictsJSON(t, fx.unlinked)}
	app, iss := recallApp(t, pool, model)

	status, body := postRecall(t, app, iss, fx.userID, fx.slug)
	if status != fiber.StatusOK {
		t.Fatalf("status %d, body %+v", status, body)
	}
	if body.Data.Scanned != 1 {
		t.Errorf("scanned %d, want 1 — the linked message must not be examined", body.Data.Scanned)
	}
	if len(body.Data.Suggested) != 1 || body.Data.Suggested[0].ID != fx.unlinked {
		t.Fatalf("suggested %+v, want the one unattached message", body.Data.Suggested)
	}
	if body.Data.Suggested[0].Subject == "" {
		t.Error("the response carries no subject — the card cannot draw a row from an id alone")
	}

	ctx := context.Background()
	var jobID, suggested *int64
	if err := pool.QueryRow(ctx,
		`SELECT job_id, suggested_job_id FROM emails WHERE id = $1`, fx.unlinked).Scan(&jobID, &suggested); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if jobID != nil {
		t.Errorf("the message was LINKED to job %d — this endpoint may only propose", *jobID)
	}
	if suggested == nil || *suggested != fx.jobID {
		t.Errorf("suggested_job_id = %v, want %d", suggested, fx.jobID)
	}

	var stage *string
	if err := pool.QueryRow(ctx,
		`SELECT stage FROM applications WHERE user_id = $1 AND job_id = $2`, fx.userID, fx.jobID).Scan(&stage); err != nil {
		t.Fatalf("read stage: %v", err)
	}
	if stage == nil || *stage != "applied" {
		// Asserting the value, not just "not something else": a nil stage would pass the
		// looser check and cannot distinguish "did not move" from "never set".
		t.Errorf("stage = %v, want it still at applied", stage)
	}

	var replies int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM application_events WHERE user_id = $1 AND kind = 'employer_reply'`,
		fx.userID).Scan(&replies); err != nil {
		t.Fatalf("count ledger: %v", err)
	}
	if replies != 0 {
		t.Errorf("%d employer_reply events were written — a proposal is not a reply", replies)
	}
}

// Someone else's application is not there to be swept.
func TestRecallApplicationMail_RefusesAnApplicationThatIsNotTheCallers(t *testing.T) {
	pool := startPostgres(t)
	fx := seedRecallFixture(t, pool)
	model := &recallModel{reply: verdictsJSON(t, fx.unlinked)}
	app, iss := recallApp(t, pool, model)

	var stranger int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email) VALUES ('recall-stranger@example.test') RETURNING id`).Scan(&stranger); err != nil {
		t.Fatalf("seed stranger: %v", err)
	}

	status, _ := postRecall(t, app, iss, stranger, fx.slug)
	if status != fiber.StatusNotFound {
		t.Fatalf("status %d, want 404", status)
	}
	if model.calls != 0 {
		t.Error("the model was called for somebody else's application")
	}
}

// A tracked job nobody applied to has no mail to find, and the refusal is the service's,
// not the handler's — so the in-process caller meets it too.
func TestRecallApplicationMail_RefusesAJobThatWasNeverAppliedTo(t *testing.T) {
	pool := startPostgres(t)
	fx := seedRecallFixture(t, pool)
	ctx := context.Background()
	savedJob, slug := seedRecallJob(t, pool, "recall-http-saved", "Platform Engineer", "Ramp")
	if _, err := pool.Exec(ctx,
		`INSERT INTO user_jobs (user_id, job_id, saved_at) VALUES ($1, $2, now())`, fx.userID, savedJob); err != nil {
		t.Fatalf("save job: %v", err)
	}

	model := &recallModel{reply: verdictsJSON(t)}
	app, iss := recallApp(t, pool, model)

	status, _ := postRecall(t, app, iss, fx.userID, slug)
	if status != fiber.StatusNotFound {
		t.Fatalf("status %d, want 404", status)
	}
	if model.calls != 0 {
		t.Error("the model was called for a job that was never applied to")
	}
}

// The person pressed a button and is waiting. A failed model call is an error, never an
// empty success that reads as "your mailbox holds nothing".
func TestRecallApplicationMail_ReportsAFailedModelCall(t *testing.T) {
	pool := startPostgres(t)
	fx := seedRecallFixture(t, pool)
	app, iss := recallApp(t, pool, &recallModel{err: errors.New("gateway down")})

	status, body := postRecall(t, app, iss, fx.userID, fx.slug)
	if status != fiber.StatusBadGateway {
		t.Fatalf("status %d, want 502", status)
	}
	if body.Error == "" {
		t.Error("the failure carried no message")
	}

	var suggested *int64
	if err := pool.QueryRow(context.Background(),
		`SELECT suggested_job_id FROM emails WHERE id = $1`, fx.unlinked).Scan(&suggested); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if suggested != nil {
		t.Errorf("a suggestion survived a failed run (job %d)", *suggested)
	}
}

// A deployment with no model configured reports the feature off rather than panicking.
func TestRecallApplicationMail_ReportsTheFeatureOffWhenNoModelIsConfigured(t *testing.T) {
	pool := startPostgres(t)
	fx := seedRecallFixture(t, pool)
	app, iss := recallApp(t, pool, nil)

	if status, _ := postRecall(t, app, iss, fx.userID, fx.slug); status != fiber.StatusServiceUnavailable {
		t.Fatalf("status %d, want 503", status)
	}
}
