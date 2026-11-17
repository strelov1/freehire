package search

import (
	"context"
	"fmt"
	"time"

	"github.com/meilisearch/meilisearch-go"
)

// SitemapDocument is the slim projection a sitemap URL needs: a slug and a lastmod. The slug
// attribute differs per index (public_slug for jobs, slug for companies), so responses are
// decoded into a map and the slug read by its per-index key.
type SitemapDocument struct {
	Slug      string
	UpdatedAt time.Time
}

// Attributes each index's sitemap projection reads.
const (
	jobSlugField     = "public_slug"
	companySlugField = "slug"
	lastmodField     = "updated_at"
)

// jobSitemapFilter narrows the job sitemap to the postings this site vouches for: the whole
// findable index (open, canonical, public, with a body, per cmd/reindex's splitJobs) minus the
// likely-evergreen reality class. Technical, non-technical and uncategorized ("other", #4412)
// postings alike are included.
//
// is_tech still decides spend (enrichment, embeddings, digest) and Indexing API ordering
// (internal/job/searchping), not what the sitemap asserts.
//
// `stale` is deliberately NOT excluded: it is the classifier's "everything else" (ordinary
// postings over 14 days old, see internal/job/jobreality), and dropping it would churn the
// sitemap every two weeks. likely-evergreen is the one accusing class, and /features/ghost-jobs
// publishes it on the pages the sitemap would otherwise invite crawlers to.
//
// The attribute is already filterable (facetSettings), so no settings patch or reindex is needed.
const jobSitemapFilter = `reality.class != "likely-evergreen" AND ` + openJobsFilter

// sitemapPage is the shared body of the two sitemap readers: one offset-addressed page of an
// index projected to a slug and lastmod, plus the index's total document count.
//
// It pages the search indexes, not Postgres, because they already hold exactly the sets worth
// giving a crawler: jobs that are open, non-duplicate, non-private and categorized
// (splitJobs); companies with a role passing that same test (cmd/reindex-companies, gated on
// companies.job_count). "Same test" is load-bearing: RefreshCompanyFacets applies splitJobs'
// extra predicates, so if either side gains one, both must move, or a company URL renders
// "0 open jobs". The engine addresses an offset directly, unlike the Postgres row_number()
// walk this replaced, which timed out the SSR render.
//
// Offset paging is not a stable cursor: additions or removals between requests shift later
// pages. Acceptable for a crawl hint on indexes rebuilt on a timer. A page past the end is
// empty, never an error.
//
// `filter` is a Meilisearch expression, or empty for the whole index. Assign it only when
// non-empty: DocumentsQuery.Filter is an interface{} with omitempty, which drops only a nil
// interface, so an assigned "" would marshal to `"filter":""`.
func (c *Client) sitemapPage(ctx context.Context, idx meilisearch.IndexManager, slugField, filter string, offset, limit int) ([]SitemapDocument, int64, error) {
	var resp meilisearch.DocumentsResult
	q := &meilisearch.DocumentsQuery{
		Offset: int64(offset),
		Limit:  int64(limit),
		Fields: []string{slugField, lastmodField},
	}
	if filter != "" {
		q.Filter = filter
	}
	if err := idx.GetDocumentsWithContext(ctx, q, &resp); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, 0, fmt.Errorf("search: sitemap page: %w", ctxErr)
		}
		return nil, 0, fmt.Errorf("search: sitemap page: %w", err)
	}
	// Decoded through a map because the slug attribute is named per index.
	var raw []map[string]any
	if err := resp.Results.DecodeInto(&raw); err != nil {
		return nil, 0, fmt.Errorf("search: sitemap page: decode: %w", err)
	}
	return sitemapDocs(raw, slugField), resp.Total, nil
}

// sitemapDocs projects decoded index rows onto the sitemap's slim shape. Both readers share it:
// they differ in how they ask the engine, not in what a row means.
func sitemapDocs(raw []map[string]any, slugField string) []SitemapDocument {
	docs := make([]SitemapDocument, 0, len(raw))
	for _, r := range raw {
		slug, _ := r[slugField].(string)
		if slug == "" {
			continue
		}
		doc := SitemapDocument{Slug: slug}
		// Documents indexed before updated_at joined the shape have no lastmod; the tag is
		// optional, so the URL ships without it, which keeps deploying ahead of the reindex safe.
		if s, ok := r[lastmodField].(string); ok {
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				doc.UpdatedAt = t
			}
		}
		docs = append(docs, doc)
	}
	return docs
}

// ListSitemapPage returns one offset-addressed page of the live jobs index narrowed to
// jobSitemapFilter, plus the total matching documents.
func (c *Client) ListSitemapPage(ctx context.Context, offset, limit int) ([]SitemapDocument, int64, error) {
	return c.sitemapPage(ctx, c.facet, jobSlugField, jobSitemapFilter, offset, limit)
}

// freshSitemapSort orders the fresh job sub-sitemap by `created_at`, not `posted_at`: the
// question is "what is new HERE", and a posting an employer published months ago but this
// catalogue first saw an hour ago is new here. The attribute is already sortable
// (facetSettings), so no settings patch or reindex is needed.
const freshSitemapSort = "created_at:desc"

// freshSitemapFilter narrows the fresh window to technical postings, unlike the paged files.
// The window is a constant 40k newest-first URLs; with non-IT categories included it shrinks
// to hours of intake, and Meilisearch returned the wider sorted window out of order part-way
// through. The fresh file is the crawl-budget signal, so it stays on the postings this board
// exists for; the rest remain reachable through the paged files.
const freshSitemapFilter = jobSitemapFilter + ` AND is_tech = "tech"`

// ListFreshSitemapPage returns one offset-addressed page of the live jobs index narrowed to
// freshSitemapFilter, newest first.
//
// It returns no total: ListSitemapPage's total exists because the sitemap index tiles the paged
// files by it, while the fresh window is tiled by a constant (and a search total would only be
// an estimate).
//
// It searches rather than paging documents because /documents returns storage order and accepts
// no sort; sorting exists only on /search. So one reader buys full coverage at arbitrary order,
// the other order at a bounded window.
//
// The caller bounds the window (the handler's freshSitemapMaxOffset): sorted pages get slower
// with offset, and the bound is about crawl budget and the SSR route's 10s fetch timeout.
func (c *Client) ListFreshSitemapPage(ctx context.Context, offset, limit int) ([]SitemapDocument, error) {
	resp, err := c.facet.SearchWithContext(ctx, "", &meilisearch.SearchRequest{
		Offset:               int64(offset),
		Limit:                int64(limit),
		Filter:               freshSitemapFilter,
		Sort:                 []string{freshSitemapSort},
		AttributesToRetrieve: []string{jobSlugField, lastmodField},
	})
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("search: fresh sitemap page: %w", ctxErr)
		}
		return nil, fmt.Errorf("search: fresh sitemap page: %w", err)
	}
	// Decoded through a map for the same reason as sitemapPage.
	var raw []map[string]any
	if err := resp.Hits.DecodeInto(&raw); err != nil {
		return nil, fmt.Errorf("search: fresh sitemap page: decode: %w", err)
	}
	return sitemapDocs(raw, jobSlugField), nil
}

// ListCompanySitemapPage is ListSitemapPage over the companies index, unfiltered: membership is
// decided upstream by companies.job_count, and jobSitemapFilter names job-only attributes.
func (c *Client) ListCompanySitemapPage(ctx context.Context, offset, limit int) ([]SitemapDocument, int64, error) {
	return c.sitemapPage(ctx, c.manager.Index(companyIndexUID), companySlugField, "", offset, limit)
}

// CountSitemapDocuments returns how many documents match jobSitemapFilter, which is what the
// sitemap index divides into chunks. The total rides on every documents response, so it asks
// for the shortest page. Not zero: Limit is omitempty, so 0 is dropped and Meilisearch applies
// its default of 20.
//
// It delegates to ListSitemapPage so the count and the chunks share one filter and cannot
// disagree.
func (c *Client) CountSitemapDocuments(ctx context.Context) (int64, error) {
	_, total, err := c.ListSitemapPage(ctx, 0, 1)
	return total, err
}

// CountCompanySitemapDocuments is CountSitemapDocuments over the companies index.
func (c *Client) CountCompanySitemapDocuments(ctx context.Context) (int64, error) {
	_, total, err := c.ListCompanySitemapPage(ctx, 0, 1)
	return total, err
}
