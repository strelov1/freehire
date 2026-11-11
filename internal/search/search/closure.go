package search

import (
	"context"
	"fmt"
	"slices"

	"github.com/meilisearch/meilisearch-go"
)

// openJobsFilter also accepts legacy documents that have no visibility markers.
const openJobsFilter = "NOT is_closed = true AND NOT is_duplicate = true"

// CloseJobs updates only existing documents, preserving their text and vectors.
// The edit-documents-by-function engine feature must be enabled before rollout.
// Unlike partial-document upserts this cannot create skeletal closed documents
// for the many database jobs that never entered the searchable catalogue.
func (c *Client) CloseJobs(ctx context.Context, ids []int64) error {
	return c.markJobs(ctx, ids, "is_closed")
}

// MarkDuplicateJobs hides existing copies without replacing their text or vectors.
func (c *Client) MarkDuplicateJobs(ctx context.Context, ids []int64) error {
	return c.markJobs(ctx, ids, "is_duplicate")
}

// retireDescVectors drops the `_vectors.desc` chunk vectors that the retired desc embedder
// (#4417) left inside documents nobody rewrote since. The engine caps one document's
// total array elements at 10,000 and counts them, so a posting with 14+ chunks of 768
// floats (a long description) made the whole task fail with "Size of array/BLOB too
// large" — and the wave holding it failed again on every lease-expiry retry. The live
// skill vector is untouched. Order matters: `remove` alone trips the same guard on its
// own return value, so the key is first overwritten with unit to shrink it.
const retireDescVectors = `if "_vectors" in doc { doc._vectors.desc = (); doc._vectors.remove("desc"); } `

// Each marker has its own idempotency guard: a closed duplicate still needs both
// markers, so the public-search predicate cannot gate these writes.
func (c *Client) markJobs(ctx context.Context, ids []int64, marker string) error {
	if len(ids) == 0 {
		return nil
	}
	task, err := c.facet.UpdateDocumentsByFunctionWithContext(ctx, &meilisearch.UpdateDocumentByFunctionRequest{
		Filter:   "(" + In("id", ids) + ") AND NOT " + marker + " = true",
		Function: retireDescVectors + "doc." + marker + " = true;",
	})
	if err != nil {
		return fmt.Errorf("search: update %s status: %w", marker, err)
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
