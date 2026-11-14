package main

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

const (
	// jobChangesRetention is how long the change feed (GET /jobs/changes) keeps entries. One
	// day more than the handler's 30-day cursor and `since` bound, so no entry a valid cursor
	// still has to reach is ever deleted.
	jobChangesRetention = 31 * 24 * time.Hour
	// pruneBatch bounds one DELETE, and pruneMaxBatches one run: a backlog drains over runs
	// instead of holding a long transaction against the trigger that writes the log.
	pruneBatch      = 10000
	pruneMaxBatches = 50
)

// pruneJobChanges deletes feed entries past their retention. Failures are logged and
// swallowed, like the other snapshots here: a missed prune costs disk, not correctness.
func pruneJobChanges(ctx context.Context, q *db.Queries) {
	cutoff := pgtype.Timestamptz{Time: time.Now().Add(-jobChangesRetention), Valid: true}
	var total int64
	for range pruneMaxBatches {
		n, err := q.PruneJobChanges(ctx, db.PruneJobChangesParams{Cutoff: cutoff, BatchSize: pruneBatch})
		if err != nil {
			log.Printf("rollup-stats: prune job changes: %v", err)
			return
		}
		total += n
		if n < pruneBatch {
			break
		}
	}
	log.Printf("rollup-stats: pruned %d job change entries", total)
}
