package main

import (
	"cmp"
	"context"
	"maps"
	"math"
	"slices"
	"time"

	"github.com/strelov1/freehire/internal/platform/db"
)

// viewCountChunk is how many documents one index task updates. Every task pays the same
// ~40 s of index post-processing however small it is, so a day's ~1M counts go in ~20 large
// tasks (~1.5 MB of JSON each) rather than hundreds of small ones.
const viewCountChunk = 50_000

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

// pushBudget caps how long one run writes view counts to the index. Agent views move over
// a million jobs into a new step a day, and at the tens of thousands of documents an hour
// Meilisearch manages next to the drain the push outlasted the day, so a run swallowed the
// next timer start. Past the budget the rest keep their old figure, as after a failed push.
const pushBudget = 4 * time.Hour

type countedJob struct{ id, n int64 }

// pushViewCounts writes the current view_count of every job this run moved into a new
// step, most viewed first, so the head of the "Most viewed" sort is right even when the
// deadline cuts the push short. It reads the figure from jobs rather than adding the run's
// deltas, so a rerun after a failed push writes the same values again. It returns how
// many it wrote and how many it left for the deadline.
func pushViewCounts(ctx context.Context, q *db.Queries, idx viewCountSetter, deltas map[int64]int64, deadline time.Time) (int, int, error) {
	var crossed []countedJob
	for chunk := range slices.Chunk(slices.Sorted(maps.Keys(deltas)), viewCountChunk) {
		rows, err := q.GetJobViewCounts(ctx, chunk)
		if err != nil {
			return 0, 0, err
		}
		for _, r := range rows {
			n := int64(r.ViewCount)
			if viewStep(n) != viewStep(n-deltas[r.ID]) {
				crossed = append(crossed, countedJob{r.ID, n})
			}
		}
	}
	return writeViewCounts(ctx, idx, crossed, deadline)
}

// writeViewCounts writes counts in chunks, largest count first, until deadline passes.
func writeViewCounts(ctx context.Context, idx viewCountSetter, counts []countedJob, deadline time.Time) (pushed, skipped int, err error) {
	slices.SortFunc(counts, func(a, b countedJob) int { return cmp.Or(cmp.Compare(b.n, a.n), cmp.Compare(a.id, b.id)) })
	for chunk := range slices.Chunk(counts, viewCountChunk) {
		if !time.Now().Before(deadline) {
			return pushed, len(counts) - pushed, nil
		}
		pending := make(map[int64]int64, len(chunk))
		for _, c := range chunk {
			pending[c.id] = c.n
		}
		if err := idx.SetViewCounts(ctx, pending); err != nil {
			return pushed, 0, err
		}
		pushed += len(chunk)
	}
	return pushed, 0, nil
}
