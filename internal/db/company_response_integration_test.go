//go:build integration

// Integration tests for the per-company response-rate rollup: which applications are
// observable, what counts as answered, and that unobservable applications are absent
// from BOTH sides of the ratio. The sample-size gate is applied by the serving layer
// and tested there. Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"
)

func seedResponseUser(t *testing.T, q *Queries, email string, withMailbox bool) int64 {
	t.Helper()
	ctx := context.Background()
	var id int64
	if err := q.db.QueryRow(ctx, `INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("seed user %s: %v", email, err)
	}
	if withMailbox {
		if _, err := q.db.Exec(ctx,
			`INSERT INTO gmail_connections (user_id, email, refresh_token_enc, status)
			 VALUES ($1, $2, 'enc', 'connected')`, id, email); err != nil {
			t.Fatalf("connect mailbox for %s: %v", email, err)
		}
	}
	return id
}

func seedResponseJob(t *testing.T, q *Queries, extID, company string) int64 {
	t.Helper()
	var id int64
	if err := q.db.QueryRow(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, company_slug, public_slug)
		 VALUES ('test', $1, 'http://example.test/r', 'Go Dev', $2, $3) RETURNING id`,
		extID, company, extID+"-slug").Scan(&id); err != nil {
		t.Fatalf("seed job %s: %v", extID, err)
	}
	return id
}

func TestRebuildInsightsCompanyResponse_CountsObservableApplications(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	answered := seedResponseUser(t, q, "resp-answered@example.test", true)
	ignored := seedResponseUser(t, q, "resp-ignored@example.test", true)
	job1 := seedResponseJob(t, q, "resp-1", "respco")
	job2 := seedResponseJob(t, q, "resp-2", "respco")

	for _, a := range []struct {
		uid, jid int64
	}{{answered, job1}, {ignored, job2}} {
		if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: a.uid, JobID: a.jid, EventSource: "user"}); err != nil {
			t.Fatalf("MarkJobApplied: %v", err)
		}
	}
	// One reply arrives, linked to the first application. It is deliberately left
	// unclassified: a linked message is evidence the employer wrote, and requiring a
	// classification would silently exclude the `external` tier, which is never
	// classified server-side.
	var replyID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO emails (user_id, external_id, source, subject, job_id, received_at)
		 VALUES ($1, 'reply-1', 'gmail', 'Thanks for applying', $2, now()) RETURNING id`,
		answered, job1).Scan(&replyID); err != nil {
		t.Fatalf("seed reply: %v", err)
	}
	if err := q.RecordEmailApplicationEvent(ctx, RecordEmailApplicationEventParams{
		ID: replyID, UserID: answered, EventSource: "mail_gmail",
	}); err != nil {
		t.Fatalf("record reply event: %v", err)
	}

	if _, err := q.RebuildInsightsCompanyResponse(ctx); err != nil {
		t.Fatalf("RebuildInsightsCompanyResponse: %v", err)
	}
	got, err := q.GetCompanyResponse(ctx, "respco")
	if err != nil {
		t.Fatalf("GetCompanyResponse: %v", err)
	}
	if got.Applications != 2 || got.Answered != 1 {
		t.Errorf("got %+v, want 2 applications and 1 answered", got)
	}
}

// The same gate as the job-level signal: where no reply could have been observed,
// an unanswered application is a gap in our data rather than an employer's silence.
// Counting it in the denominator would report our blind spot as their fault.
func TestRebuildInsightsCompanyResponse_ExcludesUnobservableApplicationsFromBothSides(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	observable := seedResponseUser(t, q, "obs@example.test", true)
	invisible := seedResponseUser(t, q, "invis@example.test", false)
	job1 := seedResponseJob(t, q, "gate-r1", "gateco")
	job2 := seedResponseJob(t, q, "gate-r2", "gateco")

	for _, a := range []struct {
		uid, jid int64
	}{{observable, job1}, {invisible, job2}} {
		if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: a.uid, JobID: a.jid, EventSource: "user"}); err != nil {
			t.Fatalf("MarkJobApplied: %v", err)
		}
	}

	if _, err := q.RebuildInsightsCompanyResponse(ctx); err != nil {
		t.Fatalf("RebuildInsightsCompanyResponse: %v", err)
	}
	got, err := q.GetCompanyResponse(ctx, "gateco")
	if err != nil {
		t.Fatalf("GetCompanyResponse: %v", err)
	}
	if got.Applications != 1 {
		t.Errorf("applications = %d, want 1 — the unobservable application must not swell the denominator", got.Applications)
	}
}

// A company nobody observably applied to has no row at all, which the serving layer
// must read as "not enough data" rather than as a zero response rate.
func TestRebuildInsightsCompanyResponse_CompanyWithNoObservableApplicationsHasNoRow(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	seedResponseJob(t, q, "quiet-1", "quietco")
	if _, err := q.RebuildInsightsCompanyResponse(ctx); err != nil {
		t.Fatalf("RebuildInsightsCompanyResponse: %v", err)
	}
	if _, err := q.GetCompanyResponse(ctx, "quietco"); err == nil {
		t.Error("a company with no observable application returned a row; absence must stay distinguishable from zero")
	}
}

// The two facts the ledger was introduced to protect, asserted at the level a visitor
// actually sees: the served company rate.
func TestRebuildInsightsCompanyResponse_DeletionAndRelink(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "rate-move@example.test", true)
	jobA := seedResponseJob(t, q, "rate-a", "alpha")
	jobB := seedResponseJob(t, q, "rate-b", "beta")
	for _, j := range []int64{jobA, jobB} {
		if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: user, JobID: j, EventSource: "user"}); err != nil {
			t.Fatalf("apply: %v", err)
		}
	}
	var reply int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO emails (user_id, external_id, source, job_id, received_at)
		 VALUES ($1, 'move-1', 'gmail', $2, now()) RETURNING id`, user, jobA).Scan(&reply); err != nil {
		t.Fatalf("seed reply: %v", err)
	}
	reconcile := func() {
		t.Helper()
		if _, err := q.RetractSupersededEmailEvent(ctx, RetractSupersededEmailEventParams{ID: reply, UserID: user}); err != nil {
			t.Fatalf("retract: %v", err)
		}
		if err := q.RecordEmailApplicationEvent(ctx, RecordEmailApplicationEventParams{
			ID: reply, UserID: user, EventSource: "mail_gmail",
		}); err != nil {
			t.Fatalf("record: %v", err)
		}
	}
	answered := func(slug string) int32 {
		t.Helper()
		// The worker clears and rebuilds in one transaction; the rebuild statement is
		// insert-only, so a test that reruns it must clear too.
		if err := q.DeleteAllInsightsCompanyResponse(ctx); err != nil {
			t.Fatalf("clear: %v", err)
		}
		if _, err := q.RebuildInsightsCompanyResponse(ctx); err != nil {
			t.Fatalf("rebuild: %v", err)
		}
		row, err := q.GetCompanyResponse(ctx, slug)
		if err != nil {
			return 0
		}
		return row.Answered
	}

	reconcile()
	if got := answered("alpha"); got != 1 {
		t.Fatalf("alpha answered = %d, want 1", got)
	}

	// Tidying the inbox must not make an employer that answered look silent.
	if _, err := pool.Exec(ctx, `UPDATE emails SET deleted_at = now() WHERE id = $1`, reply); err != nil {
		t.Fatalf("delete mail: %v", err)
	}
	reconcile()
	if got := answered("alpha"); got != 1 {
		t.Errorf("alpha answered = %d after the candidate deleted the mail, want 1 — inbox hygiene must not move a public number", got)
	}

	// Correcting a mislink must move the credit, or the wrong company stays poisoned.
	if _, err := pool.Exec(ctx, `UPDATE emails SET job_id = $2 WHERE id = $1`, reply, jobB); err != nil {
		t.Fatalf("relink: %v", err)
	}
	reconcile()
	if got := answered("alpha"); got != 0 {
		t.Errorf("alpha answered = %d after the link was corrected away, want 0", got)
	}
	if got := answered("beta"); got != 1 {
		t.Errorf("beta answered = %d after the correction, want 1", got)
	}
}
