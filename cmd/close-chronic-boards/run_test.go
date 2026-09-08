//go:build integration

// Integration tests for the chronic-board safety net's core pass (openspec change
// close-chronically-unreachable-boards, issue #2017): dry-run reports without writing,
// --apply closes exactly the chronic boards' open jobs, and a board short of the closure
// window is left alone even though it is chronic enough to appear in the operator report.
// Run with: go test -tags=integration ./cmd/close-chronic-boards/
package main

import (
	"bytes"
	"context"
	"log"
	"os"
	"strings"
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
	seedBoardHealthRegion(t, pool, provider, board, "", daysSinceSuccess)
}

// seedBoardHealthRegion is seedChronicBoard with an explicit region, for the region-ambiguity
// tests: Adzuna-shaped providers register the same board name once per country.
func seedBoardHealthRegion(t *testing.T, pool *pgxpool.Pool, provider, board, region string, daysSinceSuccess int) {
	t.Helper()
	lastSuccess := time.Now().Add(-time.Duration(daysSinceSuccess) * 24 * time.Hour)
	_, err := pool.Exec(context.Background(),
		`INSERT INTO board_health (provider, board, region, consecutive_failures, first_seen_at, last_success_at)
		 VALUES ($1, $2, $3, 20, now() - interval '500 days', $4)`,
		provider, board, region, lastSuccess)
	if err != nil {
		t.Fatalf("seed chronic board %s/%s/%s: %v", provider, board, region, err)
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

// TestCloseChronicBoardsSkipsRegionAmbiguousBoardNames mirrors
// TestBoardHealth_RegionDisambiguates (cmd/ingest): Adzuna registers the same board name once
// per country, so "it-jobs" is chronic in gb while still crawling fine in us. jobs.external_id
// carries no region dimension, so a board-scoped close on the name "it-jobs" cannot tell gb's
// postings from us's — the worker must refuse to touch it at all, the same way the ordinary
// sweep's ambiguousRegionBoards falls back rather than risk closing a healthy region's jobs.
func TestCloseChronicBoardsSkipsRegionAmbiguousBoardNames(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	ctx := context.Background()
	truncate(t, pool)

	seedBoardHealthRegion(t, pool, "adzuna", "it-jobs", "gb", 61) // chronic
	seedBoardHealthRegion(t, pool, "adzuna", "it-jobs", "us", 0)  // healthy, crawled today
	job, err := q.UpsertJob(ctx, ingestParams(externalid.Namespace("it-jobs", "1"), "Ambiguous board job"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	report, err := closeChronicBoards(ctx, q, 60, 100, true)
	if err != nil {
		t.Fatalf("closeChronicBoards: %v", err)
	}
	if report.boardsSkippedAmbiguous != 1 {
		t.Fatalf("report = %+v, want 1 board skipped as region-ambiguous", report)
	}
	if report.jobsAffected != 0 {
		t.Fatalf("report = %+v, want 0 jobs affected — the ambiguous board must not be touched", report)
	}

	after, err := q.GetJob(ctx, job.Job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if after.ClosedAt.Valid {
		t.Fatal("a region-ambiguous board's job must stay open — closing it risks a healthy region's postings")
	}
}

// The dry-run count path must refuse the same way: a report naming a job count for an
// ambiguous board would mix two regions' postings into one misleading number.
func TestCloseChronicBoardsDryRunSkipsRegionAmbiguousBoardNames(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	ctx := context.Background()
	truncate(t, pool)

	seedBoardHealthRegion(t, pool, "adzuna", "it-jobs", "gb", 61)
	seedBoardHealthRegion(t, pool, "adzuna", "it-jobs", "us", 0)
	if _, err := q.UpsertJob(ctx, ingestParams(externalid.Namespace("it-jobs", "1"), "Ambiguous board job")); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	report, err := closeChronicBoards(ctx, q, 60, 100, false)
	if err != nil {
		t.Fatalf("closeChronicBoards (dry run): %v", err)
	}
	if report.boardsSkippedAmbiguous != 1 || report.jobsAffected != 0 {
		t.Fatalf("report = %+v, want 1 board skipped and 0 jobs counted", report)
	}
}

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	flags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetFlags(flags)
	})
	return &buf
}

// TestCloseChronicBoardsLogsWhenTheCapTruncates pins that a run bound by maxBoards says so: a
// run at exactly the cap must not look identical to one with many more chronic boards behind
// it, since maxChronicBoardsPerRun's own comment (main.go) argues that case would itself be
// worth a human noticing.
func TestCloseChronicBoardsLogsWhenTheCapTruncates(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	ctx := context.Background()
	truncate(t, pool)

	seedChronicBoard(t, pool, "greenhouse", "dead-board-1", 61)
	seedChronicBoard(t, pool, "greenhouse", "dead-board-2", 61)
	seedChronicBoard(t, pool, "greenhouse", "dead-board-3", 61)

	logs := captureLog(t)
	report, err := closeChronicBoards(ctx, q, 60, 2, false) // cap of 2 against 3 chronic boards
	if err != nil {
		t.Fatalf("closeChronicBoards: %v", err)
	}
	if report.boardsProcessed != 2 {
		t.Fatalf("boardsProcessed = %d, want 2 (bound by the cap)", report.boardsProcessed)
	}
	if !strings.Contains(logs.String(), "1 more chronic board") {
		t.Errorf("log did not report the truncation: %s", logs.String())
	}
}

// A run that is NOT truncated (fewer chronic boards than the cap) must not print a false "more
// boards exist" line.
func TestCloseChronicBoardsDoesNotLogTruncationWhenNotCapped(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	ctx := context.Background()
	truncate(t, pool)

	seedChronicBoard(t, pool, "greenhouse", "dead-board", 61)

	logs := captureLog(t)
	if _, err := closeChronicBoards(ctx, q, 60, 100, false); err != nil {
		t.Fatalf("closeChronicBoards: %v", err)
	}
	if strings.Contains(logs.String(), "more chronic board") {
		t.Errorf("log falsely reported truncation with no cap hit: %s", logs.String())
	}
}
