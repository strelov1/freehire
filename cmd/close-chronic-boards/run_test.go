//go:build integration

// Integration tests for the chronic-board safety net's core pass (openspec change
// close-chronically-unreachable-boards, issue #2017): dry-run reports without writing,
// --apply closes exactly the chronic boards' open jobs, and a board short of the closure
// window is left alone even though it is chronic enough to appear in the operator report.
// Run with: go test -tags=integration ./cmd/close-chronic-boards/
package main

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/externalid"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func startPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return testdb.Pool(t)
}

func truncate(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		"TRUNCATE search_delete_outbox, jobs, companies, board_health RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

func ingestParams(externalID, title string) db.UpsertJobParams {
	return db.UpsertJobParams{
		Source:      "greenhouse",
		ExternalID:  externalID,
		URL:         "https://example.test/job",
		Title:       title,
		Company:     "Acme",
		CompanySlug: "acme",
		PublicSlug:  "pslug-" + externalID,
		Location:    "Remote",
		Remote:      true,
		Description: "Build things.",
		Category:    "backend",
	}
}

func seedChronicBoard(t *testing.T, pool *pgxpool.Pool, provider, board string, daysSinceSuccess int) {
	t.Helper()
	lastSuccess := time.Now().Add(-time.Duration(daysSinceSuccess) * 24 * time.Hour)
	_, err := pool.Exec(context.Background(),
		`INSERT INTO board_health (provider, board, region, consecutive_failures, first_seen_at, last_success_at)
		 VALUES ($1, $2, '', 20, now() - interval '500 days', $3)`,
		provider, board, lastSuccess)
	if err != nil {
		t.Fatalf("seed chronic board %s/%s: %v", provider, board, err)
	}
}

// TestCloseChronicBoardsDryRunTouchesNothing pins the safety-net's default: a run without
// --apply reports what it would close but writes nothing.
func TestCloseChronicBoardsDryRunTouchesNothing(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	ctx := context.Background()
	truncate(t, pool)

	seedChronicBoard(t, pool, "greenhouse", "dead-board", 61)
	job, err := q.UpsertJob(ctx, ingestParams(externalid.Namespace("dead-board", "1"), "Chronic"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	report, err := closeChronicBoards(ctx, q, 60, 100, false)
	if err != nil {
		t.Fatalf("closeChronicBoards (dry run): %v", err)
	}
	if report.boardsProcessed != 1 || report.jobsAffected != 1 {
		t.Fatalf("report = %+v, want 1 board and 1 job", report)
	}

	after, err := q.GetJob(ctx, job.Job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if after.ClosedAt.Valid {
		t.Fatal("dry run must not close the job")
	}
}

// TestCloseChronicBoardsApplyClosesExactlyTheChronicJobs pins --apply's scope: it closes
// the chronic board's own job, a sibling board of the same provider survives, and a board
// chronic-for-visibility but short of the closure window is left alone entirely.
func TestCloseChronicBoardsApplyClosesExactlyTheChronicJobs(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	ctx := context.Background()
	truncate(t, pool)

	seedChronicBoard(t, pool, "greenhouse", "dead-board", 61)
	seedChronicBoard(t, pool, "greenhouse", "short-of-closure", 40) // chronic (>30d) but <60d
	chronic, err := q.UpsertJob(ctx, ingestParams(externalid.Namespace("dead-board", "1"), "Chronic"))
	if err != nil {
		t.Fatalf("upsert chronic: %v", err)
	}
	shortOfClosure, err := q.UpsertJob(ctx, ingestParams(externalid.Namespace("short-of-closure", "1"), "Not due yet"))
	if err != nil {
		t.Fatalf("upsert short-of-closure: %v", err)
	}
	sibling, err := q.UpsertJob(ctx, ingestParams(externalid.Namespace("healthy-board", "1"), "Sibling"))
	if err != nil {
		t.Fatalf("upsert sibling: %v", err)
	}

	report, err := closeChronicBoards(ctx, q, 60, 100, true)
	if err != nil {
		t.Fatalf("closeChronicBoards (apply): %v", err)
	}
	if report.boardsProcessed != 1 || report.jobsAffected != 1 {
		t.Fatalf("report = %+v, want exactly 1 board past the 60-day closure window and 1 job closed", report)
	}

	chronicAfter, err := q.GetJob(ctx, chronic.Job.ID)
	if err != nil {
		t.Fatalf("get chronic: %v", err)
	}
	if !chronicAfter.ClosedAt.Valid || chronicAfter.ClosedReason != "board_unreachable" {
		t.Fatalf("chronic job = %+v, want closed with reason board_unreachable", chronicAfter)
	}

	shortAfter, err := q.GetJob(ctx, shortOfClosure.Job.ID)
	if err != nil {
		t.Fatalf("get short-of-closure: %v", err)
	}
	if shortAfter.ClosedAt.Valid {
		t.Fatal("a board chronic-for-visibility but short of the closure window must stay open")
	}

	siblingAfter, err := q.GetJob(ctx, sibling.Job.ID)
	if err != nil {
		t.Fatalf("get sibling: %v", err)
	}
	if siblingAfter.ClosedAt.Valid {
		t.Fatal("a healthy sibling board must stay open")
	}
}

// TestCloseChronicBoardsHandlesBoardlessProvider pins that a boardless provider's chronic
// health record (board = ”) routes to the source-scoped close, in the same run as an
// ordinary board-scoped one.
func TestCloseChronicBoardsHandlesBoardlessProvider(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	ctx := context.Background()
	truncate(t, pool)

	seedChronicBoard(t, pool, "uber", "", 61)
	p := ingestParams("uber-job-1", "Boardless")
	p.Source = "uber"
	p.PublicSlug = "uber-1"
	uberJob, err := q.UpsertJob(ctx, p)
	if err != nil {
		t.Fatalf("upsert uber job: %v", err)
	}

	report, err := closeChronicBoards(ctx, q, 60, 100, true)
	if err != nil {
		t.Fatalf("closeChronicBoards: %v", err)
	}
	if report.boardsProcessed != 1 || report.jobsAffected != 1 {
		t.Fatalf("report = %+v, want 1 board and 1 job", report)
	}

	after, err := q.GetJob(ctx, uberJob.Job.ID)
	if err != nil {
		t.Fatalf("get uber job: %v", err)
	}
	if !after.ClosedAt.Valid || after.ClosedReason != "board_unreachable" {
		t.Fatalf("uber job = %+v, want closed with reason board_unreachable", after)
	}
}
