//go:build integration

// Integration tests for the Gmail inbox HTTP flow against a real Postgres: the
// inbox is a flat per-user message list (search filters it), a message body is
// caller-scoped (another user's is a 404), and disconnect purges the connection
// and all synced mail. Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestGmailInboxEndToEnd(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var uid, other int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('gm@example.test') RETURNING id`).Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('other@example.test') RETURNING id`).Scan(&other); err != nil {
		t.Fatalf("seed other: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO gmail_connections (user_id, email, refresh_token_enc) VALUES ($1, 'gm@gmail.com', 'enc')`, uid); err != nil {
		t.Fatalf("seed connection: %v", err)
	}
	insEmail := func(u int64, msgID, subject, body string) int64 {
		var id int64
		if err := pool.QueryRow(ctx,
			`INSERT INTO emails (user_id, external_id, from_addr, from_name, subject, body_text, received_at)
			 VALUES ($1, $2, 'no-reply@ashbyhq.com', 'Acme', $3, $4, now()) RETURNING id`,
			u, msgID, subject, body).Scan(&id); err != nil {
			t.Fatalf("seed email: %v", err)
		}
		return id
	}
	m1 := insEmail(uid, "m1", "Thank you for applying to Acme", "Hi Ilya")
	insEmail(uid, "m2", "Re: Thank you for applying to Acme", "Reply body")
	insEmail(uid, "m3", "Interview invite", "Come chat")
	foreign := insEmail(other, "m4", "Other mail", "secret")

	iss := auth.NewIssuer("test-secret-that-is-long-enough-0001", time.Hour)
	cookie, _ := iss.Issue(uid, testTokenVersion)
	h := newInboxHandlers(db.New(pool), pool, nil, nil, "", false, "")

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	ra := auth.RequireAuth(iss, testVersions)
	app.Get("/api/v1/me/gmail", ra, h.GmailStatus)
	app.Delete("/api/v1/me/gmail", ra, h.GmailDisconnect)
	app.Get("/api/v1/me/inbox", ra, h.GetInbox)
	app.Get("/api/v1/me/emails/:id", ra, h.GetEmail)

	do := func(method, path string) (int, map[string]any) {
		r := httptest.NewRequest(method, path, nil)
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		resp, err := app.Test(r, -1)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		var body map[string]any
		return decodeJSON(t, resp, &body), body
	}

	// Status: connected.
	if code, body := do("GET", "/api/v1/me/gmail"); code != 200 {
		t.Fatalf("status: %d", code)
	} else if d, _ := body["data"].(map[string]any); d["connected"] != true || d["email"] != "gm@gmail.com" {
		t.Errorf("status data = %v", body["data"])
	}

	// Inbox: a flat list of this user's three messages (m1, m2, m3); m4 is another
	// user's and must not appear.
	_, body := do("GET", "/api/v1/me/inbox")
	if msgs, _ := body["data"].([]any); len(msgs) != 3 {
		t.Fatalf("messages = %d, want 3", len(msgs))
	}

	// Search: "interview" matches only m3.
	_, body = do("GET", "/api/v1/me/inbox?q=interview")
	if m, _ := body["data"].([]any); len(m) != 1 {
		t.Errorf("search 'interview' messages = %d, want 1", len(m))
	}

	// Message body, caller-scoped.
	if code, body := do("GET", fmt.Sprintf("/api/v1/me/emails/%d", m1)); code != 200 {
		t.Errorf("own email: %d", code)
	} else if d, _ := body["data"].(map[string]any); d["body_text"] != "Hi Ilya" {
		t.Errorf("body = %v", body["data"])
	}
	if code, _ := do("GET", fmt.Sprintf("/api/v1/me/emails/%d", foreign)); code != 404 {
		t.Errorf("foreign email: %d, want 404", code)
	}

	// Disconnect purges connection + mail.
	if code, _ := do("DELETE", "/api/v1/me/gmail"); code != 200 {
		t.Errorf("disconnect: %d", code)
	}
	if _, body := do("GET", "/api/v1/me/gmail"); func() bool { d, _ := body["data"].(map[string]any); return d["connected"] == true }() {
		t.Error("still connected after disconnect")
	}
	if _, body := do("GET", "/api/v1/me/inbox"); func() bool { g, _ := body["data"].([]any); return len(g) != 0 }() {
		t.Error("inbox not purged after disconnect")
	}

	// A calendar-only consent (UpsertCalendarGrant) writes the same gmail_connections row
	// with an empty address — GmailStatus must not report that as Mail connected, or the
	// SPA's Mail card shows "Connected" with no address behind it.
	if _, err := pool.Exec(ctx,
		`INSERT INTO gmail_connections (user_id, email, refresh_token_enc, status) VALUES ($1, '', 'enc', 'connected')`, uid); err != nil {
		t.Fatalf("seed calendar-only connection: %v", err)
	}
	if _, body := do("GET", "/api/v1/me/gmail"); func() bool {
		d, _ := body["data"].(map[string]any)
		return d["connected"] == true
	}() {
		t.Error("calendar-only grant (empty email) reported as Mail connected")
	}

	// mentor_calendar_connected must agree with the SAME "connected" test the booking
	// flow's own grant lookup applies (mentorship.GetMentorCalendarGrant): a grant that
	// covers calendar.events but has since been marked needs_reconsent must NOT read as
	// connected here, or a mentor's profile form keeps telling them the meeting link is
	// optional while every new booking silently gets no link at all.
	if _, err := pool.Exec(ctx,
		`UPDATE gmail_connections SET scopes = $2 WHERE user_id = $1`,
		uid, []string{"https://www.googleapis.com/auth/calendar.events"}); err != nil {
		t.Fatalf("seed mentor calendar scope: %v", err)
	}
	if _, body := do("GET", "/api/v1/me/gmail"); func() bool {
		d, _ := body["data"].(map[string]any)
		return d["mentor_calendar_connected"] != true
	}() {
		t.Error("a connected calendar.events grant was not reported as mentor_calendar_connected")
	}
	if _, err := pool.Exec(ctx,
		`UPDATE gmail_connections SET status = 'needs_reconsent' WHERE user_id = $1`, uid); err != nil {
		t.Fatalf("mark needs_reconsent: %v", err)
	}
	if _, body := do("GET", "/api/v1/me/gmail"); func() bool {
		d, _ := body["data"].(map[string]any)
		return d["mentor_calendar_connected"] == true
	}() {
		t.Error("a needs_reconsent grant was still reported as mentor_calendar_connected")
	}

	// mentor_busy_sync_connected needs BOTH the explicit opt-in flag and the shared
	// calendar.readonly scope: the flag alone (opted in, but the grant has since lost or
	// never gained the scope) must not read as connected, since the worker could not
	// actually call the API.
	if _, err := pool.Exec(ctx,
		`UPDATE gmail_connections SET status = 'connected', scopes = '{}', mentor_busy_sync_opted_in = true WHERE user_id = $1`,
		uid); err != nil {
		t.Fatalf("seed opted-in without scope: %v", err)
	}
	if _, body := do("GET", "/api/v1/me/gmail"); func() bool {
		d, _ := body["data"].(map[string]any)
		return d["mentor_busy_sync_connected"] == true
	}() {
		t.Error("opted-in without the calendar.readonly scope was reported as mentor_busy_sync_connected")
	}

	// The scope alone (no opt-in) must not read as connected either — that is precisely
	// the unrelated-grant case this feature exists to distinguish from, since the
	// candidate-side calendar flow requests this very same scope.
	if _, err := pool.Exec(ctx,
		`UPDATE gmail_connections SET scopes = $2, mentor_busy_sync_opted_in = false WHERE user_id = $1`,
		uid, []string{"https://www.googleapis.com/auth/calendar.readonly"}); err != nil {
		t.Fatalf("seed scope without opt-in: %v", err)
	}
	if _, body := do("GET", "/api/v1/me/gmail"); func() bool {
		d, _ := body["data"].(map[string]any)
		return d["mentor_busy_sync_connected"] == true
	}() {
		t.Error("an unrelated calendar.readonly grant (no opt-in) was reported as mentor_busy_sync_connected")
	}

	// Both together: connected.
	if _, err := pool.Exec(ctx,
		`UPDATE gmail_connections SET mentor_busy_sync_opted_in = true WHERE user_id = $1`, uid); err != nil {
		t.Fatalf("seed opted-in with scope: %v", err)
	}
	if _, body := do("GET", "/api/v1/me/gmail"); func() bool {
		d, _ := body["data"].(map[string]any)
		return d["mentor_busy_sync_connected"] != true
	}() {
		t.Error("opted-in with the calendar.readonly scope was not reported as mentor_busy_sync_connected")
	}

	// A needs_reconsent grant must not read as connected even with both the flag and the
	// scope recorded — the same rule mentor_calendar_connected already follows above.
	if _, err := pool.Exec(ctx,
		`UPDATE gmail_connections SET status = 'needs_reconsent' WHERE user_id = $1`, uid); err != nil {
		t.Fatalf("mark needs_reconsent: %v", err)
	}
	if _, body := do("GET", "/api/v1/me/gmail"); func() bool {
		d, _ := body["data"].(map[string]any)
		return d["mentor_busy_sync_connected"] == true
	}() {
		t.Error("a needs_reconsent grant was still reported as mentor_busy_sync_connected")
	}
}
