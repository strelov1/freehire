//go:build integration

package db

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestOutgoingMailCannotDriveEmployerSignals(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	user := insertUser(t, pool, "direction@example.test")
	job := insertJob(t, pool, "direction-job")
	applyToJob(t, pool, user, job, "applied")
	incoming := insertLinkedEmail(t, pool, user, "incoming", job)
	var outgoing int64
	err := pool.QueryRow(ctx, `INSERT INTO emails
  (user_id, source, external_id, thread_id, from_addr, subject, body_text, direction, to_addrs, read_at, received_at)
  VALUES ($1, 'gmail', 'outgoing', 'hiring', 'alias@example.test', 'Re: Interview', 'Confirmed', 'outgoing', 'hr@example.test', now(), now() + interval '1 hour')
  RETURNING id`, user).Scan(&outgoing)
	if err != nil {
		t.Fatal(err)
	}
	linkEmail(t, pool, outgoing, job, "manual")
	t.Run("enqueue", func(t *testing.T) {
		if _, err := q.EnqueuePendingEmailClassification(ctx); err != nil {
			t.Fatal(err)
		}
		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM email_classification_outbox WHERE email_id=$1", outgoing).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatal("outgoing mail entered employer classification queue")
		}
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM email_classification_outbox WHERE email_id=$1", incoming).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Fatal("incoming mail lost employer classification")
		}
	})
	t.Run("stale lease", func(t *testing.T) {
		if _, err := pool.Exec(ctx, "INSERT INTO email_classification_outbox(email_id) VALUES ($1) ON CONFLICT DO NOTHING", outgoing); err != nil {
			t.Fatal(err)
		}
		claimed, err := q.ClaimEmailClassificationBatch(ctx, ClaimEmailClassificationBatchParams{LeaseSeconds: 60, BatchSize: 100})
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range claimed {
			if row.EmailID == outgoing {
				t.Fatal("outgoing mail leased for classification")
			}
		}
	})
	t.Run("classification writes", func(t *testing.T) {
		signal := pgtype.Text{String: "interview_invitation", Valid: true}
		if err := q.SetEmailClassification(ctx, SetEmailClassificationParams{ID: outgoing, UserID: user, StatusSignal: signal, Model: pgtype.Text{String: "test", Valid: true}}); err != nil {
			t.Fatal(err)
		}
		var classified bool
		if err := pool.QueryRow(ctx, "SELECT classified_at IS NOT NULL FROM emails WHERE id=$1", outgoing).Scan(&classified); err != nil {
			t.Fatal(err)
		}
		if classified {
			t.Fatal("outgoing mail accepted worker classification")
		}
		changed, err := q.AgentTriageEmail(ctx, AgentTriageEmailParams{ID: outgoing, UserID: user, StatusSignal: signal})
		if err != nil {
			t.Fatal(err)
		}
		if changed != 0 {
			t.Fatal("outgoing mail accepted agent classification")
		}
	})
	t.Run("employer silence", func(t *testing.T) {
		app, err := q.GetUserApplication(ctx, GetUserApplicationParams{UserID: user, JobID: job})
		if err != nil {
			t.Fatal(err)
		}
		if app.LastActivityAt.Time.After(time.Now().Add(time.Minute)) {
			t.Fatal("candidate reply reset employer silence")
		}
	})
	t.Run("events and stage", func(t *testing.T) {
		if err := q.RecordEmailApplicationEvent(ctx, RecordEmailApplicationEventParams{ID: outgoing, UserID: user, EventSource: "gmail"}); err != nil {
			t.Fatal(err)
		}
		if err := q.AdvanceUserJobStage(ctx, AdvanceUserJobStageParams{UserID: user, JobID: job, Stage: "interview", EmailID: outgoing, EventSource: "gmail"}); err != nil {
			t.Fatal(err)
		}
		stage, err := q.GetUserJobStage(ctx, GetUserJobStageParams{UserID: user, JobID: job})
		if err != nil {
			t.Fatal(err)
		}
		if stage != "applied" {
			t.Fatalf("candidate reply changed stage to %s", stage)
		}
		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM application_events WHERE user_id=$1 AND kind IN ('employer_reply','stage_set')", user).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatal("candidate reply created employer/stage events")
		}
	})
}

func TestDirectionSchemaPreservesExistingGmailWriter(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	user := insertUser(t, pool, "direction-default@example.test")
	// Match the already-deployed writer: its INSERT does not name the new columns.
	if _, err := pool.Exec(ctx, `INSERT INTO emails
 (user_id, source, external_id, thread_id, from_addr, from_name, subject, body_text, body_html, received_at, ical_uid)
 VALUES ($1, 'gmail', 'legacy-writer', '', '', '', '', '', '', now(), '')
 ON CONFLICT (user_id, source, external_id) DO NOTHING`, user); err != nil {
		t.Fatal(err)
	}
	var direction, to, cc string
	var sentCursor int64
	if err := pool.QueryRow(ctx, "SELECT direction,to_addrs,cc_addrs FROM emails WHERE user_id=$1", user).Scan(&direction, &to, &cc); err != nil {
		t.Fatal(err)
	}
	if direction != "incoming" || to != "" || cc != "" {
		t.Fatalf("legacy writer defaults: %q/%q/%q", direction, to, cc)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO gmail_connections(user_id,email,refresh_token_enc) VALUES ($1,'direction-default@example.test','encrypted')", user); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT sent_sync_cursor FROM gmail_connections WHERE user_id=$1", user).Scan(&sentCursor); err != nil {
		t.Fatal(err)
	}
	if sentCursor != 0 {
		t.Fatalf("sent cursor = %d, want first-run backfill", sentCursor)
	}
}
