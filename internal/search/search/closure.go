package search

import (
	"context"
	"fmt"
	"slices"

	"github.com/meilisearch/meilisearch-go"
)

// openJobsFilter also accepts legacy documents that have no visibility markers.
const openJobsFilter = "NOT is_closed = true AND NOT is_duplicate = true"

// CloseJobs marks existing documents closed, keeping their text and vectors. Unlike a
// partial-document upsert it cannot create skeletal documents for jobs never indexed.
// Needs the engine's edit-documents-by-function feature enabled.
func (c *Client) CloseJobs(ctx context.Context, ids []int64) error {
	return c.markJobs(ctx, ids, "is_closed")
}

// MarkDuplicateJobs hides existing copies without replacing their text or vectors.
func (c *Client) MarkDuplicateJobs(ctx context.Context, ids []int64) error {
	return c.markJobs(ctx, ids, "is_duplicate")
}

// retireDescVectors drops the stale `_vectors.desc` chunk vectors of the retired desc
// embedder. The engine caps a document at 10,000 array elements, so a long description
// made the whole task fail with "Size of array/BLOB too large". The skill vector is
// untouched. The key is overwritten with unit first because `remove` alone trips the
// same guard on its own return value.
const retireDescVectors = `if "_vectors" in doc { doc._vectors.desc = (); doc._vectors.remove("desc"); } `

// markJobs sets marker on existing documents. Each marker has its own idempotency guard
// (a closed duplicate still needs both), so the public-search predicate cannot gate it.
func (c *Client) markJobs(ctx context.Context, ids []int64, marker string) error {
	if len(ids) == 0 {
		return nil
	}
	task, err := c.facet.UpdateDocumentsByFunction(ctx, &meilisearch.UpdateDocumentByFunctionRequest{
		Filter:   "(" + In("id", ids) + ") AND NOT " + marker + " = true",
		Function: retireDescVectors + "doc." + marker + " = true;",
	})
	if err != nil {
		return fmt.Errorf("search: update %s status: %w", marker, err)
	}
	return c.awaitTask(ctx, c.facet, task.TaskUID)
}

// withOpenJobs adds the open-jobs predicate to filter; see AndFragment.
func withOpenJobs(filter any) any { return AndFragment(filter, openJobsFilter) }

// AndFragment ANDs a filter fragment onto filter without changing the caller's AND/OR groups
// or mutating a filter shared by concurrent requests.
func AndFragment(filter any, frag string) any {
	switch f := filter.(type) {
	case nil:
		return frag
	case string:
		if f == "" {
			return frag
		}
		return "(" + f + ") AND " + frag
	case [][]string:
		return append(slices.Clone(f), []string{frag})
	case []string:
		return append(slices.Clone(f), frag)
	case []any:
		return append(slices.Clone(f), frag)
	default:
		// Keep an unsupported shape so the engine rejects it instead of dropping constraints.
		return []any{filter, frag}
	}
}
