//go:build integration

// Integration test for SetGmailStatus's own guarantee: every caller that marks a Google
// grant needing reconsent — gmail-sync, cal-sync, mentor-calendar-write, and
// mentor-busy-sync alike — shares this one query, so the busy-sync opt-in flag clearing
// alongside it belongs here rather than duplicated in each caller.
// Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"testing"
)

// SetGmailStatus is called with "needs_reconsent" from four different packages
// (internal/application/gmailsync, internal/application/calsync,
// internal/engage/mentorship, internal/engage/mentorship/busysync) whenever the ONE
// shared refresh token this account holds fails. mentor_busy_sync_opted_in exists solely
// to gate a consent that same token covers, so ANY caller marking needs_reconsent must
// invalidate it — otherwise a later reconnect through an unrelated flow (which restores
// status to 'connected' with calendar.readonly still granted) would silently resurrect a
// consent the mentor never re-gave, regardless of which feature detected the revocation.
func TestSetGmailStatusNeedsReconsentClearsMentorBusySyncOptIn(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	var uid int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('gmail-status-clear@example.test') RETURNING id`).Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO gmail_connections (user_id, email, refresh_token_enc, status, mentor_busy_sync_opted_in)
		 VALUES ($1, '', 'enc', 'connected', true)`, uid); err != nil {
		t.Fatalf("seed connection: %v", err)
	}

	if err := q.SetGmailStatus(ctx, SetGmailStatusParams{UserID: uid, Status: "needs_reconsent"}); err != nil {
		t.Fatalf("SetGmailStatus: %v", err)
	}

	conn, err := q.GetGmailConnection(ctx, uid)
	if err != nil {
		t.Fatalf("GetGmailConnection: %v", err)
	}
	if conn.Status != "needs_reconsent" {
		t.Errorf("status = %q, want needs_reconsent", conn.Status)
	}
	if conn.MentorBusySyncOptedIn {
		t.Error("mentor_busy_sync_opted_in survived a move to needs_reconsent")
	}
}

// A status other than needs_reconsent must not touch the flag — this query's only other
// caller (UpsertGmailConnection aside) sets 'connected' on an ordinary reconnect, and that
// path already has its own, deliberate rule for the flag (untouched, since only this
// feature's own connect callback may set it true).
func TestSetGmailStatusOtherStatusesLeaveTheOptInFlagAlone(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	var uid int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('gmail-status-keep@example.test') RETURNING id`).Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO gmail_connections (user_id, email, refresh_token_enc, status, mentor_busy_sync_opted_in)
		 VALUES ($1, '', 'enc', 'needs_reconsent', true)`, uid); err != nil {
		t.Fatalf("seed connection: %v", err)
	}

	if err := q.SetGmailStatus(ctx, SetGmailStatusParams{UserID: uid, Status: "connected"}); err != nil {
		t.Fatalf("SetGmailStatus: %v", err)
	}

	conn, err := q.GetGmailConnection(ctx, uid)
	if err != nil {
		t.Fatalf("GetGmailConnection: %v", err)
	}
	if !conn.MentorBusySyncOptedIn {
		t.Error("mentor_busy_sync_opted_in was cleared by a status other than needs_reconsent")
	}
}
