//go:build integration

package main

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/externalid"
)

func seedBoardGoneBoard(t *testing.T, pool *pgxpool.Pool, q *db.Queries, provider, board, region string) {
	t.Helper()
	ctx := context.Background()
	for range 3 {
		if _, err := q.RecordBoardFailure(ctx, db.RecordBoardFailureParams{
			Provider: provider,
			Board:    board,
			Region:   region,
			LastError: pgtype.Text{
				String: "teamtailor: career site: board no longer exists on the platform",
				Valid:  true,
			},
			BoardGone: true,
		}); err != nil {
			t.Fatalf("record board-gone failure for %s/%s/%s: %v", provider, board, region, err)
		}
	}
	if _, err := pool.Exec(ctx,
		`UPDATE board_health SET board_gone_since = now() - interval '8 days',
		 last_error_at = now() - interval '1 day' WHERE provider = $1 AND board = $2 AND region = $3`,
		provider, board, region); err != nil {
		t.Fatalf("age board-gone health %s/%s/%s: %v", provider, board, region, err)
	}
}

func seedBoardGoneSibling(t *testing.T, q *db.Queries, provider string) {
	t.Helper()
	if _, err := q.RecordBoardSuccess(context.Background(), db.RecordBoardSuccessParams{
		Provider: provider, Board: "healthy-sibling", Region: "",
		LastIngestedCount: pgtype.Int4{Int32: 1, Valid: true}, Reached: true,
	}); err != nil {
		t.Fatalf("record healthy sibling for %s: %v", provider, err)
	}
}

func upsertBoardGoneJob(t *testing.T, q *db.Queries, board, id string) int64 {
	t.Helper()
	params := ingestParams(externalid.Namespace(board, id), "Stale role")
	params.Source = "teamtailor"
	job, err := q.UpsertJob(context.Background(), params)
	if err != nil {
		t.Fatalf("upsert %s/%s job: %v", board, id, err)
	}
	return job.Job.ID
}

// TestCloseBoardGoneWorkerDryRunAndApply exercises the worker's independently armed path
// end to end: reporting counts the candidate without writing, then apply closes only that
// board's jobs and leaves its live sibling intact.
func TestCloseBoardGoneWorkerDryRunAndApply(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	ctx := context.Background()
	truncate(t, pool)
	seedBoardGoneBoard(t, pool, q, "teamtailor", "gone-board", "")
	seedBoardGoneSibling(t, q, "teamtailor")
	deadID := upsertBoardGoneJob(t, q, "gone-board", "1")
	siblingID := upsertBoardGoneJob(t, q, "healthy-sibling", "2")

	logs := captureLog(t)
	report, err := closeBoardGoneBoards(ctx, q, 7, 100, false)
	if err != nil {
		t.Fatalf("board-gone dry run: %v", err)
	}
	if report.boardsProcessed != 1 || report.jobsAffected != 1 || report.boardsSkippedAmbiguous != 0 {
		t.Fatalf("dry-run report = %+v, want one eligible board and one job", report)
	}
	if !strings.Contains(logs.String(), "--apply-board-gone to close") {
		t.Fatalf("dry-run log must explain the separate apply flag: %s", logs.String())
	}
	dead, err := q.GetJob(ctx, deadID)
	if err != nil {
		t.Fatalf("get dry-run candidate: %v", err)
	}
	if dead.ClosedAt.Valid {
		t.Fatal("board-gone dry run closed a job")
	}

	logs = captureLog(t)
	report, err = closeBoardGoneBoards(ctx, q, 7, 100, true)
	if err != nil {
		t.Fatalf("board-gone apply: %v", err)
	}
	if report.boardsProcessed != 1 || report.jobsAffected != 1 {
		t.Fatalf("apply report = %+v, want one eligible board and one closed job", report)
	}
	if !strings.Contains(logs.String(), "closed 1 job(s) for board-gone board teamtailor/gone-board") {
		t.Fatalf("apply log does not report the close: %s", logs.String())
	}
	dead, err = q.GetJob(ctx, deadID)
	if err != nil {
		t.Fatalf("get applied candidate: %v", err)
	}
	if !dead.ClosedAt.Valid || dead.ClosedReason != "board_gone" {
		t.Fatalf("applied candidate = %+v, want closed_reason board_gone", dead)
	}
	sibling, err := q.GetJob(ctx, siblingID)
	if err != nil {
		t.Fatalf("get sibling: %v", err)
	}
	if sibling.ClosedAt.Valid {
		t.Fatal("apply closed a healthy sibling job")
	}
}

func TestCloseBoardGoneWorkerRefusesAmbiguousBoard(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	ctx := context.Background()
	truncate(t, pool)
	seedBoardGoneBoard(t, pool, q, "teamtailor", "regional-board", "gb")
	seedBoardGoneBoard(t, pool, q, "teamtailor", "regional-board", "us")
	seedBoardGoneSibling(t, q, "teamtailor")
	jobID := upsertBoardGoneJob(t, q, "regional-board", "1")

	logs := captureLog(t)
	report, err := closeBoardGoneBoards(ctx, q, 7, 100, true)
	if err != nil {
		t.Fatalf("board-gone apply on ambiguous board: %v", err)
	}
	if report.boardsSkippedAmbiguous != 2 || report.jobsAffected != 0 {
		t.Fatalf("report = %+v, want both regional health rows refused and no jobs closed", report)
	}
	if !strings.Contains(logs.String(), "teamtailor/regional-board (gb)") {
		t.Fatalf("ambiguous-region log does not identify its region: %s", logs.String())
	}
	job, err := q.GetJob(ctx, jobID)
	if err != nil {
		t.Fatalf("get ambiguous job: %v", err)
	}
	if job.ClosedAt.Valid {
		t.Fatal("region-ambiguous board job was closed")
	}
}

func TestCloseBoardGoneWorkerReportsCapAndQueryFailure(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	ctx := context.Background()
	truncate(t, pool)
	for _, board := range []string{"gone-a", "gone-b", "gone-c"} {
		seedBoardGoneBoard(t, pool, q, "teamtailor", board, "")
	}
	seedBoardGoneSibling(t, q, "teamtailor")
	logs := captureLog(t)
	report, err := closeBoardGoneBoards(ctx, q, 7, 2, false)
	if err != nil {
		t.Fatalf("capped board-gone dry run: %v", err)
	}
	if report.boardsProcessed != 2 {
		t.Fatalf("processed %d boards, want cap of 2", report.boardsProcessed)
	}
	if !strings.Contains(logs.String(), "1 more board-gone board(s) exist") {
		t.Fatalf("cap truncation was not reported: %s", logs.String())
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if report, err := closeBoardGoneBoards(canceled, q, 7, 2, false); err == nil {
		t.Fatalf("canceled query returned report %+v without an error", report)
	}
}
