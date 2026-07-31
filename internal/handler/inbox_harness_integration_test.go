//go:build integration

// Integration tests for the agent inbox surface against a real Postgres: the mail
// routes authenticate a full-scope API key (not only a session cookie), the
// listing serves bodies and a triage queue without marking mail read, a harness
// can push mail it fetched itself, and one triage call records the verdict and
// advances the application stage. Run with:
// go test -tags=integration ./internal/handler/
package handler

import (
	"bytes"
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
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
)

// harnessInboxFixture is one signed-in user with an inbox, an app wired through the
// real route registration (so the tests pin what register mounts, not a
// hand-wired middleware), and both a session cookie and a full-scope API key.
type harnessInboxFixture struct {
	t      *testing.T
	pool   *pgxpool.Pool
	q      *db.Queries
	app    *fiber.App
	userID int64
	cookie string
	key    string
}

func newHarnessInboxFixture(t *testing.T, email string) *harnessInboxFixture {
	t.Helper()
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)

	var uid int64
	// Verified: the fixture mints an API key, which an unproven address cannot do.
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, email_verified) VALUES ($1, true) RETURNING id`, email).Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	iss := auth.NewIssuer("test-secret-that-is-long-enough-0001", time.Hour)
	cookie, _ := iss.Issue(uid, testTokenVersion)

	token, hash, prefix, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	if _, err := queries.CreateAPIKey(ctx, db.CreateAPIKeyParams{
		UserID: uid, Name: "harness", TokenHash: hash, TokenPrefix: prefix, Scope: "full",
	}); err != nil {
		t.Fatalf("store key: %v", err)
	}

	h := newInboxHandlers(queries, pool, nil, nil, "", false, "inbox.freehire.test")
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	h.register(app.Group("/api/v1"), middleware{
		cookie: auth.RequireAuth(iss, testVersions),
		key:    auth.RequireAuthOrKey(iss, testVersions, apiKeys{queries}),
	})

	return &harnessInboxFixture{t: t, pool: pool, q: queries, app: app, userID: uid, cookie: cookie, key: token}
}

// callKey issues a request authenticated by the API key — the harness's credential.
func (f *harnessInboxFixture) callKey(method, path string, body any) (int, map[string]any) {
	f.t.Helper()
	return f.call(method, path, body, func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+f.key)
	})
}

// callAnon issues a request with no credential at all.
func (f *harnessInboxFixture) callAnon(method, path string) (int, map[string]any) {
	f.t.Helper()
	return f.call(method, path, nil, func(*http.Request) {})
}

func (f *harnessInboxFixture) call(method, path string, body any, auth func(*http.Request)) (int, map[string]any) {
	f.t.Helper()
	var r *http.Request
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			f.t.Fatalf("marshal body: %v", err)
		}
		r = httptest.NewRequest(method, path, bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	auth(r)
	resp, err := f.app.Test(r, -1)
	if err != nil {
		f.t.Fatalf("%s %s: %v", method, path, err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var decoded map[string]any
	_ = json.Unmarshal(raw, &decoded)
	return resp.StatusCode, decoded
}

// seedEmail stores one message directly, bypassing the ingest endpoint.
func (f *harnessInboxFixture) seedEmail(userID int64, source, externalID, subject, body string) int64 {
	f.t.Helper()
	var id int64
	err := f.pool.QueryRow(context.Background(),
		`INSERT INTO emails (user_id, source, external_id, subject, body_text, received_at)
		 VALUES ($1, $2, $3, $4, $5, now()) RETURNING id`,
		userID, source, externalID, subject, body).Scan(&id)
	if err != nil {
		f.t.Fatalf("seed email: %v", err)
	}
	return id
}

// TestMailSurfaceAcceptsAnAPIKey asserts the whole point of the change: a harness
// holding its owner's full-scope key can read the inbox without a browser session,
// while an unauthenticated request stays refused and a key still cannot cross the
// tenant boundary.
func TestMailSurfaceAcceptsAnAPIKey(t *testing.T) {
	f := newHarnessInboxFixture(t, "keyed@example.test")
	f.seedEmail(f.userID, "hosted", "k-1", "Yours", "body")

	var strangerID int64
	if err := f.pool.QueryRow(context.Background(),
		`INSERT INTO users (email) VALUES ('stranger@example.test') RETURNING id`).Scan(&strangerID); err != nil {
		t.Fatalf("seed stranger: %v", err)
	}
	theirs := f.seedEmail(strangerID, "hosted", "k-2", "Theirs", "body")

	code, body := f.callKey("GET", "/api/v1/me/inbox", nil)
	if code != 200 {
		t.Fatalf("keyed inbox listing = %d, want 200", code)
	}
	if data, _ := body["data"].([]any); len(data) != 1 {
		t.Errorf("keyed listing returned %d messages, want 1", len(data))
	}

	if code, _ := f.callAnon("GET", "/api/v1/me/inbox"); code != 401 {
		t.Errorf("unauthenticated listing = %d, want 401", code)
	}

	if code, _ := f.callKey("GET", "/api/v1/me/emails/"+strconv.FormatInt(theirs, 10), nil); code != 404 {
		t.Errorf("keyed read of another user's message = %d, want 404", code)
	}
}

// messages decodes a listing response's data array into per-message maps.
func messages(t *testing.T, body map[string]any) []map[string]any {
	t.Helper()
	raw, _ := body["data"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, r := range raw {
		m, ok := r.(map[string]any)
		if !ok {
			t.Fatalf("listing row is %T, want an object", r)
		}
		out = append(out, m)
	}
	return out
}

// TestAgentListingServesBodiesWithoutMarkingRead asserts the agent reads a whole
// page of mail in one request and — unlike opening a message — leaves read_at
// alone. read_at means "a human saw this"; a harness sweeping the backlog must
// not zero its owner's unread count.
func TestAgentListingServesBodiesWithoutMarkingRead(t *testing.T) {
	f := newHarnessInboxFixture(t, "bodies@example.test")
	id := f.seedEmail(f.userID, "external", "b-1", "Invitation", "we would like to meet you")

	code, body := f.callKey("GET", "/api/v1/me/inbox?body=1", nil)
	if code != 200 {
		t.Fatalf("listing with bodies = %d, want 200", code)
	}
	rows := messages(t, body)
	if len(rows) != 1 {
		t.Fatalf("listing returned %d messages, want 1", len(rows))
	}
	if got := rows[0]["body_text"]; got != "we would like to meet you" {
		t.Errorf("body_text = %v, want the stored body", got)
	}

	var readAt *time.Time
	if err := f.pool.QueryRow(context.Background(),
		`SELECT read_at FROM emails WHERE id = $1`, id).Scan(&readAt); err != nil {
		t.Fatalf("read read_at: %v", err)
	}
	if readAt != nil {
		t.Error("listing with bodies marked the message read; only opening one may do that")
	}

	// Without the option the body stays out of the payload, so the web inbox keeps
	// transferring snippets.
	_, plain := f.callKey("GET", "/api/v1/me/inbox", nil)
	if got, ok := messages(t, plain)[0]["body_text"]; ok && got != "" {
		t.Errorf("body rode along uninvited: %v", got)
	}
}

// TestAgentListingReadsHTMLOnlyMailAsText asserts the listing hands the agent the
// same readable text the classifier reads: many ATS templates carry no plain-text
// part, and shipping raw HTML would waste the agent's context.
func TestAgentListingReadsHTMLOnlyMailAsText(t *testing.T) {
	f := newHarnessInboxFixture(t, "htmlonly@example.test")
	if _, err := f.pool.Exec(context.Background(),
		`INSERT INTO emails (user_id, source, external_id, subject, body_html, received_at)
		 VALUES ($1, 'external', 'h-1', 'HTML only', '<p>Congratulations, we have an <b>offer</b></p>', now())`,
		f.userID); err != nil {
		t.Fatalf("seed html-only email: %v", err)
	}

	_, body := f.callKey("GET", "/api/v1/me/inbox?body=1", nil)
	got, _ := messages(t, body)[0]["body_text"].(string)
	if got == "" {
		t.Fatal("html-only message returned an empty body; the agent would see only the subject")
	}
	if strings.Contains(got, "<b>") {
		t.Errorf("body_text carried raw HTML: %q", got)
	}
	if !strings.Contains(got, "offer") {
		t.Errorf("body_text lost the message text: %q", got)
	}
}

// TestAgentListingUnclassifiedIsTheWorkQueue asserts the agent can find what still
// needs triage — its only way to, since external mail is never enqueued for the
// classification worker.
func TestAgentListingUnclassifiedIsTheWorkQueue(t *testing.T) {
	f := newHarnessInboxFixture(t, "queue@example.test")
	pending := f.seedEmail(f.userID, "external", "q-1", "Needs triage", "body")
	done := f.seedEmail(f.userID, "external", "q-2", "Already triaged", "body")
	if _, err := f.pool.Exec(context.Background(),
		`UPDATE emails SET classified_at = now(), status_signal = 'rejection' WHERE id = $1`, done); err != nil {
		t.Fatalf("stamp classified: %v", err)
	}

	_, body := f.callKey("GET", "/api/v1/me/inbox?unclassified=1", nil)
	rows := messages(t, body)
	if len(rows) != 1 {
		t.Fatalf("unclassified listing returned %d messages, want 1", len(rows))
	}
	if got := int64(rows[0]["id"].(float64)); got != pending {
		t.Errorf("unclassified listing returned %d, want the untriaged %d", got, pending)
	}
	meta, _ := body["meta"].(map[string]any)
	if total, _ := meta["total"].(float64); total != 1 {
		t.Errorf("unclassified total = %v, want 1 — the count must honour the filter", total)
	}
}

// TestAgentListingAcceptsTheExternalSource asserts pushed mail is reachable through
// the account switcher like any other source, and that an unknown one is still a
// 400 rather than a silently empty list.
func TestAgentListingAcceptsTheExternalSource(t *testing.T) {
	f := newHarnessInboxFixture(t, "source@example.test")
	f.seedEmail(f.userID, "external", "s-1", "Pushed", "body")
	f.seedEmail(f.userID, "hosted", "s-2", "Hosted", "body")

	_, body := f.callKey("GET", "/api/v1/me/inbox?source=external", nil)
	if rows := messages(t, body); len(rows) != 1 {
		t.Errorf("source=external returned %d messages, want 1", len(rows))
	}
	if code, _ := f.callKey("GET", "/api/v1/me/inbox?source=nowhere", nil); code != 400 {
		t.Errorf("unknown source = %d, want 400", code)
	}
}

// TestAgentListingCapsThePage asserts an agent asking for bodies cannot request an
// unbounded page: bodies are the one listing payload heavy enough to matter.
func TestAgentListingCapsThePage(t *testing.T) {
	f := newHarnessInboxFixture(t, "cap@example.test")

	_, body := f.callKey("GET", "/api/v1/me/inbox?body=1&limit=500", nil)
	meta, _ := body["meta"].(map[string]any)
	if limit, _ := meta["limit"].(float64); limit != 50 {
		t.Errorf("limit = %v, want it clamped to 50", limit)
	}
}

// pushBody builds an ingest batch payload of n messages.
func pushBody(msgs ...map[string]any) map[string]any {
	return map[string]any{"messages": msgs}
}

func message(externalID, subject, body string) map[string]any {
	return map[string]any{
		"external_id": externalID,
		"from_addr":   "ats@acme.example",
		"from_name":   "Acme Hiring",
		"subject":     subject,
		"body_text":   body,
		"received_at": time.Now().UTC().Format(time.RFC3339),
	}
}

// TestIngestStoresPushedMail asserts the core of the bring-your-own-harness tier:
// a caller's own mail client hands over messages, they land in the ordinary inbox
// under source 'external', and a re-sync updates rather than duplicates.
func TestIngestStoresPushedMail(t *testing.T) {
	f := newHarnessInboxFixture(t, "ingest@example.test")

	code, body := f.callKey("POST", "/api/v1/me/emails", pushBody(
		message("<a@acme>", "Application received", "thanks for applying"),
		message("<b@acme>", "Interview invitation", "are you free Tuesday?"),
	))
	if code != 200 {
		t.Fatalf("ingest = %d, want 200 (body %v)", code, body)
	}
	data, _ := body["data"].(map[string]any)
	if inserted, _ := data["inserted"].(float64); inserted != 2 {
		t.Errorf("inserted = %v, want 2", inserted)
	}
	if updated, _ := data["updated"].(float64); updated != 0 {
		t.Errorf("updated = %v, want 0", updated)
	}

	_, listing := f.callKey("GET", "/api/v1/me/inbox?source=external", nil)
	if rows := messages(t, listing); len(rows) != 2 {
		t.Fatalf("pushed mail listed %d messages, want 2", len(rows))
	}

	// A re-sync of one of them reports an update, not a second row.
	_, body = f.callKey("POST", "/api/v1/me/emails", pushBody(
		message("<a@acme>", "Application received", "thanks for applying"),
	))
	data, _ = body["data"].(map[string]any)
	if inserted, _ := data["inserted"].(float64); inserted != 0 {
		t.Errorf("re-push inserted = %v, want 0", inserted)
	}
	if updated, _ := data["updated"].(float64); updated != 1 {
		t.Errorf("re-push updated = %v, want 1", updated)
	}
	_, listing = f.callKey("GET", "/api/v1/me/inbox?source=external", nil)
	if rows := messages(t, listing); len(rows) != 2 {
		t.Errorf("re-push left %d messages, want 2", len(rows))
	}
}

// TestIngestRejectsBadBatches asserts a malformed batch is refused whole rather
// than partly written: the caller's harness retries, and a half-stored batch would
// make "inserted" a lie.
func TestIngestRejectsBadBatches(t *testing.T) {
	f := newHarnessInboxFixture(t, "badbatch@example.test")

	if code, _ := f.callKey("POST", "/api/v1/me/emails", pushBody(
		message("<ok@acme>", "Fine", "body"),
		message("", "No id", "body"),
	)); code != 400 {
		t.Errorf("batch with an empty external id = %d, want 400", code)
	}
	_, listing := f.callKey("GET", "/api/v1/me/inbox", nil)
	if rows := messages(t, listing); len(rows) != 0 {
		t.Errorf("a rejected batch stored %d messages; it must be all-or-nothing", len(rows))
	}

	oversized := make([]map[string]any, 101)
	for i := range oversized {
		oversized[i] = message("<bulk-"+strconv.Itoa(i)+"@acme>", "Bulk", "body")
	}
	if code, _ := f.callKey("POST", "/api/v1/me/emails", pushBody(oversized...)); code != 400 {
		t.Errorf("oversized batch = %d, want 400", code)
	}

	if code, _ := f.callKey("POST", "/api/v1/me/emails", pushBody()); code != 400 {
		t.Errorf("empty batch = %d, want 400", code)
	}
}

// applyToJob makes the fixture's user an applicant of a job at a given stage.
func (f *harnessInboxFixture) applyToJob(slug, stage string) int64 {
	f.t.Helper()
	ctx := context.Background()
	var jobID int64
	err := f.pool.QueryRow(ctx,
		`INSERT INTO jobs (source, external_id, url, title, company, public_slug)
		 VALUES ('test', $1, 'http://example.test/'||$1, 'Go Dev', 'Acme', $1) RETURNING id`, slug).Scan(&jobID)
	if err != nil {
		f.t.Fatalf("seed job: %v", err)
	}
	if _, err := f.pool.Exec(ctx,
		`INSERT INTO user_jobs (user_id, job_id, applied_at, stage) VALUES ($1, $2, now(), $3)`,
		f.userID, jobID, stage); err != nil {
		f.t.Fatalf("seed application: %v", err)
	}
	return jobID
}

func (f *harnessInboxFixture) stageOf(jobID int64) string {
	f.t.Helper()
	var stage *string
	if err := f.pool.QueryRow(context.Background(),
		`SELECT stage FROM user_jobs WHERE user_id = $1 AND job_id = $2`, f.userID, jobID).Scan(&stage); err != nil {
		f.t.Fatalf("read stage: %v", err)
	}
	if stage == nil {
		return ""
	}
	return *stage
}

// TestTriageRecordsTheVerdictAndAdvancesTheStage asserts one call does what the
// classification worker does: classify, link, stamp, and move the application
// forward — so an agent-run inbox behaves like a server-classified one.
func TestTriageRecordsTheVerdictAndAdvancesTheStage(t *testing.T) {
	f := newHarnessInboxFixture(t, "triage@example.test")
	jobID := f.applyToJob("go-dev-acme-aaaaaaaa", "applied")
	id := f.seedEmail(f.userID, "external", "t-1", "Interview invitation", "are you free?")

	code, body := f.callKey("POST", "/api/v1/me/emails/"+strconv.FormatInt(id, 10)+"/triage",
		map[string]any{"signal": "interview_invitation", "slug": "go-dev-acme-aaaaaaaa", "confidence": 0.9})
	if code != 200 {
		t.Fatalf("triage = %d, want 200 (body %v)", code, body)
	}
	data, _ := body["data"].(map[string]any)
	if got := data["status_signal"]; got != "interview_invitation" {
		t.Errorf("status_signal = %v, want interview_invitation", got)
	}
	if got := data["linked_slug"]; got != "go-dev-acme-aaaaaaaa" {
		t.Errorf("linked_slug = %v, want the linked application", got)
	}
	if got := data["link_source"]; got != "agent" {
		t.Errorf("link_source = %v, want agent", got)
	}
	if got := f.stageOf(jobID); got != "interview" {
		t.Errorf("stage = %q, want interview — a forward signal must advance the application", got)
	}

	// The triaged message leaves the work queue.
	_, listing := f.callKey("GET", "/api/v1/me/inbox?unclassified=1", nil)
	if rows := messages(t, listing); len(rows) != 0 {
		t.Errorf("triaged message still in the work queue: %d rows", len(rows))
	}
}

// TestTriageNeverResurrectsASettledApplication asserts the guard that cost a prod
// incident once: a forward signal onto a rejected application must not drag it
// back into the active pipeline.
func TestTriageNeverResurrectsASettledApplication(t *testing.T) {
	f := newHarnessInboxFixture(t, "settled@example.test")
	jobID := f.applyToJob("go-dev-acme-bbbbbbbb", "rejected")
	id := f.seedEmail(f.userID, "external", "t-2", "Interview invitation", "are you free?")

	if code, _ := f.callKey("POST", "/api/v1/me/emails/"+strconv.FormatInt(id, 10)+"/triage",
		map[string]any{"signal": "interview_invitation", "slug": "go-dev-acme-bbbbbbbb"}); code != 200 {
		t.Fatalf("triage = %d, want 200", code)
	}
	if got := f.stageOf(jobID); got != "rejected" {
		t.Errorf("stage = %q, want it left rejected", got)
	}
}

// TestTriageRejectsBadInput asserts an out-of-vocabulary signal and an unknown
// slug change nothing — the agent is not trusted to invent labels or job ids.
func TestTriageRejectsBadInput(t *testing.T) {
	f := newHarnessInboxFixture(t, "badtriage@example.test")
	id := f.seedEmail(f.userID, "external", "t-3", "Something", "body")
	path := "/api/v1/me/emails/" + strconv.FormatInt(id, 10) + "/triage"

	if code, _ := f.callKey("POST", path, map[string]any{"signal": "vibes"}); code != 400 {
		t.Errorf("unknown signal = %d, want 400", code)
	}
	if code, _ := f.callKey("POST", path, map[string]any{"signal": "rejection", "slug": "no-such-job"}); code != 404 {
		t.Errorf("unknown slug = %d, want 404", code)
	}
	var signal *string
	if err := f.pool.QueryRow(context.Background(),
		`SELECT status_signal FROM emails WHERE id = $1`, id).Scan(&signal); err != nil {
		t.Fatalf("read signal: %v", err)
	}
	if signal != nil {
		t.Errorf("a rejected triage still wrote signal %q", *signal)
	}
}

// TestTriageIsScopedToTheCaller asserts a key cannot triage another user's mail.
func TestTriageIsScopedToTheCaller(t *testing.T) {
	f := newHarnessInboxFixture(t, "triageowner@example.test")
	var strangerID int64
	if err := f.pool.QueryRow(context.Background(),
		`INSERT INTO users (email) VALUES ('triagestranger@example.test') RETURNING id`).Scan(&strangerID); err != nil {
		t.Fatalf("seed stranger: %v", err)
	}
	theirs := f.seedEmail(strangerID, "external", "t-4", "Theirs", "body")

	if code, _ := f.callKey("POST", "/api/v1/me/emails/"+strconv.FormatInt(theirs, 10)+"/triage",
		map[string]any{"signal": "offer"}); code != 404 {
		t.Errorf("triaging another user's message = %d, want 404", code)
	}
}

// A classify-only triage keeps the link — and must keep the confidence that link
// was made with. The two travel together: a row reading link_source='agent' with a
// NULL match_confidence claims a link nobody can say how sure they were about, and
// the inbox renders that confidence beside the chip.
//
// Reachable from two callers now (a user's own harness and the in-app assistant),
// so the classify-then-reclassify sequence is no longer hypothetical.
func TestTriageWithoutALinkKeepsTheConfidenceOfTheExistingOne(t *testing.T) {
	f := newHarnessInboxFixture(t, "confidence@example.test")
	f.applyToJob("go-dev-acme-cccccccc", "applied")
	id := f.seedEmail(f.userID, "external", "t-conf", "Interview invitation", "are you free?")
	path := "/api/v1/me/emails/" + strconv.FormatInt(id, 10) + "/triage"

	if code, body := f.callKey("POST", path,
		map[string]any{"signal": "interview_invitation", "slug": "go-dev-acme-cccccccc", "confidence": 0.9}); code != 200 {
		t.Fatalf("first triage = %d, want 200 (body %v)", code, body)
	}

	// Re-classify without deciding the link and without restating a confidence.
	if code, body := f.callKey("POST", path, map[string]any{"signal": "assessment"}); code != 200 {
		t.Fatalf("second triage = %d, want 200 (body %v)", code, body)
	}

	var confidence *float32
	var linkSource *string
	if err := f.pool.QueryRow(context.Background(),
		`SELECT match_confidence, link_source FROM emails WHERE id = $1`, id).Scan(&confidence, &linkSource); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if linkSource == nil || *linkSource != "agent" {
		t.Fatalf("link_source = %v, want the link kept", linkSource)
	}
	if confidence == nil {
		t.Fatal("match_confidence was wiped by a triage that never touched the link")
	}
	if *confidence != 0.9 {
		t.Errorf("match_confidence = %v, want the 0.9 the link was made with", *confidence)
	}
}
