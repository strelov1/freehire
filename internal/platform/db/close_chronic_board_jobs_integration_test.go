//go:build integration

// Integration tests for the chronic-board safety-net close (openspec change
// close-chronically-unreachable-boards, issue #2017): once board_health.ListChronicBoards has
// proven a board (or a boardless provider) unreachable past the closure window, these two
// queries retire its open jobs with a close reason distinct from the ordinary unseen sweep's.
// Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/platform/externalid"
)

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

	closed, err := q.CloseChronicProviderJobs(ctx, "uber")
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
