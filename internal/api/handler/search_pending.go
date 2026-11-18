package handler

import (
	"context"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/search/search"
)

// pendingRemovals names the jobs, among those given, whose removal from the index is
// queued but not yet applied. *db.Queries satisfies it.
type pendingRemovals interface {
	ListPendingSearchRemovals(ctx context.Context, jobIds []int64) ([]int64, error)
}

// pendingRemovalsTimeout bounds the lookup dropPendingRemovals makes (a primary-key read over one page of ids).
const pendingRemovalsTimeout = 300 * time.Millisecond

// dropPendingRemovals takes out of a page the postings that have closed but are still in the index: the drain applies closures asynchronously and a facet rebuild pauses it for hours. It asks the queue (search_delete_outbox) about the page's own ids, since the index cannot exclude a six-figure id list.
//
// Unlike hide_applied_companies this drops hits after the query, but a dropped hit is by definition not a result, so the total is lowered by the same count; the backlog on later pages stays over-counted until the drain catches up.
//
// Best-effort: a failed lookup logs and serves the page as the index has it.
func (h *searchHandlers) dropPendingRemovals(ctx context.Context, res search.SearchResult) search.SearchResult {
	if h.removals == nil || len(res.Hits) == 0 {
		return res
	}
	ids := make([]int64, len(res.Hits))
	for i, hit := range res.Hits {
		ids[i] = hit.ID
	}
	// An optional check must not hold the search behind a saturated pool or a stalled query.
	ctx, cancel := context.WithTimeout(ctx, pendingRemovalsTimeout)
	defer cancel()
	gone, err := h.removals.ListPendingSearchRemovals(ctx, ids)
	if err != nil {
		log.Printf("search: read pending removals: %v", err)
		return res
	}
	if len(gone) == 0 {
		return res
	}
	closed := make(map[int64]struct{}, len(gone))
	for _, id := range gone {
		closed[id] = struct{}{}
	}
	kept := make([]search.JobDocument, 0, len(res.Hits))
	for _, hit := range res.Hits {
		if _, ok := closed[hit.ID]; !ok {
			kept = append(kept, hit)
		}
	}
	res.Total = max(res.Total-int64(len(res.Hits)-len(kept)), 0)
	res.Hits = kept
	return res
}
