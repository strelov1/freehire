//go:build integration

// Integration tests for the chronic-board safety-net close (openspec change
// close-chronically-unreachable-boards, issue #2017): once board_health.ListChronicBoards has
// proven a board (or a boardless provider) unreachable past the closure window, these two
// queries retire its open jobs with a close reason distinct from the ordinary unseen sweep's.
// They also re-validate board_health's CURRENT state within their own statement (PR #2641
// review), so a board that recovers between an earlier ListChronicBoards read and this call is
// never closed on stale information.
// Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/externalid"
)

// chronicWindow is the age_window this file's tests pass to the close/count queries — the same
// 60-day default cmd/close-chronic-boards uses.
var chronicWindow = pgtype.Interval{Days: 60, Valid: true}

// seedChronicBoardHealth inserts a board_health row that satisfies the chronic predicate these
// queries re-validate: last_success_at 61 days ago, one day past the 60-day chronicWindow used
// throughout this file.
func seedChronicBoardHealth(t *testing.T, ctx context.Context, pool *pgxpool.Pool, q *Queries, provider, board string) {
	t.Helper()
	if _, err := q.RecordBoardFailure(ctx, RecordBoardFailureParams{
		Provider: provider, Board: board, Region: "",
		LastError: pgtype.Text{String: "boom", Valid: true},
	}); err != nil {
		t.Fatalf("seed board_health %s/%s: %v", provider, board, err)
	}
	past := time.Now().Add(-61 * 24 * time.Hour)
	if _, err := pool.Exec(ctx,
		"UPDATE board_health SET last_success_at = $3 WHERE provider = $1 AND board = $2",
		provider, board, past); err != nil {
		t.Fatalf("backdate board_health %s/%s: %v", provider, board, err)
	}
}

func closedReason(t *testing.T, ctx context.Context, q *Queries, id int64) string {
	t.Helper()
	job, err := q.GetJob(ctx, id)
	if err != nil {
		t.Fatalf("get job %d: %v", id, err)
	}
	return job.ClosedReason
}

// TestCloseChronicBoardJobsClosesOnlyThatBoard pins the board-scoped safety net: it closes a
// chronic board's own open jobs, leaves a sibling board of the same provider untouched, and
// records the distinct 'board_unreachable' close reason with an atomic search-delete enqueue.
func TestCloseChronicBoardJobsClosesOnlyThatBoard(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	seedChronicBoardHealth(t, ctx, pool, q, "greenhouse", "dead-board")
	chronic, err := ingestUpsert(ctx, q, ingestParams(externalid.Namespace("dead-board", "1"), "Chronic"))
	if err != nil {
		t.Fatalf("upsert chronic: %v", err)
	}
	sibling, err := ingestUpsert(ctx, q, ingestParams(externalid.Namespace("healthy-board", "1"), "Sibling"))
	if err != nil {
		t.Fatalf("upsert sibling: %v", err)
	}

	closed, err := q.CloseChronicBoardJobs(ctx, CloseChronicBoardJobsParams{
		Source:       "greenhouse",
		BoardPattern: externalid.BoardPattern("dead-board"),
		Board:        "dead-board",
		AgeWindow:    chronicWindow,
	})
	if err != nil {
		t.Fatalf("close chronic board jobs: %v", err)
	}
	if closed != 1 {
		t.Fatalf("closed %d jobs, want 1", closed)
	}

	chronicAfter, err := q.GetJob(ctx, chronic.ID)
	if err != nil {
		t.Fatalf("get chronic: %v", err)
	}
	if !chronicAfter.ClosedAt.Valid {
		t.Fatal("chronic board's job must be closed")
	}
	if got := closedReason(t, ctx, q, chronic.ID); got != "board_unreachable" {
		t.Fatalf("closed_reason = %q, want %q", got, "board_unreachable")
	}

	siblingAfter, err := q.GetJob(ctx, sibling.ID)
	if err != nil {
		t.Fatalf("get sibling: %v", err)
	}
	if siblingAfter.ClosedAt.Valid {
		t.Fatal("a sibling board of the same provider must stay open")
	}

	var queued int
	if err := pool.QueryRow(ctx,
		"SELECT count(*) FROM search_delete_outbox WHERE job_id = $1", chronic.ID).Scan(&queued); err != nil {
		t.Fatalf("read search_delete_outbox: %v", err)
	}
	if queued != 1 {
		t.Fatalf("search_delete_outbox has %d rows for the closed job, want 1", queued)
	}
}

// TestCloseChronicBoardJobsIsANoOpOnAlreadyClosedRows pins idempotency: a job the ordinary
// sweep already closed is not re-touched or re-queued by a later safety-net run.
func TestCloseChronicBoardJobsIsANoOpOnAlreadyClosedRows(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	seedChronicBoardHealth(t, ctx, pool, q, "greenhouse", "dead-board")
	job, err := ingestUpsert(ctx, q, ingestParams(externalid.Namespace("dead-board", "1"), "Already closed"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	ageJob(t, pool, job.ID, 49*time.Hour)
	if _, err := q.CloseUnseenJobsForBoard(ctx, CloseUnseenJobsForBoardParams{
		Source:       "greenhouse",
		Cutoff:       pgTimestamptz(time.Now().Add(-48 * time.Hour)),
		BoardPattern: externalid.BoardPattern("dead-board"),
	}); err != nil {
		t.Fatalf("ordinary sweep close: %v", err)
	}

	closed, err := q.CloseChronicBoardJobs(ctx, CloseChronicBoardJobsParams{
		Source:       "greenhouse",
		BoardPattern: externalid.BoardPattern("dead-board"),
		Board:        "dead-board",
		AgeWindow:    chronicWindow,
	})
	if err != nil {
		t.Fatalf("close chronic board jobs: %v", err)
	}
	if closed != 0 {
		t.Fatalf("closed %d already-closed jobs, want 0", closed)
	}

	if got := closedReason(t, ctx, q, job.ID); got != "unseen" {
		t.Fatalf("closed_reason changed to %q, want the original %q", got, "unseen")
	}
}

// TestCloseChronicProviderJobsClosesTheWholeBoardlessProvider pins the source-scoped path for
// a boardless provider's chronic record: since it has no finer board grain, going chronic
// closes the whole provider's open jobs, and a different provider is unaffected.
func TestCloseChronicProviderJobsClosesTheWholeBoardlessProvider(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	seedChronicBoardHealth(t, ctx, pool, q, "uber", "")

	p1 := ingestParams("uber-job-1", "Boardless 1")
	p1.Source = "uber"
	p1.PublicSlug = "uber-1"
	uberJob1, err := ingestUpsert(ctx, q, p1)
	if err != nil {
		t.Fatalf("upsert uber 1: %v", err)
	}
	p2 := ingestParams("uber-job-2", "Boardless 2")
	p2.Source = "uber"
	p2.PublicSlug = "uber-2"
	uberJob2, err := ingestUpsert(ctx, q, p2)
	if err != nil {
		t.Fatalf("upsert uber 2: %v", err)
	}
	otherProviderJob, err := ingestUpsert(ctx, q, ingestParams("greenhouse-job-1", "Different provider"))
	if err != nil {
		t.Fatalf("upsert other provider: %v", err)
	}

	closed, err := q.CloseChronicProviderJobs(ctx, CloseChronicProviderJobsParams{Source: "uber", AgeWindow: chronicWindow})
	if err != nil {
		t.Fatalf("close chronic provider jobs: %v", err)
	}
	if closed != 2 {
		t.Fatalf("closed %d jobs, want 2 (uber's whole catalogue)", closed)
	}

	for _, id := range []int64{uberJob1.ID, uberJob2.ID} {
		job, err := q.GetJob(ctx, id)
		if err != nil {
			t.Fatalf("get uber job %d: %v", id, err)
		}
		if !job.ClosedAt.Valid {
			t.Fatalf("uber job %d must be closed", id)
		}
		if job.ClosedReason != "board_unreachable" {
			t.Fatalf("uber job %d closed_reason = %q, want %q", id, job.ClosedReason, "board_unreachable")
		}
	}

	otherAfter, err := q.GetJob(ctx, otherProviderJob.ID)
	if err != nil {
		t.Fatalf("get other provider job: %v", err)
	}
	if otherAfter.ClosedAt.Valid {
		t.Fatal("a different provider's job must stay open")
	}
}

// TestCloseChronicBoardJobsDoesNotCloseARecoveredBoard pins the TOCTOU fix (PR #2641 review):
// if the board recovers — a real crawl succeeds, calling RecordBoardSuccess — in the gap
// between an earlier caller reading it as chronic (e.g. via ListChronicBoards) and this call,
// the close must not fire. The query re-validates board_health's CURRENT state rather than
// trusting the caller's now-stale read.
func TestCloseChronicBoardJobsDoesNotCloseARecoveredBoard(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	seedChronicBoardHealth(t, ctx, pool, q, "greenhouse", "dead-board")
	job, err := ingestUpsert(ctx, q, ingestParams(externalid.Namespace("dead-board", "1"), "Recovers mid-run"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// The board recovers AFTER a caller would have read it as chronic but BEFORE the close
	// statement actually runs.
	if err := q.RecordBoardSuccess(ctx, RecordBoardSuccessParams{
		Provider: "greenhouse", Board: "dead-board", Region: "",
		LastIngestedCount: pgtype.Int4{Int32: 3, Valid: true},
	}); err != nil {
		t.Fatalf("record recovery: %v", err)
	}

	closed, err := q.CloseChronicBoardJobs(ctx, CloseChronicBoardJobsParams{
		Source:       "greenhouse",
		BoardPattern: externalid.BoardPattern("dead-board"),
		Board:        "dead-board",
		AgeWindow:    chronicWindow,
	})
	if err != nil {
		t.Fatalf("close chronic board jobs: %v", err)
	}
	if closed != 0 {
		t.Fatalf("closed %d jobs, want 0 — the board recovered before this statement ran", closed)
	}

	after, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if after.ClosedAt.Valid {
		t.Fatal("a job on a board that recovered must stay open")
	}
}

// TestCloseChronicProviderJobsDoesNotCloseARecoveredProvider is the boardless-provider mirror
// of the test above.
func TestCloseChronicProviderJobsDoesNotCloseARecoveredProvider(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	seedChronicBoardHealth(t, ctx, pool, q, "uber", "")
	p := ingestParams("uber-job-1", "Recovers mid-run")
	p.Source = "uber"
	p.PublicSlug = "uber-1"
	job, err := ingestUpsert(ctx, q, p)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	if err := q.RecordBoardSuccess(ctx, RecordBoardSuccessParams{
		Provider: "uber", Board: "", Region: "",
		LastIngestedCount: pgtype.Int4{Int32: 5, Valid: true},
	}); err != nil {
		t.Fatalf("record recovery: %v", err)
	}

	closed, err := q.CloseChronicProviderJobs(ctx, CloseChronicProviderJobsParams{Source: "uber", AgeWindow: chronicWindow})
	if err != nil {
		t.Fatalf("close chronic provider jobs: %v", err)
	}
	if closed != 0 {
		t.Fatalf("closed %d jobs, want 0 — the provider recovered before this statement ran", closed)
	}

	after, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if after.ClosedAt.Valid {
		t.Fatal("a job on a provider that recovered must stay open")
	}
}
