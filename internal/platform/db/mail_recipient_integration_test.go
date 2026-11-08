//go:build integration

package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// insertVerifiedUser seeds an account optional mail may reach: mail_recipient reads ”
// for an unverified address, which is what insertUser leaves.
func insertVerifiedUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	uid := insertUser(t, pool, email)
	if _, err := pool.Exec(context.Background(), `UPDATE users SET email_verified = true WHERE id = $1`, uid); err != nil {
		t.Fatalf("verify user: %v", err)
	}
	return uid
}

// mail_recipient (migration 0247) is the one rule every optional mail reads: an
// unverified address or an account idle for 180 days gets ”, which each sender
// soft-skips. The session check is what keeps an account active.
func TestMailRecipient(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	seed := func(email string, verified bool, idle string) int64 {
		uid := insertUser(t, pool, email)
		if _, err := pool.Exec(ctx, `UPDATE users SET email_verified = $2, last_active_at = now() - $3::interval WHERE id = $1`, uid, verified, idle); err != nil {
			t.Fatalf("seed %s: %v", email, err)
		}
		return uid
	}
	recipient := func(uid int64) string {
		var got string
		if err := pool.QueryRow(ctx, `SELECT mail_recipient(email, email_verified, last_active_at) FROM users WHERE id = $1`, uid).Scan(&got); err != nil {
			t.Fatalf("mail_recipient: %v", err)
		}
		return got
	}

	active := seed("mr-active@example.com", true, "1 day")
	unverified := seed("mr-unverified@example.com", false, "1 day")
	idle := seed("mr-idle@example.com", true, "181 days")

	if got := recipient(active); got != "mr-active@example.com" {
		t.Errorf("active verified: got %q", got)
	}
	if got := recipient(unverified); got != "" {
		t.Errorf("unverified: got %q, want ''", got)
	}
	if got := recipient(idle); got != "" {
		t.Errorf("idle 181 days: got %q, want ''", got)
	}

	// A signed-in request brings the idle account back.
	if _, err := q.GetUserTokenVersion(ctx, idle); err != nil {
		t.Fatalf("GetUserTokenVersion: %v", err)
	}
	if got := recipient(idle); got != "mr-idle@example.com" {
		t.Errorf("after a session check: got %q, want the address", got)
	}

	// So does a request made with an API key, which never passes the session check.
	keyUser := seed("mr-key@example.com", true, "200 days")
	if _, err := q.CreateAPIKey(ctx, CreateAPIKeyParams{UserID: keyUser, Name: "cli", TokenHash: "mr-hash", TokenPrefix: "fhk_mr", Scope: "full"}); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if _, err := q.AuthenticateAPIKey(ctx, "mr-hash"); err != nil {
		t.Fatalf("AuthenticateAPIKey: %v", err)
	}
	if got := recipient(keyUser); got != "mr-key@example.com" {
		t.Errorf("after an API-key request: got %q, want the address", got)
	}

	// The broadcast audience applies the same rule.
	n, err := q.CountBroadcastCandidates(ctx, CountBroadcastCandidatesParams{Campaign: "mr-test", Half: ""})
	if err != nil {
		t.Fatalf("CountBroadcastCandidates: %v", err)
	}
	var want int64
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users u LEFT JOIN notification_settings ns ON ns.user_id = u.id
		 WHERE mail_recipient(u.email, u.email_verified, u.last_active_at) <> '' AND COALESCE(ns.news_email_enabled, true)
		   AND NOT EXISTS (SELECT 1 FROM broadcast_emails be WHERE be.user_id = u.id AND be.campaign = 'mr-test')`).Scan(&want); err != nil {
		t.Fatal(err)
	}
	if n != want {
		t.Errorf("broadcast audience = %d, want the %d mailable accounts", n, want)
	}
}
