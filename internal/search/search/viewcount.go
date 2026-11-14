package search

import (
	"context"
	"fmt"
	"strconv"

	"github.com/meilisearch/meilisearch-go"
)

// setViewCount reads the figure from the task context by the document's id, so one task
// carries a whole chunk. retireDescVectors goes first for the reason markJobs gives.
const setViewCount = retireDescVectors +
	`let n = context.counts[doc.id.to_string()]; if n != () { doc.view_count = n; }`

// SetViewCounts writes jobs' current view_count into the documents that are already in
// the index, leaving their text, facets and vectors alone. view_count only backs the
// "Most viewed" sort, so it does not go through search_outbox: rebuilding a whole
// document for one counter flooded the drain (2026-11-14). Like markJobs, an id that is
// not indexed is skipped rather than created as a skeleton document.
func (c *Client) SetViewCounts(ctx context.Context, counts map[int64]int64) error {
	if len(counts) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(counts))
	byID := make(map[string]any, len(counts))
	for id, n := range counts {
		ids = append(ids, id)
		byID[strconv.FormatInt(id, 10)] = n
	}
	task, err := c.facet.UpdateDocumentsByFunctionWithContext(ctx, &meilisearch.UpdateDocumentByFunctionRequest{
		Filter:   In("id", ids),
		Function: setViewCount,
		Context:  map[string]any{"counts": byID},
	})
	if err != nil {
		return fmt.Errorf("search: update view counts: %w", err)
	}
	return c.awaitTask(ctx, c.facet, task.TaskUID)
}
