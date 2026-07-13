//go:build integration

// Integration tests for the hosted-mailbox HTTP flow against a real Postgres:
// claim is idempotent (one address per user), the inbox spans Gmail + hosted mail
// with a source filter, and release purges only the hosted mail. Run with:
// go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
)

func TestHostedMailboxEndToEnd(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var uid int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('mbx@example.test') RETURNING id`).Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	// A Gmail message so we can prove release purges only hosted mail.
	if _, err := pool.Exec(ctx,
		`INSERT INTO emails (user_id, source, external_id, subject, subject_norm, body_text, received_at)
		 VALUES ($1, 'gmail', 'g1', 'Gmail msg', 'gmail msg', 'body', now())`, uid); err != nil {
		t.Fatalf("seed gmail email: %v", err)
	}

	iss := auth.NewIssuer("test-secret-that-is-long-enough-0001", time.Hour)
	cookie, _ := iss.Issue(uid)
	h := &API{pool: pool, queries: db.New(pool), issuer: iss, mailDomain: "inbox.freehire.test"}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	ra := auth.RequireAuth(iss)
	app.Get("/api/v1/me/mailbox", ra, h.GetMailbox)
	app.Post("/api/v1/me/mailbox", ra, h.ClaimMailbox)
	app.Delete("/api/v1/me/mailbox", ra, h.ReleaseMailbox)
	app.Get("/api/v1/me/inbox", ra, h.GetInbox)

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

	// Status: no mailbox yet, feature available.
	if code, body := do("GET", "/api/v1/me/mailbox"); code != 200 {
		t.Fatalf("status: %d", code)
	} else if d, _ := body["data"].(map[string]any); d["available"] != true || d["address"] != nil {
		t.Errorf("initial status = %v", body["data"])
	}

	// Claim allocates the bare-handle address.
	_, body := do("POST", "/api/v1/me/mailbox")
	d, _ := body["data"].(map[string]any)
	addr, _ := d["address"].(string)
	if addr != "mbx@inbox.freehire.test" {
		t.Fatalf("claimed address = %q", addr)
	}

	// Re-claim is idempotent — same address, still one row.
	_, body = do("POST", "/api/v1/me/mailbox")
	if d2, _ := body["data"].(map[string]any); d2["address"] != addr {
		t.Errorf("re-claim address = %v, want %q", body["data"], addr)
	}
	var count int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM mailboxes WHERE user_id=$1`, uid).Scan(&count)
	if count != 1 {
		t.Errorf("mailboxes rows = %d, want 1", count)
	}

	// A hosted message joins the same inbox as the Gmail one.
	if _, err := pool.Exec(ctx,
		`INSERT INTO emails (user_id, source, external_id, subject, subject_norm, body_text, received_at)
		 VALUES ($1, 'hosted', 'h1', 'Hosted msg', 'hosted msg', 'body', now())`, uid); err != nil {
		t.Fatalf("seed hosted email: %v", err)
	}
	if _, body := do("GET", "/api/v1/me/inbox"); func() bool { g, _ := body["data"].([]any); return len(g) != 2 }() {
		t.Error("inbox should list 2 messages (gmail + hosted)")
	}
	// Source filter narrows to the hosted account only.
	if _, body := do("GET", "/api/v1/me/inbox?source=hosted"); func() bool { g, _ := body["data"].([]any); return len(g) != 1 }() {
		t.Error("hosted filter should list 1 message")
	}

	// Release purges hosted mail + the mailbox; Gmail mail survives.
	if code, _ := do("DELETE", "/api/v1/me/mailbox"); code != 200 {
		t.Errorf("release: %d", code)
	}
	var hosted, gmail int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM emails WHERE user_id=$1 AND source='hosted'`, uid).Scan(&hosted)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM emails WHERE user_id=$1 AND source='gmail'`, uid).Scan(&gmail)
	if hosted != 0 {
		t.Errorf("hosted mail not purged: %d", hosted)
	}
	if gmail != 1 {
		t.Errorf("gmail mail should survive release: %d", gmail)
	}
	if _, body := do("GET", "/api/v1/me/mailbox"); func() bool { d, _ := body["data"].(map[string]any); return d["address"] != nil }() {
		t.Error("mailbox still present after release")
	}
}
