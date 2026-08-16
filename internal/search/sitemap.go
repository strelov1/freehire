package search

import (
	"context"
	"fmt"
	"time"

	"github.com/meilisearch/meilisearch-go"
)

// SitemapDocument is the slim projection a sitemap URL needs — the public slug and
// a lastmod. It is a projection of the indexed JobDocument (the field names are the
// document's own), requested through the `fields` parameter so a 25k-entry page
// carries two attributes per document instead of the ~3.7 KB whole.
type SitemapDocument struct {
	Slug      string    `json:"public_slug"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ListSitemapPage returns one offset-addressed page of the live jobs index along
// with the index's total document count.
//
// The sitemap pages the SEARCH INDEX rather than the jobs table because the index
// already holds exactly the set worth handing a crawler: open, non-duplicate,
// non-private, categorized (see cmd/reindex's splitJobs). The table's equivalent
// scope is ~2.7x larger and includes postings the site's own search cannot find.
//
// The engine addresses an offset directly instead of walking to it — measured on
// prod (2026-08-16, 1.26M documents): offset 0 and offset 1.2M both answer in under
// 0.25s, and a full 25k page in ~2.5s. That is what replaced a Postgres
// `row_number()` walk which had grown to 64s over 3.4M rows and was timing out the
// SSR render of /sitemap.xml.
//
// Offset paging is not a stable cursor: documents added or removed between two page
// requests shift later pages. That is acceptable here and nowhere else — a sitemap
// is a crawl hint, not a contract, and the index is rebuilt hourly anyway. A page
// past the end is an empty page, never an error.
func (c *Client) ListSitemapPage(ctx context.Context, offset, limit int) ([]SitemapDocument, int64, error) {
	var resp meilisearch.DocumentsResult
	q := &meilisearch.DocumentsQuery{
		Offset: int64(offset),
		Limit:  int64(limit),
		Fields: []string{"public_slug", "updated_at"},
	}
	if err := c.facet.GetDocumentsWithContext(ctx, q, &resp); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, 0, fmt.Errorf("search: sitemap page: %w", ctxErr)
		}
		return nil, 0, fmt.Errorf("search: sitemap page: %w", err)
	}
	var docs []SitemapDocument
	if err := resp.Results.DecodeInto(&docs); err != nil {
		return nil, 0, fmt.Errorf("search: sitemap page: decode: %w", err)
	}
	return docs, resp.Total, nil
}

// CountSitemapDocuments returns how many documents the live jobs index holds, which
// is what the sitemap index divides into chunks. The total rides on every documents
// response, so this asks for the shortest page the engine will serve rather than a
// count of its own. Not zero: DocumentsQuery.Limit is `omitempty`, so a 0 is dropped
// from the request body and Meilisearch applies its own default of 20.
func (c *Client) CountSitemapDocuments(ctx context.Context) (int64, error) {
	_, total, err := c.ListSitemapPage(ctx, 0, 1)
	return total, err
}
