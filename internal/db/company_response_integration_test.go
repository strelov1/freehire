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
		if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: a.uid, JobID: a.jid}); err != nil {
			t.Fatalf("MarkJobApplied: %v", err)
		}
	}
	// One reply arrives, linked to the first application.
	if _, err := pool.Exec(ctx,
		`INSERT INTO emails (user_id, external_id, source, subject, job_id, received_at)
		 VALUES ($1, 'reply-1', 'gmail', 'Thanks for applying', $2, now())`, answered, job1); err != nil {
		t.Fatalf("seed reply: %v", err)
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
		if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: a.uid, JobID: a.jid}); err != nil {
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
