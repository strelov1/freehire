//go:build integration

// Integration tests for the agent inbox surface's SQL layer: mail a caller's own
// harness pushed (source 'external') is stored idempotently, is never enqueued for
// server-side classification, and carries the agent's triage verdict. These are
// SQL-level guarantees, so they can only be verified against a real Postgres.
// Run with: go test -tags=integration ./internal/db/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package db

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// insertEmailWithSource stores one unclassified message for a user under a given
// source, returning its id.
func insertEmailWithSource(t *testing.T, pool *pgxpool.Pool, userID int64, source, externalID string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO emails (user_id, source, external_id, received_at)
		 VALUES ($1, $2, $3, now()) RETURNING id`,
		userID, source, externalID).Scan(&id)
	if err != nil {
		t.Fatalf("insert %s email: %v", source, err)
	}
	return id
}

// queuedEmailIDs reads the classification outbox's current contents.
func queuedEmailIDs(t *testing.T, pool *pgxpool.Pool) map[int64]bool {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT email_id FROM email_classification_outbox`)
	if err != nil {
		t.Fatalf("read outbox: %v", err)
	}
	defer rows.Close()
	queued := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan outbox row: %v", err)
		}
		queued[id] = true
	}
	return queued
}

// TestExternalMailIsNotEnqueuedForClassification asserts the one line that makes
// the bring-your-own-harness tier free: mail the caller pushed themselves brings
// its own classifier, so the pending sweep must skip it however long it stays
// unclassified — while Gmail and hosted mail keep being enqueued as before.
func TestExternalMailIsNotEnqueuedForClassification(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := insertUser(t, pool, "byo@example.test")
	gmail := insertEmailWithSource(t, pool, user, "gmail", "g-1")
	hosted := insertEmailWithSource(t, pool, user, "hosted", "h-1")
	external := insertEmailWithSource(t, pool, user, "external", "x-1")

	if _, err := q.EnqueuePendingEmailClassification(ctx); err != nil {
		t.Fatalf("enqueue pending: %v", err)
	}

	queued := queuedEmailIDs(t, pool)
	if queued[external] {
		t.Error("external mail was enqueued for server-side classification; it must bring its own classifier")
	}
	if !queued[gmail] {
		t.Error("gmail mail was not enqueued; existing sources must keep classifying")
	}
	if !queued[hosted] {
		t.Error("hosted mail was not enqueued; existing sources must keep classifying")
	}
}

// TestExternalMailStaysUnqueuedAcrossSweeps asserts the skip is not a one-shot
// race with the outbox's ON CONFLICT: a second sweep must not pick the message up
// either, since external mail is never classified server-side.
func TestExternalMailStaysUnqueuedAcrossSweeps(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := insertUser(t, pool, "byo2@example.test")
	external := insertEmailWithSource(t, pool, user, "external", "x-2")

	for range 2 {
		if _, err := q.EnqueuePendingEmailClassification(ctx); err != nil {
			t.Fatalf("enqueue pending: %v", err)
		}
	}

	if queuedEmailIDs(t, pool)[external] {
		t.Error("external mail was enqueued on a repeat sweep")
	}
}

// pushExternal upserts one message the way the ingest endpoint will, returning the
// row id and whether this call inserted it (as opposed to updating an existing one).
func pushExternal(t *testing.T, q *Queries, userID int64, externalID, subject, body string) (int64, bool) {
	t.Helper()
	row, err := q.UpsertExternalEmail(context.Background(), UpsertExternalEmailParams{
		UserID:     userID,
		ExternalID: externalID,
		ThreadID:   "thread-" + externalID,
		FromAddr:   "ats@example.test",
		FromName:   "Acme Hiring",
		Subject:    subject,
		BodyText:   body,
		BodyHtml:   "",
		ReceivedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	if err != nil {
		t.Fatalf("upsert external email: %v", err)
	}
	return row.ID, row.Inserted
}

// TestUpsertExternalEmailIsIdempotent asserts a harness can re-sync freely: the
// dedup key is (user, source, external id), so re-pushing a message updates it in
// place and reports itself as an update, never a second row.
func TestUpsertExternalEmailIsIdempotent(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)

	user := insertUser(t, pool, "push@example.test")

	id, inserted := pushExternal(t, q, user, "msg-1", "Application received", "thanks for applying")
	if !inserted {
		t.Error("first push reported an update; it must report an insert")
	}

	sameID, inserted := pushExternal(t, q, user, "msg-1", "Application received (edited)", "revised body")
	if inserted {
		t.Error("re-push reported an insert; it must report an update")
	}
	if sameID != id {
		t.Fatalf("re-push created a new row %d, want the existing %d", sameID, id)
	}

	var count int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM emails WHERE user_id = $1`, user).Scan(&count); err != nil {
		t.Fatalf("count emails: %v", err)
	}
	if count != 1 {
		t.Fatalf("re-push left %d rows, want 1", count)
	}

	var subject, bodyText, source string
	if err := pool.QueryRow(context.Background(),
		`SELECT subject, body_text, source FROM emails WHERE id = $1`, id).
		Scan(&subject, &bodyText, &source); err != nil {
		t.Fatalf("read email: %v", err)
	}
	if source != "external" {
		t.Errorf("source = %q, want external", source)
	}
	if subject != "Application received (edited)" || bodyText != "revised body" {
		t.Errorf("re-push did not refresh content: subject=%q body=%q", subject, bodyText)
	}
}

// TestUpsertExternalEmailPreservesUserState asserts a re-sync cannot undo what the
// user or their agent already decided about a message. Content is refreshed;
// read, deletion, and the triage verdict are the reader's state and must survive —
// otherwise a nightly sync would resurrect deleted mail and wipe classifications.
func TestUpsertExternalEmailPreservesUserState(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := insertUser(t, pool, "state@example.test")
	job := insertJob(t, pool, "acme-backend")
	id, _ := pushExternal(t, q, user, "msg-2", "Interview invitation", "when are you free?")

	if _, err := pool.Exec(ctx,
		`UPDATE emails SET read_at = now(), deleted_at = now(), status_signal = 'rejection',
		     job_id = $2, link_source = 'agent', classified_at = now()
		 WHERE id = $1`, id, job); err != nil {
		t.Fatalf("stamp reader state: %v", err)
	}

	pushExternal(t, q, user, "msg-2", "Interview invitation", "when are you free?")

	var readAt, deletedAt, classifiedAt pgtype.Timestamptz
	var signal, linkSource pgtype.Text
	var jobID pgtype.Int8
	if err := pool.QueryRow(ctx,
		`SELECT read_at, deleted_at, status_signal, job_id, link_source, classified_at
		 FROM emails WHERE id = $1`, id).
		Scan(&readAt, &deletedAt, &signal, &jobID, &linkSource, &classifiedAt); err != nil {
		t.Fatalf("read email state: %v", err)
	}
	if !readAt.Valid {
		t.Error("re-push cleared read_at; a re-sync must not un-read a message")
	}
	if !deletedAt.Valid {
		t.Error("re-push cleared deleted_at; a re-sync must not resurrect a deleted message")
	}
	if signal.String != "rejection" || !classifiedAt.Valid {
		t.Errorf("re-push wiped the classification: signal=%q classified=%v", signal.String, classifiedAt.Valid)
	}
	if jobID.Int64 != job || linkSource.String != "agent" {
		t.Errorf("re-push wiped the link: job=%d source=%q", jobID.Int64, linkSource.String)
	}
}

// TestUpsertExternalEmailIsScopedToItsOwner asserts the dedup key includes the
// user, so two people whose mail servers hand out the same Message-ID keep
// separate rows.
func TestUpsertExternalEmailIsScopedToItsOwner(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)

	userA := insertUser(t, pool, "owner-a@example.test")
	userB := insertUser(t, pool, "owner-b@example.test")

	idA, _ := pushExternal(t, q, userA, "shared-id", "A's mail", "body a")
	idB, insertedB := pushExternal(t, q, userB, "shared-id", "B's mail", "body b")

	if !insertedB {
		t.Error("the second user's push reported an update; the dedup key must include the user")
	}
	if idA == idB {
		t.Fatal("two users' messages collapsed into one row")
	}
}

// triageState is the verdict columns a triage write owns, read back together.
type triageState struct {
	signal       pgtype.Text
	jobID        pgtype.Int8
	suggestedJob pgtype.Int8
	linkSource   pgtype.Text
	confidence   pgtype.Float4
	model        pgtype.Text
	classifiedAt pgtype.Timestamptz
}

// triageSignal records a classify-only verdict — the common case, where the agent
// judges what a message is without deciding its link.
func triageSignal(t *testing.T, q *Queries, id, userID int64, signal string) int64 {
	t.Helper()
	rows, err := q.AgentTriageEmail(context.Background(), AgentTriageEmailParams{
		ID: id, UserID: userID,
		StatusSignal: pgtype.Text{String: signal, Valid: true},
	})
	if err != nil {
		t.Fatalf("triage %s: %v", signal, err)
	}
	return rows
}

func readTriageState(t *testing.T, pool *pgxpool.Pool, id int64) triageState {
	t.Helper()
	var s triageState
	err := pool.QueryRow(context.Background(),
		`SELECT status_signal, job_id, suggested_job_id, link_source, match_confidence,
		        classification_model, classified_at
		 FROM emails WHERE id = $1`, id).
		Scan(&s.signal, &s.jobID, &s.suggestedJob, &s.linkSource, &s.confidence, &s.model, &s.classifiedAt)
	if err != nil {
		t.Fatalf("read triage state: %v", err)
	}
	return s
}

// TestAgentTriageWritesOneVerdict asserts triage is a single atomic verdict, the
// same shape SetEmailClassification writes: status, link, provenance and the
// classified stamp land together, so a message is never classified-but-unstamped
// or linked-but-unclassified.
func TestAgentTriageWritesOneVerdict(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := insertUser(t, pool, "triage@example.test")
	job := insertJob(t, pool, "acme-backend")
	suggested := insertJob(t, pool, "other-role")
	id, _ := pushExternal(t, q, user, "msg-3", "Invitation", "let us talk")
	if _, err := pool.Exec(ctx, `UPDATE emails SET suggested_job_id = $2 WHERE id = $1`, id, suggested); err != nil {
		t.Fatalf("seed suggestion: %v", err)
	}

	rows, err := q.AgentTriageEmail(ctx, AgentTriageEmailParams{
		ID:           id,
		UserID:       user,
		StatusSignal: pgtype.Text{String: "interview_invitation", Valid: true},
		JobID:        pgtype.Int8{Int64: job, Valid: true},
		Confidence:   pgtype.Float4{Float32: 0.9, Valid: true},
	})
	if err != nil {
		t.Fatalf("triage: %v", err)
	}
	if rows != 1 {
		t.Fatalf("triage matched %d rows, want 1", rows)
	}

	s := readTriageState(t, pool, id)
	if s.signal.String != "interview_invitation" {
		t.Errorf("signal = %q, want interview_invitation", s.signal.String)
	}
	if s.jobID.Int64 != job {
		t.Errorf("job_id = %d, want %d", s.jobID.Int64, job)
	}
	if s.linkSource.String != "agent" {
		t.Errorf("link_source = %q, want agent", s.linkSource.String)
	}
	if s.suggestedJob.Valid {
		t.Error("triage left a pending suggestion; the agent's verdict supersedes it")
	}
	if !s.classifiedAt.Valid || s.model.String != "agent" {
		t.Errorf("triage did not stamp provenance: classified=%v model=%q", s.classifiedAt.Valid, s.model.String)
	}
	if s.confidence.Float32 != 0.9 {
		t.Errorf("confidence = %v, want 0.9", s.confidence.Float32)
	}
}

// TestAgentTriageWithoutJobKeepsTheLink asserts a classify-only triage does not
// silently unlink: an omitted job means "I am not deciding the link", not "clear
// it". Unlinking stays the explicit /unlink action.
func TestAgentTriageWithoutJobKeepsTheLink(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := insertUser(t, pool, "keeplink@example.test")
	job := insertJob(t, pool, "kept-role")
	id, _ := pushExternal(t, q, user, "msg-4", "Update", "some news")
	if _, err := pool.Exec(ctx,
		`UPDATE emails SET job_id = $2, link_source = 'manual',
		    application_id = (SELECT a.id FROM applications a
		                       WHERE a.user_id = emails.user_id AND a.job_id = $2)
		 WHERE emails.id = $1`, id, job); err != nil {
		t.Fatalf("seed link: %v", err)
	}

	triageSignal(t, q, id, user, "rejection")

	s := readTriageState(t, pool, id)
	if s.jobID.Int64 != job {
		t.Errorf("classify-only triage cleared the link: job_id = %v", s.jobID)
	}
	if s.linkSource.String != "manual" {
		t.Errorf("classify-only triage rewrote link provenance to %q, want manual", s.linkSource.String)
	}
	if s.signal.String != "rejection" {
		t.Errorf("signal = %q, want rejection", s.signal.String)
	}
}

// TestAgentTriageIsScopedToTheCaller asserts one user's key can never triage
// another user's mail: the write matches no row, which the handler renders as 404.
func TestAgentTriageIsScopedToTheCaller(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)

	owner := insertUser(t, pool, "owner@example.test")
	stranger := insertUser(t, pool, "stranger@example.test")
	id, _ := pushExternal(t, q, owner, "msg-5", "Private", "not yours")

	rows := triageSignal(t, q, id, stranger, "offer")
	if rows != 0 {
		t.Fatalf("a stranger's triage matched %d rows, want 0", rows)
	}
	if s := readTriageState(t, pool, id); s.signal.Valid {
		t.Errorf("a stranger's triage wrote signal %q onto another user's mail", s.signal.String)
	}
}

// TestAgentTriageIsRepeatable asserts re-triaging overwrites rather than failing,
// so an agent that changes its mind — or re-runs after a crash — is not blocked.
func TestAgentTriageIsRepeatable(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)

	user := insertUser(t, pool, "repeat@example.test")
	id, _ := pushExternal(t, q, user, "msg-6", "Update", "news")

	for _, sig := range []string{"acknowledgement", "rejection"} {
		triageSignal(t, q, id, user, sig)
	}

	if s := readTriageState(t, pool, id); s.signal.String != "rejection" {
		t.Errorf("signal = %q, want the latest verdict rejection", s.signal.String)
	}
}

// listIDs runs the inbox listing with the given filters and returns the ids it
// yielded, alongside the paging total.
func listIDs(t *testing.T, q *Queries, p ListEmailsParams) ([]int64, int64) {
	t.Helper()
	ctx := context.Background()
	p.Lim, p.Off = 50, 0
	rows, err := q.ListEmails(ctx, p)
	if err != nil {
		t.Fatalf("list emails: %v", err)
	}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	total, err := q.CountEmails(ctx, CountEmailsParams{
		UserID: p.UserID, Src: p.Src, Unread: p.Unread,
		Status: p.Status, Q: p.Q, Unclassified: p.Unclassified,
	})
	if err != nil {
		t.Fatalf("count emails: %v", err)
	}
	return ids, total.Total
}

// TestUnclassifiedFilterIsTheAgentsWorkQueue asserts the listing can answer "what
// still needs triage" — the agent's only way to find its backlog, since external
// mail is never enqueued for the worker.
func TestUnclassifiedFilterIsTheAgentsWorkQueue(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)

	user := insertUser(t, pool, "queue@example.test")
	pending, _ := pushExternal(t, q, user, "q-1", "Needs triage", "body")
	done, _ := pushExternal(t, q, user, "q-2", "Already triaged", "body")

	triageSignal(t, q, done, user, "rejection")

	ids, total := listIDs(t, q, ListEmailsParams{UserID: user, Unclassified: true})
	if len(ids) != 1 || ids[0] != pending {
		t.Fatalf("unclassified listing = %v, want [%d]", ids, pending)
	}
	if total != 1 {
		t.Errorf("unclassified total = %d, want 1 — the count must honour the filter too", total)
	}

	all, allTotal := listIDs(t, q, ListEmailsParams{UserID: user})
	if len(all) != 2 || allTotal != 2 {
		t.Errorf("unfiltered listing = %v (total %d), want both messages", all, allTotal)
	}
}

// TestUnclassifiedFilterComposesWithSource asserts the new filter narrows within
// the existing ones rather than replacing them.
func TestUnclassifiedFilterComposesWithSource(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)

	user := insertUser(t, pool, "compose@example.test")
	external, _ := pushExternal(t, q, user, "c-1", "Pushed", "body")
	insertEmailWithSource(t, pool, user, "gmail", "c-2")

	ids, total := listIDs(t, q, ListEmailsParams{UserID: user, Src: "external", Unclassified: true})
	if len(ids) != 1 || ids[0] != external {
		t.Fatalf("listing = %v, want only the external message %d", ids, external)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
}

// TestListingBodiesAreOptIn asserts bodies ride along only when asked for, so the
// web inbox keeps transferring snippets while an agent gets what it needs to
// classify in one request.
func TestListingBodiesAreOptIn(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := insertUser(t, pool, "bodies@example.test")
	pushExternal(t, q, user, "b-1", "Invitation", "we would like to meet you")

	without, err := q.ListEmails(ctx, ListEmailsParams{UserID: user, Lim: 10})
	if err != nil {
		t.Fatalf("list without bodies: %v", err)
	}
	if len(without) != 1 {
		t.Fatalf("listing returned %d rows, want 1", len(without))
	}
	if without[0].BodyText != "" {
		t.Errorf("body rode along uninvited: %q", without[0].BodyText)
	}
	if without[0].Snippet == "" {
		t.Error("snippet went missing; the web list row depends on it")
	}

	with, err := q.ListEmails(ctx, ListEmailsParams{UserID: user, WithBody: true, Lim: 10})
	if err != nil {
		t.Fatalf("list with bodies: %v", err)
	}
	if with[0].BodyText != "we would like to meet you" {
		t.Errorf("body_text = %q, want the stored body", with[0].BodyText)
	}
}
