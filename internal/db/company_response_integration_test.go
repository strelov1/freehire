//go:build integration

// Integration tests for the per-company response-rate rollup: which applications are
// observable, what counts as answered, and that unobservable applications are absent
// from BOTH sides of the ratio. The sample-size gate is applied by the serving layer
// and tested there. Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
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
		`INSERT INTO emails (user_id, external_id, source, subject, job_id, application_id, received_at)
		 VALUES ($1, 'reply-1', 'gmail', 'Thanks for applying', $2,
		         (SELECT a.id FROM applications a WHERE a.user_id = $1 AND a.job_id = $2), now()) RETURNING id`,
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

// rebuildAnswered clears and rebuilds the rollup, then reads one company's counts. The
// worker does both in one transaction and the rebuild statement is insert-only, so a
// test that reruns it must clear first.
func rebuildAnswered(t *testing.T, q *Queries, slug string) (applications, answered int32) {
	t.Helper()
	ctx := context.Background()
	if err := q.DeleteAllInsightsCompanyResponse(ctx); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if _, err := q.RebuildInsightsCompanyResponse(ctx); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	row, err := q.GetCompanyResponse(ctx, slug)
	if err != nil {
		return 0, 0
	}
	return row.Applications, row.Answered
}

// seedReply links a message to an application and records the employer_reply event.
// Deliberately unclassified: a linked message is evidence the employer wrote, and
// requiring a classification would exclude the never-classified `external` tier.
func seedReply(t *testing.T, q *Queries, userID, jobID int64, extID string) int64 {
	t.Helper()
	ctx := context.Background()
	var id int64
	if err := q.db.QueryRow(ctx,
		`INSERT INTO emails (user_id, external_id, source, job_id, application_id, received_at)
		 VALUES ($1, $2, 'gmail', $3,
		         (SELECT a.id FROM applications a WHERE a.user_id = $1 AND a.job_id = $3),
		         now())
		 RETURNING id`, userID, extID, jobID).Scan(&id); err != nil {
		t.Fatalf("seed reply %s: %v", extID, err)
	}
	if err := q.RecordEmailApplicationEvent(ctx, RecordEmailApplicationEventParams{
		ID: id, UserID: userID, EventSource: "mail_gmail",
	}); err != nil {
		t.Fatalf("record reply event %s: %v", extID, err)
	}
	return id
}

// cmd/prune is the only hard-delete path for jobs, and application_events.job_id is
// ON DELETE SET NULL — so a removal must not change what the rollup says about the
// employer. The denominator already survives: company_slug is denormalized onto every
// event. The numerator is the half at risk, because the reply is matched back to its
// application through job_id, and two cleared references never match.
func TestRebuildInsightsCompanyResponse_SurvivesPrunedPosting(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "prune-answered@example.test", true)
	job := seedResponseJob(t, q, "prune-1", "prunedco")
	if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: user, JobID: job, EventSource: "user"}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	seedReply(t, q, user, job, "prune-reply-1")

	if apps, ans := rebuildAnswered(t, q, "prunedco"); apps != 1 || ans != 1 {
		t.Fatalf("before the prune: %d applications, %d answered; want 1 and 1", apps, ans)
	}

	if _, err := pool.Exec(ctx, `DELETE FROM jobs WHERE id = $1`, job); err != nil {
		t.Fatalf("prune the posting: %v", err)
	}

	apps, ans := rebuildAnswered(t, q, "prunedco")
	if apps != 1 {
		t.Errorf("applications = %d after the posting was pruned, want 1 — the denormalized slug keeps the application countable", apps)
	}
	if ans != 1 {
		t.Errorf("answered = %d after the posting was pruned, want 1 — an employer that replied must not be served as silent", ans)
	}
}

// The guard against the tempting wrong fix. Correlating a reply to its application by
// (user, company) instead of by the application itself looks like it survives a prune,
// and it does — by crediting one employer with a reply to a different application.
func TestRebuildInsightsCompanyResponse_PrunedApplicationsToOneEmployerStayDistinct(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "prune-twice@example.test", true)
	answeredJob := seedResponseJob(t, q, "twice-1", "twiceco")
	silentJob := seedResponseJob(t, q, "twice-2", "twiceco")
	for _, j := range []int64{answeredJob, silentJob} {
		if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: user, JobID: j, EventSource: "user"}); err != nil {
			t.Fatalf("apply: %v", err)
		}
	}
	seedReply(t, q, user, answeredJob, "twice-reply-1")

	if _, err := pool.Exec(ctx, `DELETE FROM jobs WHERE id = ANY($1::bigint[])`,
		[]int64{answeredJob, silentJob}); err != nil {
		t.Fatalf("prune both postings: %v", err)
	}

	apps, ans := rebuildAnswered(t, q, "twiceco")
	if apps != 2 {
		t.Errorf("applications = %d, want 2", apps)
	}
	if ans != 1 {
		t.Errorf("answered = %d, want exactly 1 — one reply answers one application, not every application to that employer", ans)
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
		`INSERT INTO emails (user_id, external_id, source, job_id, application_id, received_at)
		 VALUES ($1, 'move-1', 'gmail', $2, (SELECT a.id FROM applications a WHERE a.user_id = $1 AND a.job_id = $2), now()) RETURNING id`, user, jobA).Scan(&reply); err != nil {
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
	if _, err := pool.Exec(ctx, `UPDATE emails SET job_id = $2,
		    application_id = (SELECT a.id FROM applications a
		                       WHERE a.user_id = emails.user_id AND a.job_id = $2)
		 WHERE emails.id = $1`, reply, jobB); err != nil {
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

// The median is computed from the gap between an application and its first reply. Both
// dates live on the ledger and neither is read through the posting, so removing the
// posting must not move the number a visitor sees.
func TestRebuildInsightsCompanyResponse_MedianSurvivesPrunedPostings(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "median-prune@example.test", true)
	job := seedResponseJob(t, q, "median-1", "medianco")
	appliedAt := time.Now().Add(-10 * 24 * time.Hour).UTC().Truncate(time.Second)
	if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{
		UserID: user, JobID: job, EventSource: "user",
		At: pgtype.Timestamptz{Time: appliedAt, Valid: true},
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	var reply int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO emails (user_id, external_id, source, job_id, application_id, received_at)
		 VALUES ($1, 'median-reply', 'gmail', $2, (SELECT a.id FROM applications a WHERE a.user_id = $1 AND a.job_id = $2), $3) RETURNING id`,
		user, job, appliedAt.Add(4*24*time.Hour)).Scan(&reply); err != nil {
		t.Fatalf("seed reply: %v", err)
	}
	if err := q.RecordEmailApplicationEvent(ctx, RecordEmailApplicationEventParams{
		ID: reply, UserID: user, EventSource: "mail_gmail",
	}); err != nil {
		t.Fatalf("record reply: %v", err)
	}

	median := func() float32 {
		t.Helper()
		if err := q.DeleteAllInsightsCompanyResponse(ctx); err != nil {
			t.Fatalf("clear: %v", err)
		}
		if _, err := q.RebuildInsightsCompanyResponse(ctx); err != nil {
			t.Fatalf("rebuild: %v", err)
		}
		row, err := q.GetCompanyResponse(ctx, "medianco")
		if err != nil {
			t.Fatalf("GetCompanyResponse: %v", err)
		}
		if !row.MedianReplyDays.Valid {
			t.Fatal("no median served")
		}
		return row.MedianReplyDays.Float32
	}

	before := median()
	if before < 3.9 || before > 4.1 {
		t.Fatalf("median before the prune = %v days, want about 4", before)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM jobs WHERE id = $1`, job); err != nil {
		t.Fatalf("prune: %v", err)
	}
	if after := median(); after != before {
		t.Errorf("median = %v days after the posting was pruned, want %v — neither date is read through the posting", after, before)
	}
}
