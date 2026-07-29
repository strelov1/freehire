//go:build integration

// Integration tests for the job-report decision SQL: GetReport carries the reporter and
// job context a decision notice needs, and MarkReportResolved stores the moderator's note
// in the same review_reason column dismiss already writes. Both are joins and column
// writes, so only a real Postgres proves them. Run with:
//
//	go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// seedReport files a pending report by reporter against job, returning its id.
func seedReport(t *testing.T, pool *pgxpool.Pool, reporterID, jobID int64) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO job_reports (reported_by, job_id, reason, details)
		 VALUES ($1, $2, 'not_relevant', 'listed remote, the source says hybrid') RETURNING id`,
		reporterID, jobID).Scan(&id)
	if err != nil {
		t.Fatalf("insert report: %v", err)
	}
	return id
}

func TestGetReportCarriesReporterAndJobContext(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	uid := seedAPIKeyUser(t, pool, "reporter@example.test")
	jid := insertJob(t, pool, "report-context-1")
	rid := seedReport(t, pool, uid, jid)

	row, err := q.GetReport(ctx, rid)
	if err != nil {
		t.Fatalf("GetReport: %v", err)
	}
	if row.ReporterEmail != "reporter@example.test" {
		t.Errorf("ReporterEmail = %q, want reporter@example.test", row.ReporterEmail)
	}
	if row.JobTitle != "A job" {
		t.Errorf("JobTitle = %q, want 'A job'", row.JobTitle)
	}
	if row.JobSlug != "job-report-context-1" {
		t.Errorf("JobSlug = %q, want job-report-context-1", row.JobSlug)
	}
}

func TestMarkReportResolvedStoresTheModeratorNote(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	modID := seedAPIKeyUser(t, pool, "note-mod@example.test")
	uid := seedAPIKeyUser(t, pool, "note-reporter@example.test")
	jid := insertJob(t, pool, "report-note-1")
	rid := seedReport(t, pool, uid, jid)

	const note = "Fixed — the job is now marked hybrid"
	row, err := q.MarkReportResolved(ctx, MarkReportResolvedParams{
		ID: rid, ReviewedBy: modID, ReviewReason: note,
	})
	if err != nil {
		t.Fatalf("MarkReportResolved: %v", err)
	}
	if row.Status != "resolved" {
		t.Errorf("Status = %q, want resolved", row.Status)
	}
	if row.ReviewReason != note {
		t.Errorf("ReviewReason = %q, want %q", row.ReviewReason, note)
	}
}
