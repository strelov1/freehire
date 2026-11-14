//go:build integration

package db

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestOutgoingMailNeverBecomesEmployerSignal(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	user := insertUser(t, pool, "sent@example.test")
	job := insertJob(t, pool, "sent-job")
	applyToJob(t, pool, user, job, "applied")
	p := UpsertEmailParams{UserID: user, ExternalID: "sent", ThreadID: "hiring", FromAddr: "alias@example.test", ToAddrs: "hr@example.test", Direction: "outgoing", Subject: "Re: Interview", ReceivedAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true}}
	for range 2 {
		if err := q.UpsertEmail(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := q.ListEmails(ctx, ListEmailsParams{UserID: user, Lim: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Direction != "outgoing" || rows[0].ToAddrs != "hr@example.test" || !rows[0].Read {
		t.Fatalf("sent inbox row: %+v", rows)
	}
	id := rows[0].ID
	pending, err := q.ListEmails(ctx, ListEmailsParams{UserID: user, Lim: 10, Unclassified: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("sent mail entered triage listing: %+v", pending)
	}
	if _, err = q.EnqueuePendingEmailClassification(ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM email_classification_outbox WHERE email_id=$1", id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("sent mail enqueued")
	}
	// Even a stale queue row must never reach the classifier.
	if _, err = pool.Exec(ctx, "INSERT INTO email_classification_outbox(email_id) VALUES ($1)", id); err != nil {
		t.Fatal(err)
	}
	claimed, err := q.ClaimEmailClassificationBatch(ctx, ClaimEmailClassificationBatchParams{LeaseSeconds: 60, BatchSize: 100})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range claimed {
		if row.EmailID == id {
			t.Fatal("sent mail leased for classification")
		}
	}
	linkEmail(t, pool, id, job, "manual")
	app, err := q.GetUserApplication(ctx, GetUserApplicationParams{UserID: user, JobID: job})
	if err != nil {
		t.Fatal(err)
	}
	if app.LastActivityAt.Time.After(time.Now().Add(time.Minute)) {
		t.Fatal("candidate reply reset employer silence")
	}

	if err = q.RecordEmailApplicationEvent(ctx, RecordEmailApplicationEventParams{ID: id, UserID: user, EventSource: "gmail"}); err != nil {
		t.Fatal(err)
	}
	if err = q.AdvanceUserJobStage(ctx, AdvanceUserJobStageParams{UserID: user, JobID: job, Stage: "interview", EmailID: id, EventSource: "gmail"}); err != nil {
		t.Fatal(err)
	}
	stage, err := q.GetUserJobStage(ctx, GetUserJobStageParams{UserID: user, JobID: job})
	if err != nil {
		t.Fatal(err)
	}
	if stage != "applied" {
		t.Fatalf("sent mail changed stage to %s", stage)
	}
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM application_events WHERE user_id=$1 AND kind IN ('employer_reply','stage_set')", user).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("sent mail created employer/stage events")
	}
	changed, err := q.AgentTriageEmail(ctx, AgentTriageEmailParams{ID: id, UserID: user, StatusSignal: pgtype.Text{String: "interview_invitation", Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	if changed != 0 {
		t.Fatal("candidate mail accepted employer classification")
	}
}

func TestReimportRepairsOutgoingAlias(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	user := insertUser(t, pool, "old-alias@example.test")
	job := insertJob(t, pool, "old-alias-job")
	applyToJob(t, pool, user, job, "applied")
	id := insertLinkedEmail(t, pool, user, "old-alias", job)
	if _, err := pool.Exec(ctx, "UPDATE emails SET status_signal='interview_invitation', classified_at=now() WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	if err := q.RecordEmailApplicationEvent(ctx, RecordEmailApplicationEventParams{ID: id, UserID: user, EventSource: "gmail"}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO email_classification_outbox(email_id) VALUES ($1)", id); err != nil {
		t.Fatal(err)
	}
	if err := q.UpsertEmail(ctx, UpsertEmailParams{UserID: user, ExternalID: "old-alias", Direction: "outgoing", ToAddrs: "hr@example.test", ReceivedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}}); err != nil {
		t.Fatal(err)
	}
	email, err := q.GetEmail(ctx, GetEmailParams{ID: id, UserID: user})
	if err != nil {
		t.Fatal(err)
	}
	if email.Direction != "outgoing" || email.ToAddrs != "hr@example.test" || email.StatusSignal.Valid || !email.Read {
		t.Fatalf("alias metadata not repaired: %+v", email)
	}
	var n int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM application_events WHERE source_ref=$1 AND kind='employer_reply' AND retracted_at IS NULL", id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("candidate reply retained employer event")
	}
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM email_classification_outbox WHERE email_id=$1", id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("candidate reply retained classification work")
	}
}
