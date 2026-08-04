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
	"strings"
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
		// No mailbox factory: these tests cover the STORED path, which is the fallback and
		// the one a caller with no Gmail grant still gets. The search path has its own.
		h = h.withRecall(mailrecall.New(mailrecall.NewDBStore(queries), client),
			llmBinding{client: client}, nil)
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

// verdictsJSON says "the message at position N belongs". The model is addressed by
// POSITION, not by our id — a searched message has none of ours.
func verdictsJSON(t *testing.T, positions ...int) string {
	t.Helper()
	type verdict struct {
		Index      int     `json:"index"`
		Belongs    bool    `json:"belongs"`
		Confidence float64 `json:"confidence"`
	}
	out := struct {
		Verdicts []verdict `json:"verdicts"`
	}{}
	for _, n := range positions {
		out.Verdicts = append(out.Verdicts, verdict{Index: n, Belongs: true, Confidence: 0.95})
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
	model := &recallModel{reply: verdictsJSON(t, 1)}
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
	model := &recallModel{reply: verdictsJSON(t, 1)}
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

// fakeMailboxes stands a searchable mailbox up without a Google credential.
type fakeMailboxes struct{ box *fakeMailbox }

func (f *fakeMailboxes) For(context.Context, int64) mailrecall.Mailbox {
	if f.box == nil {
		return nil
	}
	return f.box
}

type fakeMailbox struct {
	found    []mailrecall.Message
	imported []string
	store    func(providerID string) error
}

func (m *fakeMailbox) Search(context.Context, int64, string, string, time.Time, time.Time) ([]mailrecall.Message, error) {
	return m.found, nil
}

func (m *fakeMailbox) Import(_ context.Context, _ int64, providerID string) error {
	m.imported = append(m.imported, providerID)
	if m.store != nil {
		return m.store(providerID)
	}
	return nil
}

// recallSearchApp mounts both recall routes over a handler whose mailbox is the fake.
func recallSearchApp(t *testing.T, pool *pgxpool.Pool, model llms.Model, box *fakeMailbox) (*fiber.App, *auth.Issuer) {
	t.Helper()
	queries := db.New(pool)
	client := llm.NewWithModel(model)
	h := newInboxHandlers(queries, pool, nil, nil, "", false, "inbox.freehire.test").
		withRecall(mailrecall.New(mailrecall.NewDBStore(queries), client),
			llmBinding{client: client}, &fakeMailboxes{box: box})

	iss := auth.NewIssuer("test-secret-that-is-long-enough-0001", time.Hour)
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	ra := auth.RequireAuthOrKey(iss, testVersions, apiKeys{queries})
	app.Post("/api/v1/me/tracking/:slug/mail-recall", ra, h.RecallApplicationMail)
	app.Post("/api/v1/me/tracking/:slug/mail-recall/link", ra, h.LinkRecalledMail)

	return app, iss
}

func postLink(t *testing.T, app *fiber.App, iss *auth.Issuer, userID int64, slug, providerID string) int {
	t.Helper()
	cookie, _ := iss.Issue(userID, testTokenVersion)
	r := httptest.NewRequest(fiber.MethodPost, "/api/v1/me/tracking/"+slug+"/mail-recall/link",
		strings.NewReader(`{"provider_id":"`+providerID+`"}`))
	r.Header.Set("Content-Type", "application/json")
	r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	resp, err := app.Test(r, -1)
	if err != nil {
		t.Fatalf("post link: %v", err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

// The change's central promise: a sweep over the mailbox keeps nothing. What a person has
// not confirmed is not stored — which is a change from the path that shipped first, where
// a confident answer wrote a suggestion whether anybody agreed with it or not.
func TestRecallOverAMailboxStoresNothing(t *testing.T) {
	pool := startPostgres(t)
	fx := seedRecallFixture(t, pool)
	box := &fakeMailbox{found: []mailrecall.Message{{
		ProviderID: "g-new", FromAddr: "maria@derq.example", FromName: "Maria Alvarez",
		Subject: "Next step — a 45 minute call", BodyText: "Could we book 45 minutes?",
		ReceivedAt: time.Now().Add(-24 * time.Hour),
	}}}
	app, iss := recallSearchApp(t, pool, &recallModel{reply: verdictsJSON(t, 1)}, box)

	status, body := postRecall(t, app, iss, fx.userID, fx.slug)
	if status != fiber.StatusOK {
		t.Fatalf("status %d, body %+v", status, body)
	}
	if len(body.Data.Suggested) != 1 || body.Data.Suggested[0].ID != 0 {
		t.Fatalf("suggested %+v — a searched message has no id of ours yet", body.Data.Suggested)
	}

	var rows int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM emails WHERE user_id=$1 AND external_id='g-new'`, fx.userID).Scan(&rows); err != nil {
		t.Fatalf("count: %v", err)
	}
	if rows != 0 {
		t.Errorf("the sweep stored %d rows for a message nobody confirmed", rows)
	}
	var suggested int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM emails WHERE user_id=$1 AND suggested_job_id IS NOT NULL`, fx.userID).Scan(&suggested); err != nil {
		t.Fatalf("count suggestions: %v", err)
	}
	if suggested != 0 {
		t.Errorf("the sweep planted %d suggestions", suggested)
	}
}

// Pressing Link is the moment the message arrives.
func TestLinkRecalledMailImportsThenLinks(t *testing.T) {
	pool := startPostgres(t)
	fx := seedRecallFixture(t, pool)
	ctx := context.Background()
	box := &fakeMailbox{store: func(providerID string) error {
		_, err := pool.Exec(ctx,
			`INSERT INTO emails (user_id, source, external_id, subject, body_text, received_at)
			 VALUES ($1,'gmail',$2,'Next step','Could we book 45 minutes?', now())
			 ON CONFLICT (user_id, source, external_id) DO NOTHING`, fx.userID, providerID)
		return err
	}}
	app, iss := recallSearchApp(t, pool, &recallModel{reply: verdictsJSON(t)}, box)

	if status := postLink(t, app, iss, fx.userID, fx.slug, "g-new"); status != fiber.StatusOK {
		t.Fatalf("link status %d", status)
	}
	if len(box.imported) != 1 || box.imported[0] != "g-new" {
		t.Fatalf("imported %v, want the message the caller pressed", box.imported)
	}

	var jobID *int64
	if err := pool.QueryRow(ctx,
		`SELECT job_id FROM emails WHERE user_id=$1 AND external_id='g-new'`, fx.userID).Scan(&jobID); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if jobID == nil || *jobID != fx.jobID {
		t.Errorf("job_id = %v, want %d — import must be followed by the link", jobID, fx.jobID)
	}
}

// A message the sync had already fetched is linked, not duplicated.
func TestLinkRecalledMailDoesNotDuplicateAStoredMessage(t *testing.T) {
	pool := startPostgres(t)
	fx := seedRecallFixture(t, pool)
	ctx := context.Background()
	box := &fakeMailbox{store: func(string) error { return nil }} // the row is already there
	app, iss := recallSearchApp(t, pool, &recallModel{reply: verdictsJSON(t)}, box)

	if status := postLink(t, app, iss, fx.userID, fx.slug, "recall-http-unlinked"); status != fiber.StatusOK {
		t.Fatalf("link status %d", status)
	}
	var rows int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM emails WHERE user_id=$1 AND external_id='recall-http-unlinked'`,
		fx.userID).Scan(&rows); err != nil {
		t.Fatalf("count: %v", err)
	}
	if rows != 1 {
		t.Errorf("%d rows for one message — the import must be idempotent", rows)
	}
}

// Someone else's application is not there to import into.
func TestLinkRecalledMailRefusesAnotherCallersApplication(t *testing.T) {
	pool := startPostgres(t)
	fx := seedRecallFixture(t, pool)
	box := &fakeMailbox{store: func(string) error { return nil }}
	app, iss := recallSearchApp(t, pool, &recallModel{reply: verdictsJSON(t)}, box)

	var stranger int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email) VALUES ('recall-link-stranger@example.test') RETURNING id`).Scan(&stranger); err != nil {
		t.Fatalf("seed stranger: %v", err)
	}
	if status := postLink(t, app, iss, stranger, fx.slug, "g-new"); status != fiber.StatusNotFound {
		t.Fatalf("status %d, want 404", status)
	}
	if len(box.imported) != 0 {
		t.Error("a message was imported for somebody else's application")
	}
}
