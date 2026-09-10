//go:build integration

// Integration tests for the empty-feed safety net (migration 0158): a board whose crawls all
// SUCCEED and whose feed carries nothing eventually has its jobs closed with 'feed_empty', while
// the three things that merely LOOK like an empty feed are left strictly alone.
// Run with: go test -tags=integration ./cmd/close-chronic-boards/
package main

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/externalid"
)

// seedEmptyFeedBoard writes a board_health row shaped like a reachable-but-empty board: it
// succeeded daysSinceYield ago and every day since (last_success_at is fresh, failures zero),
// but its last actual yield is daysSinceYield old.
func seedEmptyFeedBoard(t *testing.T, pool *pgxpool.Pool, provider, board string, daysSinceYield int) {
	t.Helper()
	seedEmptyFeedBoardRegion(t, pool, provider, board, "", daysSinceYield)
}

func seedEmptyFeedBoardRegion(t *testing.T, pool *pgxpool.Pool, provider, board, region string, daysSinceYield int) {
	t.Helper()
	lastYield := time.Now().Add(-time.Duration(daysSinceYield) * 24 * time.Hour)
	_, err := pool.Exec(context.Background(),
		`INSERT INTO board_health (provider, board, region, consecutive_failures, first_seen_at,
		                           last_success_at, last_yield_at)
		 VALUES ($1, $2, $3, 0, now() - interval '500 days', now() - interval '1 hour', $4)`,
		provider, board, region, lastYield)
	if err != nil {
		t.Fatalf("seed empty-feed board %s/%s/%s: %v", provider, board, region, err)
	}
}

// TestCloseEmptyFeedBoardsDryRunTouchesNothing pins the default, same as the chronic pass:
// reporting is what a run without --apply does, and it writes nothing.
func TestCloseEmptyFeedBoardsDryRunTouchesNothing(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	ctx := context.Background()
	truncate(t, pool)

	seedEmptyFeedBoard(t, pool, "greenhouse", "drained-board", 40)
	job, err := q.UpsertJob(ctx, ingestParams(externalid.Namespace("drained-board", "1"), "Stale"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	report, err := closeEmptyFeedBoards(ctx, q, 30, 100, false)
	if err != nil {
		t.Fatalf("closeEmptyFeedBoards (dry run): %v", err)
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

// TestCloseEmptyFeedBoardsApplyClosesWithItsOwnReason pins both halves of what --apply does:
// the job closes, and it closes as 'feed_empty' rather than borrowing a reason that would send
// an operator looking in the wrong place.
func TestCloseEmptyFeedBoardsApplyClosesWithItsOwnReason(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	ctx := context.Background()
	truncate(t, pool)

	seedEmptyFeedBoard(t, pool, "greenhouse", "drained-board", 40)
	job, err := q.UpsertJob(ctx, ingestParams(externalid.Namespace("drained-board", "1"), "Stale"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	if _, err := closeEmptyFeedBoards(ctx, q, 30, 100, true); err != nil {
		t.Fatalf("closeEmptyFeedBoards (apply): %v", err)
	}

	after, err := q.GetJob(ctx, job.Job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if !after.ClosedAt.Valid {
		t.Fatal("--apply must close an empty-feed board's job")
	}
	if after.ClosedReason != "feed_empty" {
		t.Errorf("closed_reason = %q, want %q — conflating this with another mechanism loses the "+
			"one thing the column is for", after.ClosedReason, "feed_empty")
	}
}

// TestCloseEmptyFeedBoardsSparesABoardThatNeverRecordedAYield is the guard against the mistake
// that would close the entire catalogue. Migration 0158 seeds last_yield_at for every existing
// row precisely so this case is rare — but a row can still carry NULL (one that has never
// succeeded, or one written between the ALTER and the backfill), and NULL means "no yield has
// been OBSERVED", which is not the same claim as "this feed is empty".
func TestCloseEmptyFeedBoardsSparesABoardThatNeverRecordedAYield(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	ctx := context.Background()
	truncate(t, pool)

	if _, err := pool.Exec(ctx,
		`INSERT INTO board_health (provider, board, region, consecutive_failures, first_seen_at,
		                           last_success_at, last_yield_at)
		 VALUES ('greenhouse', 'never-measured', '', 0, now() - interval '500 days',
		         now() - interval '1 hour', NULL)`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := q.UpsertJob(ctx, ingestParams(externalid.Namespace("never-measured", "1"), "Live")); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	report, err := closeEmptyFeedBoards(ctx, q, 30, 100, true)
	if err != nil {
		t.Fatalf("closeEmptyFeedBoards: %v", err)
	}
	if report.boardsProcessed != 0 || report.jobsAffected != 0 {
		t.Fatalf("report = %+v, want nothing: a NULL last_yield_at is an unmeasured board, not an empty feed", report)
	}
}

// TestCloseEmptyFeedBoardsSparesAnUnreachableBoard keeps the two diagnoses apart. A board nobody
// can read also has an old last_yield_at — for the entirely different reason that nothing has
// been able to reach it — and closing it here would file a broken adapter under a reason that
// points at the source's inventory. It belongs to the chronic pass, which measures
// last_success_at.
func TestCloseEmptyFeedBoardsSparesAnUnreachableBoard(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	ctx := context.Background()
	truncate(t, pool)

	if _, err := pool.Exec(ctx,
		`INSERT INTO board_health (provider, board, region, consecutive_failures, first_seen_at,
		                           last_success_at, last_yield_at)
		 VALUES ('greenhouse', 'unreachable', '', 20, now() - interval '500 days',
		         now() - interval '90 days', now() - interval '90 days')`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := q.UpsertJob(ctx, ingestParams(externalid.Namespace("unreachable", "1"), "Stale")); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	report, err := closeEmptyFeedBoards(ctx, q, 30, 100, true)
	if err != nil {
		t.Fatalf("closeEmptyFeedBoards: %v", err)
	}
	if report.boardsProcessed != 0 || report.jobsAffected != 0 {
		t.Fatalf("report = %+v, want nothing: an unreachable board is the chronic pass's business", report)
	}
}

// TestCloseEmptyFeedBoardsSparesABoardInsideTheWindow: a feed that went quiet last week is not
// yet evidence of anything.
func TestCloseEmptyFeedBoardsSparesABoardInsideTheWindow(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	ctx := context.Background()
	truncate(t, pool)

	seedEmptyFeedBoard(t, pool, "greenhouse", "quiet-lately", 7)
	if _, err := q.UpsertJob(ctx, ingestParams(externalid.Namespace("quiet-lately", "1"), "Live")); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	report, err := closeEmptyFeedBoards(ctx, q, 30, 100, true)
	if err != nil {
		t.Fatalf("closeEmptyFeedBoards: %v", err)
	}
	if report.boardsProcessed != 0 || report.jobsAffected != 0 {
		t.Fatalf("report = %+v, want nothing for a board 7 days quiet against a 30-day window", report)
	}
}

// TestCloseEmptyFeedBoardsSparesASiblingBoard pins the board scoping: an empty board's close
// must not reach the same provider's healthy neighbour.
func TestCloseEmptyFeedBoardsSparesASiblingBoard(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	ctx := context.Background()
	truncate(t, pool)

	seedEmptyFeedBoard(t, pool, "greenhouse", "drained-board", 40)
	seedEmptyFeedBoard(t, pool, "greenhouse", "healthy-board", 0)
	drained, err := q.UpsertJob(ctx, ingestParams(externalid.Namespace("drained-board", "1"), "Stale"))
	if err != nil {
		t.Fatalf("upsert drained: %v", err)
	}
	sibling, err := q.UpsertJob(ctx, ingestParams(externalid.Namespace("healthy-board", "2"), "Live"))
	if err != nil {
		t.Fatalf("upsert sibling: %v", err)
	}

	if _, err := closeEmptyFeedBoards(ctx, q, 30, 100, true); err != nil {
		t.Fatalf("closeEmptyFeedBoards: %v", err)
	}

	closedRow, err := q.GetJob(ctx, drained.Job.ID)
	if err != nil {
		t.Fatalf("get drained: %v", err)
	}
	if !closedRow.ClosedAt.Valid {
		t.Error("the drained board's job should have closed")
	}
	siblingRow, err := q.GetJob(ctx, sibling.Job.ID)
	if err != nil {
		t.Fatalf("get sibling: %v", err)
	}
	if siblingRow.ClosedAt.Valid {
		t.Error("a healthy sibling board's job must not close alongside it")
	}
}

// TestCloseEmptyFeedBoardsSkipsARegionAmbiguousBoard: jobs.external_id carries no region, so a
// board name registered under two regions cannot be board-scoped without risking a healthy
// region's postings — the same refusal the chronic pass makes.
func TestCloseEmptyFeedBoardsSkipsARegionAmbiguousBoard(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	ctx := context.Background()
	truncate(t, pool)

	seedEmptyFeedBoardRegion(t, pool, "adzuna", "it-jobs", "hu", 40)
	seedEmptyFeedBoardRegion(t, pool, "adzuna", "it-jobs", "at", 40)
	if _, err := q.UpsertJob(ctx, ingestParams(externalid.Namespace("it-jobs", "1"), "Ambiguous")); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	report, err := closeEmptyFeedBoards(ctx, q, 30, 100, true)
	if err != nil {
		t.Fatalf("closeEmptyFeedBoards: %v", err)
	}
	if report.boardsSkippedAmbiguous != 2 || report.jobsAffected != 0 {
		t.Fatalf("report = %+v, want both region rows skipped and nothing closed", report)
	}
}
