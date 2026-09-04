//go:build integration

// Integration test for account deletion against a real Postgres: it seeds an account
// with something in every user-owned table that matters, deletes it through the HTTP
// endpoint, and asserts the promise the feature makes — nothing of the member's is
// left, in the database or in object storage, while other members' content and the
// moderation trail survive. The FK cascade is doing most of the work, which is
// exactly why it needs a test: a future table added without ON DELETE CASCADE would
// otherwise leave rows behind silently.
// Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/identity/accountdelete"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

// memBlobs is an in-memory blobstore.Store that records what was deleted.
type memBlobs struct {
	objects map[string]bool
	failOn  string
}

func newMemBlobs(keys ...string) *memBlobs {
	m := &memBlobs{objects: map[string]bool{}}
	for _, k := range keys {
		m.objects[k] = true
	}
	return m
}

func (m *memBlobs) Put(context.Context, string, string, io.Reader, int64) error { return nil }
func (m *memBlobs) Get(context.Context, string) (io.ReadCloser, error)          { return nil, nil }

func (m *memBlobs) Delete(_ context.Context, key string) error {
	if m.failOn != "" && key == m.failOn {
		return io.ErrUnexpectedEOF
	}
	delete(m.objects, key)
	return nil
}

func newDeleteAccountApp(pool *pgxpool.Pool, iss *auth.Issuer, blobs *memBlobs) *fiber.App {
	queries := db.New(pool)
	h := &authHandlers{
		queries:       queries,
		issuer:        iss,
		accountEmails: queries,
		accountDelete: accountdelete.New(accountdelete.NewQueriesRepository(queries), blobs, nil),
	}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Delete("/api/v1/me", auth.RequireAuth(iss, queries), h.DeleteAccount)
	// Mounted to prove a token for the deleted account no longer authenticates.
	app.Get("/api/v1/me/probe", auth.RequireAuth(iss, queries), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})
	return app
}

func deleteAccountRequest(t *testing.T, app *fiber.App, cookie, confirm string) *http.Response {
	t.Helper()
	r := httptest.NewRequest(fiber.MethodDelete, "/api/v1/me", strings.NewReader(`{"email":"`+confirm+`"}`))
	r.Header.Set("Content-Type", "application/json")
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	}
	resp, err := app.Test(r)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	return resp
}

// countRows is the assertion workhorse: how many rows in `table` still reference the
// user through `column`.
func countRows(t *testing.T, pool *pgxpool.Pool, table, column string, userID int64) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM `+table+` WHERE `+column+` = $1`, userID).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func TestDeleteAccountEndToEnd(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	const leaverEmail = "leaver@example.test"
	var leaver, stayer int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email, role, resume_object_key, resume_uploaded_at, photo_object_key, photo_uploaded_at)
		 VALUES ($1, 'moderator', 'resumes/leaver', now(), 'photos/leaver', now()) RETURNING id`, leaverEmail).Scan(&leaver); err != nil {
		t.Fatalf("seed leaver: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('stayer@example.test') RETURNING id`).Scan(&stayer); err != nil {
		t.Fatalf("seed stayer: %v", err)
	}

	// Subjects the seeded rows hang off.
	if _, err := pool.Exec(ctx, `INSERT INTO companies (slug, name, job_count) VALUES ('acme', 'Acme', 1)`); err != nil {
		t.Fatalf("seed company: %v", err)
	}
	var jobID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO jobs (source, external_id, url, title, public_slug, created_by)
		 VALUES ('greenhouse', 'eng:1', 'http://example.test', 'Engineer', 'engineer-eng-1', $1) RETURNING id`,
		leaver).Scan(&jobID); err != nil {
		t.Fatalf("seed job: %v", err)
	}

	// One row per user-owned surface, so a table that loses its cascade shows up here.
	seed := []struct {
		what string
		sql  string
		args []any
	}{
		{"api key", `INSERT INTO api_keys (user_id, name, token_hash, token_prefix) VALUES ($1, 'cli', 'hash', 'fh_abcd')`, []any{leaver}},
		{"identity", `INSERT INTO user_identities (provider, provider_user_id, user_id) VALUES ('google', 'g-1', $1)`, []any{leaver}},
		{"job interaction", `INSERT INTO user_jobs (user_id, job_id, saved_at) VALUES ($1, $2, now())`, []any{leaver, jobID}},
		{"job analysis", `INSERT INTO user_job_analysis (user_id, job_id, analysis, model) VALUES ($1, $2, '{}'::jsonb, 'test-model')`, []any{leaver, jobID}},
		{"cv", `INSERT INTO cvs (user_id, title, data) VALUES ($1, 'CV', '{}'::jsonb)`, []any{leaver}},
		{"credit balance", `INSERT INTO credit_balances (user_id, period, remaining) VALUES ($1, '2026-07', 10)`, []any{leaver}},
		{"credit ledger", `INSERT INTO credit_ledger (user_id, period, kind, delta) VALUES ($1, '2026-07', 'grant', 10)`, []any{leaver}},
		// The plan's two tables and the plan itself. The old credit pair above is still
		// seeded because it still exists: it is written by nobody now, dropped in a later
		// change, and until then deletion has to reach both.
		{"usage counter", `INSERT INTO usage_daily (user_id, feature, day, used) VALUES ($1, 'match', CURRENT_DATE, 2)`, []any{leaver}},
		{"usage ledger", `INSERT INTO usage_ledger (user_id, feature, day, kind, delta, ref) VALUES ($1, 'match', CURRENT_DATE, 'consume', 1, 'job-1')`, []any{leaver}},
		{"pro subscription", `UPDATE users SET pro_until_stripe = now() + interval '30 days' WHERE id = $1`, []any{leaver}},
		{"saved search", `INSERT INTO saved_searches (user_id, name, query) VALUES ($1, 'go jobs', 'q=go')`, []any{leaver}},
		{"profile", `INSERT INTO user_profiles (user_id, specializations, skills) VALUES ($1, ARRAY['backend'], ARRAY['go'])`, []any{leaver}},
		{"mailbox", `INSERT INTO mailboxes (user_id, address) VALUES ($1, 'leaver@mail.test')`, []any{leaver}},
		{"gmail connection", `INSERT INTO gmail_connections (user_id, email, refresh_token_enc) VALUES ($1, 'leaver@gmail.test', 'enc')`, []any{leaver}},
		{"hosted email", `INSERT INTO emails (user_id, source, external_id, s3_key, received_at) VALUES ($1, 'hosted', 'msg-1', 'mail/leaver/msg-1', now())`, []any{leaver}},
		{"gmail email", `INSERT INTO emails (user_id, source, external_id, received_at) VALUES ($1, 'gmail', 'g-msg-1', now())`, []any{leaver}},
		{"referral offer", `INSERT INTO referral_offers (user_id, company_slug, proof_object_key) VALUES ($1, 'acme', 'referral-proof/leaver/acme.pdf')`, []any{leaver}},
		{"company vote", `INSERT INTO company_votes (user_id, company_slug, vote) VALUES ($1, 'acme', 1)`, []any{leaver}},
		{"notification settings", `INSERT INTO notification_settings (user_id, enabled) VALUES ($1, true)`, []any{leaver}},
		{"persona", `INSERT INTO community_personas (user_id, handle) VALUES ($1, 'quiet-otter')`, []any{leaver}},
		{"stayer persona", `INSERT INTO community_personas (user_id, handle) VALUES ($1, 'brisk-heron')`, []any{stayer}},
	}
	for _, s := range seed {
		if _, err := pool.Exec(ctx, s.sql, s.args...); err != nil {
			t.Fatalf("seed %s: %v", s.what, err)
		}
	}

	// A thread the leaver opened, with a reply from someone who is staying.
	var threadID, stayerReply int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO threads (subject_type, subject_ref, title, body, author_user_id)
		 VALUES ('company', 'acme', 'Interview loop', 'How long?', $1) RETURNING id`, leaver).Scan(&threadID); err != nil {
		t.Fatalf("seed thread: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO thread_replies (thread_id, author_user_id, body) VALUES ($1, $2, 'Four rounds') RETURNING id`,
		threadID, stayer).Scan(&stayerReply); err != nil {
		t.Fatalf("seed reply: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	cookie, _ := iss.Issue(leaver, testTokenVersion)
	blobs := newMemBlobs("resumes/leaver", "photos/leaver", "referral-proof/leaver/acme.pdf", "mail/leaver/msg-1", "resumes/stayer")
	app := newDeleteAccountApp(pool, iss, blobs)

	t.Run("wrong confirmation erases nothing", func(t *testing.T) {
		if resp := deleteAccountRequest(t, app, cookie, "someone@example.test"); resp.StatusCode != fiber.StatusBadRequest {
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
		if countRows(t, pool, "users", "id", leaver) != 1 {
			t.Error("the account was erased despite a bad confirmation")
		}
	})

	t.Run("deletes the account", func(t *testing.T) {
		resp := deleteAccountRequest(t, app, cookie, leaverEmail)
		if resp.StatusCode != fiber.StatusNoContent {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 204 (body: %s)", resp.StatusCode, body)
		}
	})

	t.Run("no user-owned row survives", func(t *testing.T) {
		for _, tbl := range []struct{ table, column string }{
			{"users", "id"},
			{"api_keys", "user_id"},
			{"user_identities", "user_id"},
			{"user_jobs", "user_id"},
			{"user_job_analysis", "user_id"},
			{"cvs", "user_id"},
			{"credit_balances", "user_id"},
			{"credit_ledger", "user_id"},
			// The plan's tables. A daily counter that outlived its owner would follow a
			// new account registered on the same address into a spent allowance it never
			// used — the address is released deliberately, so what hangs off the old id
			// has to go with it.
			{"usage_daily", "user_id"},
			{"usage_ledger", "user_id"},
			{"saved_searches", "user_id"},
			{"user_profiles", "user_id"},
			{"mailboxes", "user_id"},
			{"gmail_connections", "user_id"},
			{"emails", "user_id"},
			{"referral_offers", "user_id"},
			{"company_votes", "user_id"},
			{"notification_settings", "user_id"},
			{"community_personas", "user_id"},
		} {
			if n := countRows(t, pool, tbl.table, tbl.column, leaver); n != 0 {
				t.Errorf("%s still holds %d row(s) for the deleted account", tbl.table, n)
			}
		}
	})

	t.Run("stored objects are gone, other members' are not", func(t *testing.T) {
		for _, key := range []string{"resumes/leaver", "photos/leaver", "referral-proof/leaver/acme.pdf", "mail/leaver/msg-1"} {
			if blobs.objects[key] {
				t.Errorf("object %q survived the deletion", key)
			}
		}
		if !blobs.objects["resumes/stayer"] {
			t.Error("another member's object was deleted")
		}
	})

	t.Run("community content survives de-authored", func(t *testing.T) {
		var authorID *int64
		var title string
		if err := pool.QueryRow(ctx, `SELECT author_user_id, title FROM threads WHERE id = $1`, threadID).Scan(&authorID, &title); err != nil {
			t.Fatalf("thread after deletion: %v (want it to survive)", err)
		}
		if authorID != nil {
			t.Errorf("thread author = %d, want NULL", *authorID)
		}
		var body string
		if err := pool.QueryRow(ctx, `SELECT body FROM thread_replies WHERE id = $1`, stayerReply).Scan(&body); err != nil {
			t.Errorf("the staying member's reply was deleted: %v", err)
		}
	})

	t.Run("moderation trail keeps the record, drops the identity", func(t *testing.T) {
		var createdBy *int64
		if err := pool.QueryRow(ctx, `SELECT created_by FROM jobs WHERE id = $1`, jobID).Scan(&createdBy); err != nil {
			t.Fatalf("job after deletion: %v (want it to survive)", err)
		}
		if createdBy != nil {
			t.Errorf("jobs.created_by = %d, want NULL", *createdBy)
		}
	})

	t.Run("the session no longer authenticates", func(t *testing.T) {
		r := httptest.NewRequest(fiber.MethodGet, "/api/v1/me/probe", nil)
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		resp, err := app.Test(r)
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		if resp.StatusCode != fiber.StatusUnauthorized {
			t.Errorf("status = %d, want 401 for a token whose account is gone", resp.StatusCode)
		}
	})

	t.Run("the email address is free again", func(t *testing.T) {
		var fresh int64
		if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ($1) RETURNING id`, leaverEmail).Scan(&fresh); err != nil {
			t.Fatalf("re-register with the freed address: %v", err)
		}
		if fresh == leaver {
			t.Error("the new account reused the deleted account's id")
		}
		for _, tbl := range []string{"user_jobs", "cvs", "saved_searches"} {
			if n := countRows(t, pool, tbl, "user_id", fresh); n != 0 {
				t.Errorf("the fresh account inherited %d row(s) in %s", n, tbl)
			}
		}
	})
}

// A storage failure must leave the account whole: rows-gone-objects-left cannot be
// repaired, so the endpoint refuses rather than half-deleting.
func TestDeleteAccountAbortsOnStorageFailure(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	const email = "unlucky@example.test"
	var userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email, resume_object_key, resume_uploaded_at) VALUES ($1, 'resumes/unlucky', now()) RETURNING id`,
		email).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	iss := auth.NewIssuer("test-secret", time.Hour)
	cookie, _ := iss.Issue(userID, testTokenVersion)
	blobs := newMemBlobs("resumes/unlucky")
	blobs.failOn = "resumes/unlucky"
	app := newDeleteAccountApp(pool, iss, blobs)

	if resp := deleteAccountRequest(t, app, cookie, email); resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
	if countRows(t, pool, "users", "id", userID) != 1 {
		t.Error("the account was deleted even though its objects could not be erased")
	}
	if !blobs.objects["resumes/unlucky"] {
		t.Error("the object was deleted despite the reported failure")
	}
}
