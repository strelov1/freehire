//go:build integration

// Integration tests for the Gmail inbox HTTP flow against a real Postgres: the
// inbox is a flat per-user message list (search filters it), a message body is
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
			`INSERT INTO emails (user_id, external_id, from_addr, from_name, subject, subject_norm, body_text, received_at)
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
}
