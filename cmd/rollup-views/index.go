package main

import (
	"context"
	"maps"
	"slices"

	"github.com/strelov1/freehire/internal/platform/db"
)

// viewCountChunk is how many documents one index task updates. Meilisearch copies the
// task context (the whole counts map) for every document it edits, so a task's cost grows
// with the square of its size: locally 20k documents took 6.4s in chunks of 2k and 15.7s
// in chunks of 10k. On prod an edit task costs ~5s fixed plus ~2ms a document
// (2026-11-14), so 2k keeps the fixed part small without the quadratic part.
const viewCountChunk = 2_000

type viewCountSetter interface {
	SetViewCounts(ctx context.Context, counts map[int64]int64) error
}

// pushViewCounts writes the current view_count of every job this run touched into the
// search index. It reads the figure from jobs rather than adding the run's deltas, so a
// rerun after a failed push writes the same values again.
func pushViewCounts(ctx context.Context, q *db.Queries, idx viewCountSetter, touched map[int64]struct{}) (int, error) {
	ids := slices.Sorted(maps.Keys(touched))
	pushed := 0
	for chunk := range slices.Chunk(ids, viewCountChunk) {
		rows, err := q.GetJobViewCounts(ctx, chunk)
		if err != nil {
			return pushed, err
		}
		counts := make(map[int64]int64, len(rows))
		for _, r := range rows {
			counts[r.ID] = int64(r.ViewCount)
		}
		if err := idx.SetViewCounts(ctx, counts); err != nil {
			return pushed, err
		}
		pushed += len(counts)
	}
	return pushed, nil
}
