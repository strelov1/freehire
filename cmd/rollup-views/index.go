package main

import (
	"context"
	"maps"
	"slices"

	"github.com/strelov1/freehire/internal/platform/db"
)

// viewCountChunk is how many documents one index task updates. One task per ~1.7M daily
// figures would hold a huge filter and context in a single request; 10k keeps each well
// under Meilisearch's payload limit.
const viewCountChunk = 10_000

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
