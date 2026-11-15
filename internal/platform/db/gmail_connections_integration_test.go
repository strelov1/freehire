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

// The mail connect writes its token and the scopes Google reported for it in one
// statement. They used to be two (UpsertGmailConnection, then RecordGrantScopes), and a
// calendar connect landing between them paired its token with the mail grant's scopes,
// so cal-sync skipped the account. The row must hold the scopes the moment the token
// lands, and an exchange that reported none must keep what was recorded.
func TestUpsertGmailConnectionRecordsScopesWithTheToken(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	var uid int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('gmail-upsert-scopes@example.test') RETURNING id`).Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	const mail, calendar = "https://www.googleapis.com/auth/gmail.readonly", "https://www.googleapis.com/auth/calendar.events"
	if err := q.UpsertCalendarGrant(ctx, UpsertCalendarGrantParams{
		UserID: uid, RefreshTokenEnc: "cal-token", Scopes: []string{calendar},
	}); err != nil {
		t.Fatalf("UpsertCalendarGrant: %v", err)
	}

	if err := q.UpsertGmailConnection(ctx, UpsertGmailConnectionParams{
		UserID: uid, Email: "me@example.test", RefreshTokenEnc: "mail-token", Scopes: []string{mail, calendar},
	}); err != nil {
		t.Fatalf("UpsertGmailConnection: %v", err)
	}
	listed, err := q.ListCalendarConnections(ctx, calendar)
	if err != nil {
		t.Fatalf("ListCalendarConnections: %v", err)
	}
	if len(listed) != 1 || listed[0] != uid {
		t.Errorf("calendar connections = %v, want [%d]", listed, uid)
	}
	var scopes []string
	if err := pool.QueryRow(ctx, `SELECT scopes FROM gmail_connections WHERE user_id = $1`, uid).Scan(&scopes); err != nil {
		t.Fatalf("read scopes: %v", err)
	}
	if len(scopes) != 2 || scopes[0] != mail || scopes[1] != calendar {
		t.Errorf("scopes = %v, want [%s %s]", scopes, mail, calendar)
	}

	if err := q.UpsertGmailConnection(ctx, UpsertGmailConnectionParams{
		UserID: uid, Email: "me@example.test", RefreshTokenEnc: "mail-token-2", Scopes: nil,
	}); err != nil {
		t.Fatalf("UpsertGmailConnection without scopes: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT scopes FROM gmail_connections WHERE user_id = $1`, uid).Scan(&scopes); err != nil {
		t.Fatalf("read scopes: %v", err)
	}
	if len(scopes) != 2 {
		t.Errorf("scopes after an exchange that reported none = %v, want the recorded two kept", scopes)
	}
}
