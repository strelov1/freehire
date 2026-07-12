//go:build integration

// Integration tests for the Gmail inbox HTTP flow against a real Postgres: the
// inbox groups mail by normalized subject (Re:/Fwd: folded), a message body is
// caller-scoped (another user's is a 404), and disconnect purges the connection
// and all synced mail. Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
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
	insEmail := func(u int64, msgID, subject, subjectNorm, body string) int64 {
		var id int64
		if err := pool.QueryRow(ctx,
			`INSERT INTO emails (user_id, gmail_msg_id, from_addr, from_name, subject, subject_norm, body_text, received_at)
			 VALUES ($1, $2, 'no-reply@ashbyhq.com', 'Acme', $3, $4, $5, now()) RETURNING id`,
			u, msgID, subject, subjectNorm, body).Scan(&id); err != nil {
			t.Fatalf("seed email: %v", err)
		}
		return id
	}
	m1 := insEmail(uid, "m1", "Thank you for applying to Acme", "thank you for applying to acme", "Hi Ilya")
	insEmail(uid, "m2", "Re: Thank you for applying to Acme", "thank you for applying to acme", "Reply body")
	insEmail(uid, "m3", "Interview invite", "interview invite", "Come chat")
	foreign := insEmail(other, "m4", "Other mail", "other mail", "secret")

	iss := auth.NewIssuer("test-secret-that-is-long-enough-0001", time.Hour)
	cookie, _ := iss.Issue(uid)
	h := &API{pool: pool, queries: db.New(pool), issuer: iss}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	ra := auth.RequireAuth(iss)
	app.Get("/api/v1/me/gmail", ra, h.GmailStatus)
	app.Delete("/api/v1/me/gmail", ra, h.GmailDisconnect)
	app.Get("/api/v1/me/inbox", ra, h.GetInbox)
	app.Get("/api/v1/me/inbox/group", ra, h.GetInboxGroup)
	app.Get("/api/v1/me/emails/:id", ra, h.GetEmail)

	do := func(method, path string) (int, map[string]any) {
		r := httptest.NewRequest(method, path, nil)
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		resp, err := app.Test(r, -1)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		b, _ := io.ReadAll(resp.Body)
		var body map[string]any
		_ = json.Unmarshal(b, &body)
		return resp.StatusCode, body
	}

	// Status: connected.
	if code, body := do("GET", "/api/v1/me/gmail"); code != 200 {
		t.Fatalf("status: %d", code)
	} else if d, _ := body["data"].(map[string]any); d["connected"] != true || d["email"] != "gm@gmail.com" {
		t.Errorf("status data = %v", body["data"])
	}

	// Inbox: two groups; the applying group folds m1+m2 (count 2).
	_, body := do("GET", "/api/v1/me/inbox")
	groups, _ := body["data"].([]any)
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(groups))
	}
	var applyCount float64
	for _, g := range groups {
		if gm, _ := g.(map[string]any); gm["key"] == "thank you for applying to acme" {
			applyCount, _ = gm["message_count"].(float64)
		}
	}
	if applyCount != 2 {
		t.Errorf("apply group count = %v, want 2", applyCount)
	}

	// Group thread: two messages.
	_, body = do("GET", "/api/v1/me/inbox/group?key="+url.QueryEscape("thank you for applying to acme"))
	if msgs, _ := body["data"].([]any); len(msgs) != 2 {
		t.Errorf("group messages = %d, want 2", len(msgs))
	}

	// Search: "interview" matches only the interview group.
	_, body = do("GET", "/api/v1/me/inbox?q=interview")
	if g, _ := body["data"].([]any); len(g) != 1 {
		t.Errorf("search 'interview' groups = %d, want 1", len(g))
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
}
