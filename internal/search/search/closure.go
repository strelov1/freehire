package search

import (
	"context"
	"fmt"
	"slices"

	"github.com/meilisearch/meilisearch-go"
)

// openJobsFilter also accepts legacy documents that have no closure marker.
const openJobsFilter = "NOT is_closed = true"

// CloseJobs updates only existing documents, preserving their text and vectors.
// The edit-documents-by-function engine feature must be enabled before rollout.
// Unlike partial-document upserts this cannot create skeletal closed documents
// for the many database jobs that never entered the searchable catalogue.
func (c *Client) CloseJobs(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	task, err := c.facet.UpdateDocumentsByFunctionWithContext(ctx, &meilisearch.UpdateDocumentByFunctionRequest{
		Filter:   "(" + In("id", ids) + ") AND " + openJobsFilter,
		Function: "doc.is_closed = true;",
	})
	if err != nil {
		return fmt.Errorf("search: update closure status: %w", err)
	}
	return c.awaitTask(ctx, c.facet, task.TaskUID)
}

// withOpenJobs narrows every job-search surface without changing the caller's
// AND/OR groups or mutating a filter shared by concurrent facet requests.
func withOpenJobs(filter any) any {
	switch f := filter.(type) {
	case nil:
		return openJobsFilter
	case string:
		if f == "" {
			return openJobsFilter
		}
		return "(" + f + ") AND " + openJobsFilter
	case [][]string:
		return append(slices.Clone(f), []string{openJobsFilter})
	case []string:
		return append(slices.Clone(f), openJobsFilter)
	case []any:
		return append(slices.Clone(f), openJobsFilter)
	default:
		// Preserve an unsupported caller shape so the engine rejects it rather than
		// silently dropping the caller's constraints.
		return []any{filter, openJobsFilter}
	}
}
