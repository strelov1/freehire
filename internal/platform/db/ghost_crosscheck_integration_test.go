//go:build integration

// Integration tests for the ghost cross-check queries: which postings are candidates,
// what a company's own board reports, and the stamp/clear round trip. The verdict
// itself is decided in internal/job/ghost and tested there without a database.
// Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"testing"
)

func insertCrosscheckJob(t *testing.T, q *Queries, source, extID, company, title string) int64 {
	t.Helper()
	var id int64
	err := q.db.QueryRow(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, company_slug, public_slug)
		 VALUES ($1, $2, 'http://example.test/x', $3, $4, $5) RETURNING id`,
		source, extID, title, company, extID+"-slug").Scan(&id)
	if err != nil {
		t.Fatalf("insert job %s: %v", extID, err)
	}
	return id
}

func TestGhostCrosscheck_CandidatesAreOpenAggregatorPostings(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	wanted := insertCrosscheckJob(t, q, "jobstash", "xc-1", "acme", "Go Developer")
	insertCrosscheckJob(t, q, "greenhouse", "xc-2", "acme", "Go Developer") // not an aggregator
	closed := insertCrosscheckJob(t, q, "jobstash", "xc-3", "acme", "Rust Developer")
	if _, err := pool.Exec(ctx, `UPDATE jobs SET closed_at = now() WHERE id = $1`, closed); err != nil {
		t.Fatalf("close job: %v", err)
	}

	rows, err := q.ListAggregatorJobsForCrosscheckBySource(ctx, ListAggregatorJobsForCrosscheckBySourceParams{
		Source:   "jobstash",
		AfterID:  0,
		PageSize: 100,
	})
	if err != nil {
		t.Fatalf("ListAggregatorJobsForCrosscheckBySource: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != wanted {
		t.Fatalf("rows = %+v, want only the open jobstash posting %d", rows, wanted)
	}
	if rows[0].AtsAbsentAt.Valid {
		t.Error("a fresh posting already carries an absence stamp")
	}
}

// The coverage gate's data: a company we know only through aggregators has no board
// of its own, so the query reports nothing and the worker judges nothing. Without
// this the signal would report our own crawl coverage as the employer's fault.
func TestGhostCrosscheck_AggregatorOnlyCompanyHasNoBoardTitles(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	insertCrosscheckJob(t, q, "jobstash", "gate-1", "onlyagg", "Go Developer")
	insertCrosscheckJob(t, q, "justjoin", "gate-2", "onlyagg", "Rust Developer")

	titles, err := q.ListCompanyBoardTitles(ctx, ListCompanyBoardTitlesParams{
		CompanySlug:  "onlyagg",
		BoardSources: []string{"greenhouse", "lever"},
	})
	if err != nil {
		t.Fatalf("ListCompanyBoardTitles: %v", err)
	}
	if len(titles) != 0 {
		t.Errorf("titles = %v, want none — this company has no board of ours", titles)
	}
}

func TestGhostCrosscheck_BoardTitlesComeFromTheCompanysOwnSources(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	insertCrosscheckJob(t, q, "greenhouse", "board-1", "hasboard", "Go Developer")
	insertCrosscheckJob(t, q, "jobstash", "board-2", "hasboard", "Aggregated Role")
	other := insertCrosscheckJob(t, q, "greenhouse", "board-3", "hasboard", "Closed Role")
	if _, err := pool.Exec(ctx, `UPDATE jobs SET closed_at = now() WHERE id = $1`, other); err != nil {
		t.Fatalf("close job: %v", err)
	}

	titles, err := q.ListCompanyBoardTitles(ctx, ListCompanyBoardTitlesParams{
		CompanySlug:  "hasboard",
		BoardSources: []string{"greenhouse", "lever"},
	})
	if err != nil {
		t.Fatalf("ListCompanyBoardTitles: %v", err)
	}
	if len(titles) != 1 || titles[0] != "Go Developer" {
		t.Errorf("titles = %v, want only the open greenhouse title", titles)
	}
}

// The stamp tracks the world rather than accumulating.
func TestGhostCrosscheck_StampAndClearRoundTrip(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	id := insertCrosscheckJob(t, q, "jobstash", "stamp-1", "acme", "Go Developer")

	if err := q.StampJobATSAbsent(ctx, []int64{id}); err != nil {
		t.Fatalf("StampJobATSAbsent: %v", err)
	}
	rows, err := q.ListAggregatorJobsForCrosscheckBySource(ctx, ListAggregatorJobsForCrosscheckBySourceParams{
		Source: "jobstash", AfterID: 0, PageSize: 100,
	})
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if len(rows) != 1 || !rows[0].AtsAbsentAt.Valid {
		t.Fatalf("rows = %+v, want the posting stamped", rows)
	}

	if err := q.ClearJobATSAbsent(ctx, []int64{id}); err != nil {
		t.Fatalf("ClearJobATSAbsent: %v", err)
	}
	rows, err = q.ListAggregatorJobsForCrosscheckBySource(ctx, ListAggregatorJobsForCrosscheckBySourceParams{
		Source: "jobstash", AfterID: 0, PageSize: 100,
	})
	if err != nil {
		t.Fatalf("read back after clear: %v", err)
	}
	if len(rows) != 1 || rows[0].AtsAbsentAt.Valid {
		t.Fatalf("rows = %+v, want the stamp withdrawn", rows)
	}
}

// via_recruiter is true-or-NULL, and the candidate read reports NULL as false rather
// than failing to scan it.
func TestGhostCrosscheck_CandidatesCarryTheRecruiterStatement(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	recruiter := insertCrosscheckJob(t, q, "jobstash", "vr-1", "acme-staffing", "Go Developer")
	insertCrosscheckJob(t, q, "jobstash", "vr-2", "acme", "Go Developer")
	if _, err := pool.Exec(ctx, `UPDATE jobs SET via_recruiter = true WHERE id = $1`, recruiter); err != nil {
		t.Fatalf("mark recruiter: %v", err)
	}

	rows, err := q.ListAggregatorJobsForCrosscheckBySource(ctx, ListAggregatorJobsForCrosscheckBySourceParams{
		Source: "jobstash", AfterID: 0, PageSize: 100,
	})
	if err != nil {
		t.Fatalf("ListAggregatorJobsForCrosscheckBySource: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %+v, want both postings", rows)
	}
	for _, r := range rows {
		if want := r.ID == recruiter; r.ViaRecruiter != want {
			t.Errorf("posting %d: ViaRecruiter = %v, want %v", r.ID, r.ViaRecruiter, want)
		}
	}
}

// The agency count covers every source and only open, public postings that say so.
func TestGhostCrosscheck_CountCompanyRecruiterPostings(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	said := insertCrosscheckJob(t, q, "jobstash", "cr-1", "acme-staffing", "Go Developer")
	board := insertCrosscheckJob(t, q, "greenhouse", "cr-2", "acme-staffing", "Rust Developer")
	closed := insertCrosscheckJob(t, q, "jobstash", "cr-3", "acme-staffing", "Java Developer")
	insertCrosscheckJob(t, q, "jobstash", "cr-4", "acme-staffing", "Silent Developer")
	other := insertCrosscheckJob(t, q, "jobstash", "cr-5", "acme", "Go Developer")
	if _, err := pool.Exec(ctx, `UPDATE jobs SET via_recruiter = true WHERE id = ANY($1)`,
		[]int64{said, board, closed, other}); err != nil {
		t.Fatalf("mark recruiter: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET closed_at = now() WHERE id = $1`, closed); err != nil {
		t.Fatalf("close job: %v", err)
	}

	n, err := q.CountCompanyRecruiterPostings(ctx, "acme-staffing")
	if err != nil {
		t.Fatalf("CountCompanyRecruiterPostings: %v", err)
	}
	if n != 2 {
		t.Errorf("count = %d, want 2 — the open aggregator and board postings that say so", n)
	}
}
