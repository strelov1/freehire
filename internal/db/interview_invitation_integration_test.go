//go:build integration

// Integration tests for the interview-invitation lookup the rehearsal context uses.
// Its two guarantees are SQL ones: it is owner- and vacancy-scoped, and it leaves
// read_at alone — that column means a human saw the message, and an agent sweeping it
// would zero its owner's unread count. Run with:
// go test -tags=integration ./internal/db/
package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// insertInvitation stores one classified message against a vacancy, received at the
// given offset from now, and returns its id.
func insertInvitation(t *testing.T, pool *pgxpool.Pool, userID, jobID int64, externalID, subject, signal string, age time.Duration) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO emails (user_id, source, external_id, from_addr, from_name, subject,
		                     body_text, received_at, job_id, status_signal, classified_at)
		 VALUES ($1, 'external', $2, 'talent@acme.test', 'Acme Talent', $3,
		         'Body of ' || $3, now() - $4::interval, $5, $6, now())
		 RETURNING id`,
		userID, externalID, subject, age.String(), jobID, signal).Scan(&id)
	if err != nil {
		t.Fatalf("insert invitation %s: %v", externalID, err)
	}
	return id
}

// The rehearsal opens on the employer's own description of the interview, so the
// lookup answers with the most recent invitation for that application.
func TestInterviewInvitationReturnsTheLatestForTheVacancy(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedAssistantUser(t, pool, "invited@example.test")
	jobID := insertJob(t, pool, "invitation-vacancy")

	insertInvitation(t, pool, user, jobID, "older", "First call", "interview_invitation", 72*time.Hour)
	newest := insertInvitation(t, pool, user, jobID, "newer", "Tech round with the lead", "interview_invitation", time.Hour)
	insertInvitation(t, pool, user, jobID, "ack", "We received your application", "acknowledgement", time.Minute)

	// A second application, invited more recently than the one we ask about. Without
	// it the vacancy filter could be deleted from the query and this test would still
	// pass — and the failure it guards against is the one that matters most here: a
	// rehearsal opening on another employer's interview.
	otherJob := insertJob(t, pool, "other-vacancy")
	insertInvitation(t, pool, user, otherJob, "other", "Onsite at Globex", "interview_invitation", time.Minute)

	got, err := q.GetInterviewInvitation(ctx, GetInterviewInvitationParams{UserID: user, JobID: jobID})
	if err != nil {
		t.Fatalf("GetInterviewInvitation: %v", err)
	}
	if got.ID != newest {
		t.Errorf("id = %d, want %d (the most recent invitation for THIS vacancy)", got.ID, newest)
	}
	if got.Subject != "Tech round with the lead" {
		t.Errorf("subject = %q, want the newest invitation's", got.Subject)
	}
}

// A trashed invitation is gone, not merely hidden from the web inbox. Resurfacing one
// inside an agent's context is a surprise the candidate has no way to explain.
func TestInterviewInvitationSkipsDeletedMail(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedAssistantUser(t, pool, "trashed-invite@example.test")
	jobID := insertJob(t, pool, "trashed-invitation-vacancy")

	live := insertInvitation(t, pool, user, jobID, "live", "Second round", "interview_invitation", 48*time.Hour)
	trashed := insertInvitation(t, pool, user, jobID, "trashed", "Cancelled round", "interview_invitation", time.Hour)
	if _, err := pool.Exec(ctx, `UPDATE emails SET deleted_at = now() WHERE id = $1`, trashed); err != nil {
		t.Fatalf("soft-delete: %v", err)
	}

	got, err := q.GetInterviewInvitation(ctx, GetInterviewInvitationParams{UserID: user, JobID: jobID})
	if err != nil {
		t.Fatalf("GetInterviewInvitation: %v", err)
	}
	if got.ID != live {
		t.Errorf("id = %d, want %d — the deleted invitation came back", got.ID, live)
	}
}

// Reading the invitation must not consume it. read_at means a human opened the
// message, and the inbox's unread count is built on that.
func TestInterviewInvitationLeavesReadStateAlone(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedAssistantUser(t, pool, "unread-invite@example.test")
	jobID := insertJob(t, pool, "unread-invitation-vacancy")
	id := insertInvitation(t, pool, user, jobID, "unread", "Interview on Tuesday", "interview_invitation", time.Hour)

	if _, err := q.GetInterviewInvitation(ctx, GetInterviewInvitationParams{UserID: user, JobID: jobID}); err != nil {
		t.Fatalf("GetInterviewInvitation: %v", err)
	}

	var readAt *time.Time
	if err := pool.QueryRow(ctx, `SELECT read_at FROM emails WHERE id = $1`, id).Scan(&readAt); err != nil {
		t.Fatalf("read back read_at: %v", err)
	}
	if readAt != nil {
		t.Errorf("read_at = %v, want it untouched — the lookup marked a message read", readAt)
	}
}

// The lookup is owner-scoped like every other mail query: another candidate's
// invitation for the same vacancy is not ours to read.
func TestInterviewInvitationIsOwnerScoped(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	owner := seedAssistantUser(t, pool, "owner-invite@example.test")
	other := seedAssistantUser(t, pool, "other-invite@example.test")
	jobID := insertJob(t, pool, "shared-vacancy")
	insertInvitation(t, pool, owner, jobID, "owners", "Your interview", "interview_invitation", time.Hour)

	_, err := q.GetInterviewInvitation(ctx, GetInterviewInvitationParams{UserID: other, JobID: jobID})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("error = %v, want pgx.ErrNoRows — one candidate read another's invitation", err)
	}
}
