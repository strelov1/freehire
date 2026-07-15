//go:build integration

// Integration tests for the inbox read-side triage controls against a real
// Postgres: the unread-only and label filters, mark-all-read (respecting the
// active filters), and per-message soft-delete + restore. Run with:
// go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
)

func TestInboxReadSideEndToEnd(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var uid int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('triage@example.test') RETURNING id`).Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	// Three messages: an unread rejection, a read interview invite, and an unread
	// message with no classification — enough to exercise every filter.
	seed := func(ext, subject, status string, read bool) int64 {
		var readAt any // nil → NULL (unread)
		if read {
			readAt = time.Now()
		}
		var id int64
		if err := pool.QueryRow(ctx,
			`INSERT INTO emails (user_id, source, external_id, subject, body_text, status_signal, received_at, read_at)
			 VALUES ($1, 'hosted', $2, $3, 'body', $4, now(), $5) RETURNING id`,
			uid, ext, subject, status, readAt).Scan(&id); err != nil {
			t.Fatalf("seed email %s: %v", ext, err)
		}
		return id
	}
	rejID := seed("m-rej", "Rejection", "rejection", false)
	_ = seed("m-int", "Interview", "interview_invitation", true)
	_ = seed("m-plain", "Plain", "", false)

	iss := auth.NewIssuer("test-secret-that-is-long-enough-0001", time.Hour)
	cookie, _ := iss.Issue(uid)
	h := &API{pool: pool, queries: db.New(pool), issuer: iss, mailDomain: "inbox.freehire.test"}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	ra := auth.RequireAuth(iss)
	app.Get("/api/v1/me/inbox", ra, h.GetInbox)
	app.Get("/api/v1/me/emails/:id", ra, h.GetEmail)
	app.Post("/api/v1/me/inbox/read-all", ra, h.MarkAllReadInbox)
	app.Post("/api/v1/me/emails/:id/delete", ra, h.DeleteEmail)
	app.Post("/api/v1/me/emails/:id/restore", ra, h.RestoreEmail)

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
	listLen := func(path string) int {
		_, body := do("GET", path)
		g, _ := body["data"].([]any)
		return len(g)
	}

	// Baseline: all three visible.
	if n := listLen("/api/v1/me/inbox"); n != 3 {
		t.Fatalf("baseline inbox = %d, want 3", n)
	}

	// Unread-only filter: hides the read interview invite.
	if n := listLen("/api/v1/me/inbox?unread=1"); n != 2 {
		t.Errorf("unread filter = %d, want 2", n)
	}

	// Label filter: only the rejection.
	if n := listLen("/api/v1/me/inbox?status=rejection"); n != 1 {
		t.Errorf("label filter = %d, want 1", n)
	}
	// Unknown label is a 400, not a silent empty list.
	if code, _ := do("GET", "/api/v1/me/inbox?status=bogus"); code != 400 {
		t.Errorf("unknown label status = %d, want 400", code)
	}

	// Mark-all-read under the unread filter marks the two unread ones read; the
	// unread list then empties. The interview invite was already read.
	if code, _ := do("POST", "/api/v1/me/inbox/read-all?unread=1"); code != 200 {
		t.Errorf("mark-all-read status = %d", code)
	}
	if n := listLen("/api/v1/me/inbox?unread=1"); n != 0 {
		t.Errorf("unread after mark-all = %d, want 0", n)
	}

	// Soft-delete the rejection: it drops out of the listing.
	if code, _ := do("POST", "/api/v1/me/emails/"+strconv.FormatInt(rejID, 10)+"/delete"); code != 200 {
		t.Errorf("delete status = %d", code)
	}
	if n := listLen("/api/v1/me/inbox"); n != 2 {
		t.Errorf("inbox after delete = %d, want 2", n)
	}
	// A soft-deleted message is not readable by direct id either — 404, consistent
	// with the listing that now hides it.
	if code, _ := do("GET", "/api/v1/me/emails/"+strconv.FormatInt(rejID, 10)); code != 404 {
		t.Errorf("GET deleted email = %d, want 404", code)
	}
	// Restore brings it back.
	if code, _ := do("POST", "/api/v1/me/emails/"+strconv.FormatInt(rejID, 10)+"/restore"); code != 200 {
		t.Errorf("restore status = %d", code)
	}
	if n := listLen("/api/v1/me/inbox"); n != 3 {
		t.Errorf("inbox after restore = %d, want 3", n)
	}

	// Deleting another user's message is a 404.
	var otherUID int64
	_ = pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('other@example.test') RETURNING id`).Scan(&otherUID)
	var otherID int64
	_ = pool.QueryRow(ctx,
		`INSERT INTO emails (user_id, source, external_id, subject, body_text, received_at)
		 VALUES ($1, 'hosted', 'x1', 'Theirs', 'body', now()) RETURNING id`, otherUID).Scan(&otherID)
	if code, _ := do("POST", "/api/v1/me/emails/"+strconv.FormatInt(otherID, 10)+"/delete"); code != 404 {
		t.Errorf("cross-user delete = %d, want 404", code)
	}
}

// TestSoftDeleteSurvivesResync proves a soft-deleted Gmail message stays deleted
// when the sync worker re-encounters it: UpsertEmail is ON CONFLICT DO NOTHING,
// so it never clears deleted_at.
func TestSoftDeleteSurvivesResync(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	q := db.New(pool)

	var uid int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('resync@example.test') RETURNING id`).Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	now := pgtype.Timestamptz{Time: time.Now(), Valid: true}
	if err := q.UpsertEmail(ctx, db.UpsertEmailParams{UserID: uid, ExternalID: "g-sync", Subject: "First", ReceivedAt: now}); err != nil {
		t.Fatalf("initial upsert: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE emails SET deleted_at = now() WHERE user_id=$1 AND external_id='g-sync'`, uid); err != nil {
		t.Fatalf("soft-delete: %v", err)
	}

	// Re-sync the same message.
	if err := q.UpsertEmail(ctx, db.UpsertEmailParams{UserID: uid, ExternalID: "g-sync", Subject: "First", ReceivedAt: now}); err != nil {
		t.Fatalf("re-sync upsert: %v", err)
	}

	rows, err := q.ListEmails(ctx, db.ListEmailsParams{UserID: uid, Lim: 20})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("re-synced deleted message reappeared: %d rows, want 0", len(rows))
	}
}
