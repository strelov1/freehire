//go:build integration

// Integration tests for the reply-rate benchmark's two new reads: the global
// (all-companies) response rate summed over the existing per-company rollup, and the
// live per-user observable/answered count the pipeline endpoint reads directly from
// application_events. The ten-application sample gate is applied by the serving layer
// and tested there, not here — same split company_response_integration_test.go already
// documents for the per-company figure.
// Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"testing"
)

func TestGetGlobalCompanyResponse_SumsAcrossCompanies(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	answered := seedResponseUser(t, q, "global-answered@example.test", true)
	silent := seedResponseUser(t, q, "global-silent@example.test", true)
	jobA := seedResponseJob(t, q, "global-a", "globalco-a")
	jobB := seedResponseJob(t, q, "global-b", "globalco-b")

	for _, a := range []struct{ uid, jid int64 }{{answered, jobA}, {silent, jobB}} {
		if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: a.uid, JobID: a.jid, EventSource: "user"}); err != nil {
			t.Fatalf("MarkJobApplied: %v", err)
		}
	}
	seedReply(t, q, answered, jobA, "global-reply-1")

	if _, err := q.RebuildInsightsCompanyResponse(ctx); err != nil {
		t.Fatalf("RebuildInsightsCompanyResponse: %v", err)
	}

	got, err := q.GetGlobalCompanyResponse(ctx)
	if err != nil {
		t.Fatalf("GetGlobalCompanyResponse: %v", err)
	}
	if got.Applications != 2 || got.Answered != 1 {
		t.Errorf("got %+v, want 2 applications and 1 answered summed across both companies", got)
	}
}

// Neither company individually reaches the ten-application sample gate that governs
// what is safe to serve about one NAMED employer, but the global figure aggregates
// across all of them regardless — the per-company gate protects a named company's
// privacy, not what may contribute to an aggregate.
func TestGetGlobalCompanyResponse_IncludesCompaniesBelowTheirOwnGate(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	userA := seedResponseUser(t, q, "small-a@example.test", true)
	userB := seedResponseUser(t, q, "small-b@example.test", true)
	jobA := seedResponseJob(t, q, "small-a", "smallco-a")
	jobB := seedResponseJob(t, q, "small-b", "smallco-b")

	for _, a := range []struct{ uid, jid int64 }{{userA, jobA}, {userB, jobB}} {
		if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: a.uid, JobID: a.jid, EventSource: "user"}); err != nil {
			t.Fatalf("MarkJobApplied: %v", err)
		}
	}

	if _, err := q.RebuildInsightsCompanyResponse(ctx); err != nil {
		t.Fatalf("RebuildInsightsCompanyResponse: %v", err)
	}

	got, err := q.GetGlobalCompanyResponse(ctx)
	if err != nil {
		t.Fatalf("GetGlobalCompanyResponse: %v", err)
	}
	// Each test runs against its own freshly cloned database (testdb.Pool), so an exact
	// match is safe here, not just a floor: neither company clears its own ten-application
	// gate alone, but the sum must be precisely 2 applications and 0 answered — a query
	// that fanned out a join, or double-counted either company, would still clear a "< 2"
	// floor and hide the regression.
	if got.Applications != 2 || got.Answered != 0 {
		t.Errorf("got %+v, want 2 applications and 0 answered — small companies must still contribute to the global sum", got)
	}
}

func TestGetUserResponseRate_CountsObservableAndAnswered(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "personal-user@example.test", true)
	jobAnswered := seedResponseJob(t, q, "personal-1", "personalco-1")
	jobSilent := seedResponseJob(t, q, "personal-2", "personalco-2")

	for _, j := range []int64{jobAnswered, jobSilent} {
		if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: user, JobID: j, EventSource: "user"}); err != nil {
			t.Fatalf("MarkJobApplied: %v", err)
		}
	}
	seedReply(t, q, user, jobAnswered, "personal-reply-1")

	got, err := q.GetUserResponseRate(ctx, user)
	if err != nil {
		t.Fatalf("GetUserResponseRate: %v", err)
	}
	if got.Applications != 2 || got.Answered != 1 {
		t.Errorf("got %+v, want 2 applications and 1 answered", got)
	}
}

// A caller with no connected mailbox has no observable applications at all: the same
// gap-in-our-data reasoning the per-company rollup already applies. This is what lets
// the serving layer treat "no mailbox" and "too few applications" as the same absence,
// without a separate boolean check.
func TestGetUserResponseRate_NoConnectedMailboxIsZeroObservable(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "no-mailbox@example.test", false)
	job := seedResponseJob(t, q, "no-mailbox-1", "nomailboxco")
	if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: user, JobID: job, EventSource: "user"}); err != nil {
		t.Fatalf("MarkJobApplied: %v", err)
	}

	got, err := q.GetUserResponseRate(ctx, user)
	if err != nil {
		t.Fatalf("GetUserResponseRate: %v", err)
	}
	if got.Applications != 0 || got.Answered != 0 {
		t.Errorf("got %+v, want 0 and 0 — an application from a user with no connected mailbox is not observable", got)
	}
}

func TestGetUserResponseRate_ScopedToOneUser(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	userA := seedResponseUser(t, q, "scoped-a@example.test", true)
	userB := seedResponseUser(t, q, "scoped-b@example.test", true)
	jobA := seedResponseJob(t, q, "scoped-a", "scopedco-a")
	jobB := seedResponseJob(t, q, "scoped-b", "scopedco-b")

	if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: userA, JobID: jobA, EventSource: "user"}); err != nil {
		t.Fatalf("MarkJobApplied A: %v", err)
	}
	if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: userB, JobID: jobB, EventSource: "user"}); err != nil {
		t.Fatalf("MarkJobApplied B: %v", err)
	}
	seedReply(t, q, userB, jobB, "scoped-reply-1")

	got, err := q.GetUserResponseRate(ctx, userA)
	if err != nil {
		t.Fatalf("GetUserResponseRate: %v", err)
	}
	if got.Applications != 1 || got.Answered != 0 {
		t.Errorf("got %+v for userA, want 1 application and 0 answered — userB's reply must not leak in", got)
	}
}

// A retracted reply must not count as an answer on the personal side either. The
// retraction is set directly (rather than through RetractSupersededEmailEvent, which
// only fires as a side effect of a link correction actually changing the target) to
// isolate what this test cares about: whether GetUserResponseRate's own predicate
// honors retracted_at, independent of how a row comes to be retracted.
func TestGetUserResponseRate_RetractedReplyDoesNotCount(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "retract-user@example.test", true)
	job := seedResponseJob(t, q, "retract-1", "retractco")
	if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: user, JobID: job, EventSource: "user"}); err != nil {
		t.Fatalf("MarkJobApplied: %v", err)
	}
	seedReply(t, q, user, job, "retract-reply-1")

	if _, err := pool.Exec(ctx,
		`UPDATE application_events SET retracted_at = now() WHERE kind = 'employer_reply' AND user_id = $1`,
		user); err != nil {
		t.Fatalf("retract event: %v", err)
	}

	got, err := q.GetUserResponseRate(ctx, user)
	if err != nil {
		t.Fatalf("GetUserResponseRate: %v", err)
	}
	if got.Answered != 0 {
		t.Errorf("answered = %d, want 0 — a retracted reply must not count", got.Answered)
	}
}

// cmd/prune is the only hard-delete path for jobs, and application_events.job_id is ON
// DELETE SET NULL — so a removal must not change what this query says about the caller.
// The denominator survives on its own (observable's WHERE clause never joins jobs). The
// numerator is the half at risk: the reply is matched back to its application through
// application_id, and a query that instead joined through job_id would see two cleared
// references and report a silent employer, the exact distortion
// RebuildInsightsCompanyResponse's own comment (above) documents.
func TestGetUserResponseRate_SurvivesPrunedPosting(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedResponseUser(t, q, "prune-personal@example.test", true)
	job := seedResponseJob(t, q, "prune-personal-1", "prunepersonalco")
	if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: user, JobID: job, EventSource: "user"}); err != nil {
		t.Fatalf("MarkJobApplied: %v", err)
	}
	seedReply(t, q, user, job, "prune-personal-reply-1")

	before, err := q.GetUserResponseRate(ctx, user)
	if err != nil {
		t.Fatalf("GetUserResponseRate before prune: %v", err)
	}
	if before.Applications != 1 || before.Answered != 1 {
		t.Fatalf("before the prune: got %+v, want 1 application and 1 answered", before)
	}

	if _, err := pool.Exec(ctx, `DELETE FROM jobs WHERE id = $1`, job); err != nil {
		t.Fatalf("prune the posting: %v", err)
	}

	after, err := q.GetUserResponseRate(ctx, user)
	if err != nil {
		t.Fatalf("GetUserResponseRate after prune: %v", err)
	}
	if after.Applications != 1 {
		t.Errorf("applications = %d after the posting was pruned, want 1", after.Applications)
	}
	if after.Answered != 1 {
		t.Errorf("answered = %d after the posting was pruned, want 1 — an employer that replied must not be served as silent", after.Answered)
	}
}
