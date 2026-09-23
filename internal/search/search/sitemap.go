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

// jobSitemapFilter narrows the JOB sitemap to what this site claims to be. The index
// itself stays wide — a non-tech posting is a legitimate result behind an explicit
// is_tech facet, and it keeps its page, its canonical and its place in /jobs. A
// sitemap is a different kind of statement: it asserts what is worth crawling, and
// two thirds of the index contradicted the assertion (measured on prod 2026-09-08:
// 1,295,388 of 2,014,462 documents are is_tech=non_tech, against 97,222 in
// software_engineering).
//
// `is_tech = "tech"`, not `!= "non_tech"`: the attribute is written
// tech-or-non_tech-or-ABSENT, so the negation would also admit the ~48k postings
// neither the title dictionary nor enrichment could classify. Since the sitemap
// asserts, silence has to read as "not asserted", not as "tech".
//
// likely-evergreen is excluded and `stale` deliberately is NOT. The reality classes
// are not a liveness verdict (see internal/job/jobreality): `fresh` is "at most 14
// days old", `likely-evergreen` needs two converging ghost signals, and `stale` is
// the classifier's own "everything else" — 461,069 ordinary open tech postings whose
// only fault is being more than a fortnight old. Excluding it would drop them for
// their age and churn the whole sitemap every two weeks, faster than this site's
// crawl budget reaches it. likely-evergreen is the one class that accuses, and
// /features/ghost-jobs publishes that accusation on the very pages the sitemap would
// otherwise invite a crawler to; it is 1,538 documents, so it moves no number and
// closes a contradiction.
//
// Both attributes are already declared in facetSettings().FilterableAttributes, so
// this needs no settings patch and no reindex — the "settings must reach the LIVE
// index before the binary that filters on it" hazard documented there does not apply.
const jobSitemapFilter = `is_tech = "tech" AND reality.class != "likely-evergreen"`

// sitemapPage is the shared body of the two sitemap readers: one offset-addressed
// page of an index, projected down to a slug and a lastmod, plus the index's total
// document count.
//
// The sitemap pages the SEARCH INDEXES rather than their Postgres tables because the
// indexes already hold exactly the sets worth handing a crawler — for jobs: open,
// non-duplicate, non-private, categorized (cmd/reindex's splitJobs); for companies:
// those with a role that passes that same test (cmd/reindex-companies, gated on
// companies.job_count). The jobs table's equivalent scope is ~2.7x larger and includes
// postings the site's own search cannot find.
//
// "That same test" is load-bearing and was once only half true: job_count counted
// open, non-duplicate rows while the company page's list came from search, so an
// employer hiring only for roles no dictionary could categorize shipped a sitemap URL
// rendering "0 open jobs" — 17 of 25 sampled pages on prod. RefreshCompanyFacets now
// applies splitJobs' three extra predicates; if either side gains a fourth, both move.
//
// The engine addresses an offset directly instead of walking to it, which is what
// replaced a Postgres `row_number()` walk that had grown to 64s over 3.4M rows and was
// timing out the SSR render of /sitemap.xml.
//
// Re-measured on prod 2026-09-08 at 2.01M job documents, 10k-document pages, this
// projection: the deepest unfiltered page (offset 1.9M) takes 4.43s and the deepest
// filtered one (offset 660k) 4.07s, while the single-document request
// CountSitemapDocuments issues takes 0.035s. An earlier note here recorded 0.25s at
// offset 1.2M with 1.26M documents; that margin has since narrowed from ~40x to ~2.5x
// against this route's 10s fetch timeout, and it keeps narrowing as the catalogue
// grows. The chunk size is the lever when it runs out.
//
// Offset paging is not a stable cursor: documents added or removed between two page
// requests shift later pages. That is acceptable here and nowhere else — a sitemap
// is a crawl hint, not a contract, and both indexes are rebuilt on a timer anyway. A
// page past the end is an empty page, never an error.
//
// `filter` is a Meilisearch filter expression, or empty for the whole index. It is
// assigned only when non-empty, and that guard is load-bearing rather than tidiness:
// DocumentsQuery.Filter is an `interface{}` with `json:"filter,omitempty"`, and
// omitempty drops an interface only when it is NIL — an interface holding "" marshals
// to `"filter":""`. Assigning unconditionally would therefore send an empty filter
// expression on the company path, which is not the request it sends today.
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
	// Decoded through a per-call shape because the slug attribute is named
	// differently in each index; a shared struct tag could only ever match one.
	var raw []map[string]any
	if err := resp.Results.DecodeInto(&raw); err != nil {
		return nil, 0, fmt.Errorf("search: sitemap page: decode: %w", err)
	}
	return sitemapDocs(raw, slugField), resp.Total, nil
}

// sitemapDocs projects decoded index rows onto the sitemap's slim shape. Shared by
// both readers because they differ only in how they ASK the engine — one pages
// /documents, the other searches with a sort — and not at all in what a row means.
func sitemapDocs(raw []map[string]any, slugField string) []SitemapDocument {
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
	return docs
}

// ListSitemapPage returns one offset-addressed page of the live jobs index, narrowed
// to jobSitemapFilter, along with the total number of documents matching it. See
// sitemapPage.
func (c *Client) ListSitemapPage(ctx context.Context, offset, limit int) ([]SitemapDocument, int64, error) {
	return c.sitemapPage(ctx, c.facet, jobSlugField, jobSitemapFilter, offset, limit)
}

// freshSitemapSort orders the fresh job sub-sitemap. `created_at`, not `posted_at`:
// the question a crawler is being answered is "what is new HERE", and a posting an
// employer published months ago but this catalogue first saw an hour ago is new here.
// Sorting by the source's own date would bury exactly those behind repostings of
// things already crawled.
//
// The attribute is already in facetSettings().SortableAttributes, so this needs no
// settings patch and no reindex — the "settings must reach the LIVE index before the
// binary" hazard documented there does not apply, the same way jobSitemapFilter's
// attributes do not raise it.
const freshSitemapSort = "created_at:desc"

// ListFreshSitemapPage returns one offset-addressed page of the live jobs index
// narrowed to jobSitemapFilter and ordered newest first.
//
// It returns no total, unlike ListSitemapPage. The total exists there because the
// sitemap index TILES the paged files by it (CountSitemapDocuments); the fresh window
// is tiled by a constant instead, so a count here would be a number with no reader —
// and it could only ever be EstimatedTotalHits, which is not the exact figure its
// sibling returns.
//
// It searches rather than paging documents, and that is the entire point of its
// existence: Meilisearch's /documents route — which ListSitemapPage uses, and which is
// what lets the paged sub-sitemaps address any offset in a 700k-document index
// cheaply — returns documents in internal storage order and accepts no sort. Sorting
// exists only on /search. So the two readers are not duplicates of each other: one
// buys full coverage at the price of arbitrary order, the other buys order at the
// price of a bounded window, and web-ssr-seo asks for both.
//
// The window is bounded by its CALLER (see the handler's freshSitemapMaxOffset), not
// here, because the bound is a statement about crawl budget and response time rather
// than about the index: measured on prod 2026-09-22, a sorted page costs 2ms at
// offset 0, 527ms at 10,000 and 2.0s at 30,000 warm, against the SSR route's 10s
// fetch timeout.
func (c *Client) ListFreshSitemapPage(ctx context.Context, offset, limit int) ([]SitemapDocument, error) {
	resp, err := c.facet.SearchWithContext(ctx, "", &meilisearch.SearchRequest{
		Offset:               int64(offset),
		Limit:                int64(limit),
		Filter:               jobSitemapFilter,
		Sort:                 []string{freshSitemapSort},
		AttributesToRetrieve: []string{jobSlugField, lastmodField},
	})
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("search: fresh sitemap page: %w", ctxErr)
		}
		return nil, fmt.Errorf("search: fresh sitemap page: %w", err)
	}
	// Decoded through a map for the same reason sitemapPage does it: the slug
	// attribute is named per index, so a struct tag could only ever match one.
	var raw []map[string]any
	if err := resp.Hits.DecodeInto(&raw); err != nil {
		return nil, fmt.Errorf("search: fresh sitemap page: decode: %w", err)
	}
	return sitemapDocs(raw, jobSlugField), nil
}

// ListCompanySitemapPage is ListSitemapPage over the companies index, unfiltered: what
// belongs in the company sitemap is decided upstream by companies.job_count, not here,
// and jobSitemapFilter names attributes a company document does not have.
func (c *Client) ListCompanySitemapPage(ctx context.Context, offset, limit int) ([]SitemapDocument, int64, error) {
	return c.sitemapPage(ctx, c.manager.Index(companyIndexUID), companySlugField, "", offset, limit)
}

// CountSitemapDocuments returns how many of the live jobs index's documents the job
// sitemap names — i.e. how many match jobSitemapFilter — which is what the sitemap
// index divides into chunks. The total rides on every documents response, so this asks
// for the shortest page the engine will serve rather than a count of its own. Not
// zero: DocumentsQuery.Limit is `omitempty`, so a 0 is dropped from the request body
// and Meilisearch applies its own default of 20.
//
// It delegates to ListSitemapPage rather than issuing its own query, and that is the
// structural reason the count and the chunks cannot disagree: there is one filter, in
// one place, and a sitemap index that tiled by a wider population than its chunks
// serve would name chunks that come back empty.
func (c *Client) CountSitemapDocuments(ctx context.Context) (int64, error) {
	_, total, err := c.ListSitemapPage(ctx, 0, 1)
	return total, err
}

// CountCompanySitemapDocuments is CountSitemapDocuments over the companies index.
func (c *Client) CountCompanySitemapDocuments(ctx context.Context) (int64, error) {
	_, total, err := c.ListCompanySitemapPage(ctx, 0, 1)
	return total, err
}
