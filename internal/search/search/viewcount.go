package search

import (
	"context"
	"fmt"
	"strconv"

	"github.com/meilisearch/meilisearch-go"
)

// setViewCount reads the figure from the task context by document id, so one task carries a
// whole chunk. retireDescVectors goes first, as in markJobs.
const setViewCount = retireDescVectors +
	`let n = context.counts[doc.id.to_string()]; if n != () { doc.view_count = n; }`

// SetViewCounts writes view_count into documents already in the index, leaving text, facets
// and vectors alone. It bypasses search_outbox because rebuilding a whole document for one
// counter floods the drain. As in markJobs, an unindexed id is skipped, not created.
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
