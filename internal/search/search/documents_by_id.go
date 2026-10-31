package search

import (
	"context"
	"fmt"
	"strconv"

	"github.com/meilisearch/meilisearch-go"

	"github.com/strelov1/freehire/internal/job/jobview"
)

// JobViewsByID reads the served shape of the given jobs straight from the live jobs
// index, keyed by job id. A job with no document is simply absent from the map — the
// caller decides whether that is a gap. It exists for cmd/serve-probe, which compares
// what a posting stores with what the index serves for it.
func (c *Client) JobViewsByID(ctx context.Context, ids []int64) (map[int64]jobview.Job, error) {
	if len(ids) == 0 {
		return map[int64]jobview.Job{}, nil
	}
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = strconv.FormatInt(id, 10)
	}
	var resp meilisearch.DocumentsResult
	q := &meilisearch.DocumentsQuery{Ids: keys, Limit: int64(len(ids))}
	if err := c.facet.GetDocumentsWithContext(ctx, q, &resp); err != nil {
		return nil, fmt.Errorf("search: documents by id: %w", err)
	}
	var docs []JobDocument
	if err := resp.Results.DecodeInto(&docs); err != nil {
		return nil, fmt.Errorf("search: documents by id: decode: %w", err)
	}
	out := make(map[int64]jobview.Job, len(docs))
	for _, d := range docs {
		out[d.ID] = d.Job
	}
	return out, nil
}
