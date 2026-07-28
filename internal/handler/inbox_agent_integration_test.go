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

// agentInboxFixture is one signed-in user with an inbox, an app wired through the
// real route registration (so the tests pin what register mounts, not a
// hand-wired middleware), and both a session cookie and a full-scope API key.
type agentInboxFixture struct {
	t      *testing.T
	pool   *pgxpool.Pool
	q      *db.Queries
	app    *fiber.App
	userID int64
	cookie string
	key    string
}

func newAgentInboxFixture(t *testing.T, email string) *agentInboxFixture {
	t.Helper()
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)

	var uid int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&uid); err != nil {
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

	h := &inboxHandlers{queries: queries, mailDomain: "inbox.freehire.test"}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	h.register(app.Group("/api/v1"), middleware{
		cookie: auth.RequireAuth(iss, testVersions),
		key:    auth.RequireAuthOrKey(iss, testVersions, apiKeys{queries}),
	})

	return &agentInboxFixture{t: t, pool: pool, q: queries, app: app, userID: uid, cookie: cookie, key: token}
}

// callKey issues a request authenticated by the API key — the harness's credential.
func (f *agentInboxFixture) callKey(method, path string, body any) (int, map[string]any) {
	f.t.Helper()
	return f.call(method, path, body, func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+f.key)
	})
}

// callAnon issues a request with no credential at all.
func (f *agentInboxFixture) callAnon(method, path string) (int, map[string]any) {
	f.t.Helper()
	return f.call(method, path, nil, func(*http.Request) {})
}

func (f *agentInboxFixture) call(method, path string, body any, auth func(*http.Request)) (int, map[string]any) {
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
func (f *agentInboxFixture) seedEmail(userID int64, source, externalID, subject, body string) int64 {
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
	f := newAgentInboxFixture(t, "keyed@example.test")
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
	f := newAgentInboxFixture(t, "bodies@example.test")
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
	f := newAgentInboxFixture(t, "htmlonly@example.test")
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
	f := newAgentInboxFixture(t, "queue@example.test")
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
	f := newAgentInboxFixture(t, "source@example.test")
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
	f := newAgentInboxFixture(t, "cap@example.test")

	_, body := f.callKey("GET", "/api/v1/me/inbox?body=1&limit=500", nil)
	meta, _ := body["meta"].(map[string]any)
	if limit, _ := meta["limit"].(float64); limit != 50 {
		t.Errorf("limit = %v, want it clamped to 50", limit)
	}
}
