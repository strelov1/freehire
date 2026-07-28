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
