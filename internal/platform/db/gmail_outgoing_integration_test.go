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

func TestOutgoingThreadMailAppearsAfterIncomingLink(t *testing.T) {
	pool := startPostgres(t)
	q, ctx := New(pool), context.Background()
	user := insertUser(t, pool, "thread-view@example.test")
	job := insertJob(t, pool, "thread-view-job")
	otherJob := insertJob(t, pool, "thread-other-job")
	applyToJob(t, pool, user, job, "applied")
	for _, direction := range []string{"incoming", "outgoing"} {
		if err := q.UpsertEmail(ctx, UpsertEmailParams{UserID: user, ExternalID: direction, ThreadID: "thread", Direction: direction, ReceivedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}}); err != nil {
			t.Fatal(err)
		}
	}
	incoming, err := q.GetEmailIDByExternalID(ctx, GetEmailIDByExternalIDParams{UserID: user, ExternalID: "incoming"})
	if err != nil {
		t.Fatal(err)
	}
	outgoing, err := q.GetEmailIDByExternalID(ctx, GetEmailIDByExternalIDParams{UserID: user, ExternalID: "outgoing"})
	if err != nil {
		t.Fatal(err)
	}
	assertSent := func(jobID int64, want bool) {
		t.Helper()
		rows, err := q.ListJobEmails(ctx, ListJobEmailsParams{UserID: user, JobID: pgtype.Int8{Int64: jobID, Valid: true}})
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, row := range rows {
			found = found || row.ID == outgoing
		}
		if found != want {
			t.Fatalf("job %d sent message visible=%v, want %v", jobID, found, want)
		}
	}
	assertSent(job, false)
	// Classification may link the incoming message after both messages were synced.
	linkEmail(t, pool, incoming, job, "manual")
	assertSent(job, true)
	state := readLinkState(t, pool, outgoing)
	if state.jobID.Valid || state.applicationID.Valid {
		t.Fatal("display inference persisted an outgoing attachment")
	}
	other := insertLinkedEmail(t, pool, user, "other-anchor", otherJob)
	if _, err := pool.Exec(ctx, "UPDATE emails SET thread_id='thread' WHERE id=$1", other); err != nil {
		t.Fatal(err)
	}
	assertSent(job, false)
	assertSent(otherJob, false)
	// A linked application whose posting is gone is still a conflicting context.
	var orphan int64
	if err := pool.QueryRow(ctx, "INSERT INTO applications(user_id,company_slug,role_title) VALUES($1,'orphan','Role') RETURNING id", user).Scan(&orphan); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE emails SET job_id=NULL,application_id=$2 WHERE id=$1", other, orphan); err != nil {
		t.Fatal(err)
	}
	assertSent(job, false)
	// Explicit manual and agent attachments win over ambiguous conversation context.
	for _, source := range []string{"manual", "agent"} {
		linkEmail(t, pool, outgoing, otherJob, source)
		assertSent(otherJob, true)
		assertSent(job, false)
	}
	if _, err := pool.Exec(ctx, "UPDATE emails SET job_id=NULL,application_id=NULL,link_source=NULL WHERE id=$1", outgoing); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE emails SET deleted_at=now() WHERE id=$1", other); err != nil {
		t.Fatal(err)
	}
	assertSent(job, true)
	for _, mutation := range []string{"source='hosted'", "thread_id=''", "deleted_at=now()"} {
		if _, err := pool.Exec(ctx, "UPDATE emails SET "+mutation+" WHERE id=$1", incoming); err != nil {
			t.Fatal(err)
		}
		assertSent(job, false)
		if _, err := pool.Exec(ctx, "UPDATE emails SET source='gmail',thread_id='thread',deleted_at=NULL WHERE id=$1", incoming); err != nil {
			t.Fatal(err)
		}
	}
	otherUser := insertUser(t, pool, "thread-other-user@example.test")
	if _, err := pool.Exec(ctx, "UPDATE emails SET user_id=$2 WHERE id=$1", incoming, otherUser); err != nil {
		t.Fatal(err)
	}
	assertSent(job, false)
	if _, err := pool.Exec(ctx, "UPDATE emails SET user_id=$2 WHERE id=$1", incoming, user); err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []string{"source='hosted'", "thread_id=''"} {
		if _, err := pool.Exec(ctx, "UPDATE emails SET "+mutation+" WHERE id IN ($1,$2)", incoming, outgoing); err != nil {
			t.Fatal(err)
		}
		assertSent(job, false)
		if _, err := pool.Exec(ctx, "UPDATE emails SET source='gmail',thread_id='thread' WHERE id IN ($1,$2)", incoming, outgoing); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRecallExcludesSentAndRepairedAliases(t *testing.T) {
	pool := startPostgres(t)
	q, ctx := New(pool), context.Background()
	user := insertUser(t, pool, "recall-sent@example.test")
	job := insertJob(t, pool, "recall-sent-job")
	now := time.Now()
	if err := q.UpsertEmail(ctx, UpsertEmailParams{UserID: user, ExternalID: "alias", ReceivedAt: pgtype.Timestamptz{Time: now, Valid: true}}); err != nil {
		t.Fatal(err)
	}
	args := ListEmailsForRecallParams{UserID: user, Since: pgtype.Timestamptz{Time: now.Add(-time.Hour), Valid: true}, Until: pgtype.Timestamptz{Time: now.Add(time.Hour), Valid: true}, Lim: 10}
	rows, err := q.ListEmailsForRecall(ctx, args)
	if err != nil || len(rows) != 1 {
		t.Fatalf("incoming recall rows=%d err=%v", len(rows), err)
	}
	id := rows[0].ID
	if err := q.UpsertEmail(ctx, UpsertEmailParams{UserID: user, ExternalID: "alias", Direction: "outgoing", ReceivedAt: pgtype.Timestamptz{Time: now, Valid: true}}); err != nil {
		t.Fatal(err)
	}
	changed, err := q.SuggestJobForEmail(ctx, SuggestJobForEmailParams{ID: id, UserID: user, SuggestedJobID: job, Confidence: 1})
	if err != nil || changed != 0 {
		t.Errorf("in-flight recall suggested repaired sent alias: changed=%d err=%v", changed, err)
	}
	rows, err = q.ListEmailsForRecall(ctx, args)
	if err != nil || len(rows) != 0 {
		t.Fatalf("sent alias entered recall: rows=%d err=%v", len(rows), err)
	}
}
