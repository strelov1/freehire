package search

import (
	"context"
	"fmt"
	"time"

	"github.com/meilisearch/meilisearch-go"
)

// SitemapDocument is the slim projection a sitemap URL needs — the slug the page is
// addressed by and a lastmod. Both indexes project onto it; the slug attribute they
// project FROM differs (jobs key their public URL on public_slug, companies on the
// slug that is also their primary key), which is why the response is decoded into a
// map and the slug read by its per-index key rather than by a struct tag, which
// could only ever name one of the two.
type SitemapDocument struct {
	Slug      string
	UpdatedAt time.Time
}

// slugField is the attribute each index's public URL is built from.
const (
	jobSlugField     = "public_slug"
	companySlugField = "slug"
	lastmodField     = "updated_at"
)

// sitemapPage is the shared body of the two sitemap readers: one offset-addressed
// page of an index, projected down to a slug and a lastmod, plus the index's total
// document count.
//
// The sitemap pages the SEARCH INDEXES rather than their Postgres tables because the
// indexes already hold exactly the sets worth handing a crawler — for jobs: open,
// non-duplicate, non-private, categorized (cmd/reindex's splitJobs); for companies:
// those with an open role (cmd/reindex-companies). The jobs table's equivalent scope
// is ~2.7x larger and includes postings the site's own search cannot find.
//
// The engine addresses an offset directly instead of walking to it — measured on
// prod (2026-08-16, 1.26M job documents): offset 0 and offset 1.2M both answer in
// under 0.25s, and a full 25k page in ~2.5s. That is what replaced a Postgres
// `row_number()` walk which had grown to 64s over 3.4M rows and was timing out the
// SSR render of /sitemap.xml.
//
// Offset paging is not a stable cursor: documents added or removed between two page
// requests shift later pages. That is acceptable here and nowhere else — a sitemap
// is a crawl hint, not a contract, and both indexes are rebuilt on a timer anyway. A
// page past the end is an empty page, never an error.
func (c *Client) sitemapPage(ctx context.Context, idx meilisearch.IndexManager, slugField string, offset, limit int) ([]SitemapDocument, int64, error) {
	var resp meilisearch.DocumentsResult
	q := &meilisearch.DocumentsQuery{
		Offset: int64(offset),
		Limit:  int64(limit),
		Fields: []string{slugField, lastmodField},
	}
	if err := idx.GetDocumentsWithContext(ctx, q, &resp); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, 0, fmt.Errorf("search: sitemap page: %w", ctxErr)
		}
		return nil, 0, fmt.Errorf("search: sitemap page: %w", err)
	}
	// Decoded through a per-call shape because the slug attribute is named
	// differently in each index; a shared struct tag could only ever match one.
	var raw []map[string]any
	if err := resp.Results.DecodeInto(&raw); err != nil {
		return nil, 0, fmt.Errorf("search: sitemap page: decode: %w", err)
	}
	docs := make([]SitemapDocument, 0, len(raw))
	for _, r := range raw {
		slug, _ := r[slugField].(string)
		if slug == "" {
			continue
		}
		doc := SitemapDocument{Slug: slug}
		// A document indexed before updated_at joined the shape has no lastmod. The
		// tag is optional in the protocol, so such a URL ships without one rather
		// than dropping out of the sitemap — which is what makes adding the
		// attribute safe to deploy ahead of the reindex that backfills it.
		if s, ok := r[lastmodField].(string); ok {
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				doc.UpdatedAt = t
			}
		}
		docs = append(docs, doc)
	}
	return docs, resp.Total, nil
}

// ListSitemapPage returns one offset-addressed page of the live jobs index along
// with the index's total document count. See sitemapPage.
func (c *Client) ListSitemapPage(ctx context.Context, offset, limit int) ([]SitemapDocument, int64, error) {
	return c.sitemapPage(ctx, c.facet, jobSlugField, offset, limit)
}

// ListCompanySitemapPage is ListSitemapPage over the companies index.
func (c *Client) ListCompanySitemapPage(ctx context.Context, offset, limit int) ([]SitemapDocument, int64, error) {
	return c.sitemapPage(ctx, c.manager.Index(companyIndexUID), companySlugField, offset, limit)
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

// CountCompanySitemapDocuments is CountSitemapDocuments over the companies index.
func (c *Client) CountCompanySitemapDocuments(ctx context.Context) (int64, error) {
	_, total, err := c.ListCompanySitemapPage(ctx, 0, 1)
	return total, err
}
