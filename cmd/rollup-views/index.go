package main

import (
	"context"
	"maps"
	"math"
	"slices"

	"github.com/strelov1/freehire/internal/platform/db"
)

// viewCountChunk is how many documents one index task updates. Meilisearch copies the
// task context (the whole counts map) for every document it edits, so a task's cost grows
// with the square of its size; 2k keeps the fixed per-task cost small without that.
const viewCountChunk = 2_000

// minIndexedViews and viewStepGrowth set the steps of viewStep: nothing below 5 views,
// then a new step every 25% of growth (5, 7, 8, 10, 13, 16, 20, 24, 30, ...).
const (
	minIndexedViews = 5
	viewStepGrowth  = 1.25
)

type viewCountSetter interface {
	SetViewCounts(ctx context.Context, counts map[int64]int64) error
}

// viewStep buckets a view count for the "Most viewed" sort. A count is written to the
// index only when its step changes: about 2.5M jobs gain a view every day, mostly from
// agents and crawlers, and rewriting each one kept Meilisearch busy for hours and the run
// going for a whole day (2026-11-17). Jobs below the floor never reach the top of that
// sort, and the drain writes the exact count whenever it rewrites a document anyway.
func viewStep(n int64) int {
	if n < minIndexedViews {
		return 0
	}
	return 1 + int(math.Log(float64(n)/minIndexedViews)/math.Log(viewStepGrowth))
}

// pushViewCounts writes the current view_count of every job this run moved into a new
// step. It reads the figure from jobs rather than adding the run's deltas, so a rerun
// after a failed push writes the same values again. It returns how many it wrote.
func pushViewCounts(ctx context.Context, q *db.Queries, idx viewCountSetter, deltas map[int64]int64) (int, error) {
	pushed := 0
	pending := make(map[int64]int64, viewCountChunk)
	flush := func() error {
		if len(pending) == 0 {
			return nil
		}
		if err := idx.SetViewCounts(ctx, pending); err != nil {
			return err
		}
		pushed += len(pending)
		pending = make(map[int64]int64, viewCountChunk)
		return nil
	}
	for chunk := range slices.Chunk(slices.Sorted(maps.Keys(deltas)), viewCountChunk) {
		rows, err := q.GetJobViewCounts(ctx, chunk)
		if err != nil {
			return pushed, err
		}
		for _, r := range rows {
			n := int64(r.ViewCount)
			if viewStep(n) == viewStep(n-deltas[r.ID]) {
				continue
			}
			pending[r.ID] = n
			if len(pending) == viewCountChunk {
				if err := flush(); err != nil {
					return pushed, err
				}
			}
		}
	}
	return pushed, flush()
}
