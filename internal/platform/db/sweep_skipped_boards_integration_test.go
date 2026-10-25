//go:build integration

package db

import (
	"context"
	"testing"
	"time"
)

// A board the run skipped (quiet pause or failure cooldown) was not looked at, so its rows' age
// since last seen says nothing about them: the company- and source-scoped closes must leave them
// open even when another board of the same company was crawled. The two boards here share one
// company slug; only "acme-quiet" is skipped.
func TestUnseenSweepsLeaveSkippedBoardsAlone(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	cutoff := pgTimestamptz(time.Now().Add(-48 * time.Hour))

	seed := func(externalID string) int64 {
		t.Helper()
		row, err := ingestUpsert(ctx, q, ingestParams(externalID, "Engineer "+externalID))
		if err != nil {
			t.Fatalf("upsert %s: %v", externalID, err)
		}
		ageJob(t, pool, row.ID, 60*time.Hour)
		return row.ID
	}
	open := func(id int64) bool {
		t.Helper()
		j, err := q.GetJob(ctx, id)
		if err != nil {
			t.Fatalf("get %d: %v", id, err)
		}
		return !j.ClosedAt.Valid
	}
	skipped := []string{"acme-quiet:%"}

	// Company scope.
	truncate(t, pool)
	crawled, quiet := seed("acme:1"), seed("acme-quiet:1")
	if _, err := q.CloseUnseenJobs(ctx, CloseUnseenJobsParams{Source: "greenhouse", Cutoff: cutoff, CompanySlugs: []string{"acme"}, SkippedBoardPatterns: skipped}); err != nil {
		t.Fatalf("CloseUnseenJobs: %v", err)
	}
	if open(crawled) || !open(quiet) {
		t.Errorf("company scope: crawled board open=%v (want false), skipped board open=%v (want true)", open(crawled), open(quiet))
	}
	ids, err := q.UnseenJobIDs(ctx, UnseenJobIDsParams{Source: "greenhouse", Cutoff: cutoff, CompanySlugs: []string{"acme"}, SkippedBoardPatterns: skipped})
	if err != nil || len(ids) != 0 {
		t.Errorf("UnseenJobIDs = (%v, %v), want none: the skipped board's row is not a candidate", ids, err)
	}

	// Source scope.
	truncate(t, pool)
	crawled, quiet = seed("acme:2"), seed("acme-quiet:2")
	if ids, err := q.UnseenJobIDsBySource(ctx, UnseenJobIDsBySourceParams{Source: "greenhouse", Cutoff: cutoff, SkippedBoardPatterns: skipped}); err != nil || len(ids) != 1 || ids[0] != crawled {
		t.Errorf("UnseenJobIDsBySource = (%v, %v), want [%d]", ids, err, crawled)
	}
	if _, err := q.CloseUnseenJobsBySource(ctx, CloseUnseenJobsBySourceParams{Source: "greenhouse", Cutoff: cutoff, SkippedBoardPatterns: skipped}); err != nil {
		t.Fatalf("CloseUnseenJobsBySource: %v", err)
	}
	if open(crawled) || !open(quiet) {
		t.Errorf("source scope: crawled board open=%v (want false), skipped board open=%v (want true)", open(crawled), open(quiet))
	}

	// No skipped boards at all must still close. A NULL array here would make
	// NOT (LIKE ANY(NULL)) NULL and silently close nothing, run after run.
	truncate(t, pool)
	seed("acme:3")
	seed("acme-quiet:3")
	if n, err := q.CloseUnseenJobs(ctx, CloseUnseenJobsParams{Source: "greenhouse", Cutoff: cutoff, CompanySlugs: []string{"acme"}}); err != nil || n != 2 {
		t.Errorf("CloseUnseenJobs with no skipped boards = (%d, %v), want (2, nil)", n, err)
	}
	if n, err := q.CloseUnseenJobsBySource(ctx, CloseUnseenJobsBySourceParams{Source: "greenhouse", Cutoff: cutoff}); err != nil || n != 0 {
		t.Errorf("CloseUnseenJobsBySource after the company close = (%d, %v), want (0, nil)", n, err)
	}
}
