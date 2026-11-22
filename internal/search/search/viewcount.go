package search

import (
	"context"
	"fmt"

	"github.com/meilisearch/meilisearch-go"
)

// SetViewCounts writes view_count into documents already in the index, leaving text, facets
// and vectors alone. It bypasses search_outbox because rebuilding a whole document for one
// counter floods the drain. A partial update with skipCreation skips an unindexed id instead
// of creating a skeleton, like markJobs, but without edit-by-function: that engine copied the
// whole counts map into every document it edited, so a task had to stay small, and each task
// paid ~40 s of fixed index post-processing (2026-11-22: 542 tasks of 2k, 5.3 h a day).
func (c *Client) SetViewCounts(ctx context.Context, counts map[int64]int64) error {
	if len(counts) == 0 {
		return nil
	}
	docs := make([]map[string]any, 0, len(counts))
	for id, n := range counts {
		docs = append(docs, map[string]any{primaryKey: id, "view_count": n})
	}
	pk := primaryKey
	task, err := c.facet.UpdateDocumentsWithContext(ctx, docs, &meilisearch.DocumentOptions{PrimaryKey: &pk, SkipCreation: true})
	if err != nil {
		return fmt.Errorf("search: update view counts: %w", err)
	}
	return c.awaitTask(ctx, c.facet, task.TaskUID)
}
