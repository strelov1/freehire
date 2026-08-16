package handler

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/search"
)

// sitemapLister is the search-index read both sitemaps page over: one
// offset-addressed page of indexed documents plus the index's total document count.
// Narrow on purpose — a sitemap needs a slug and a lastmod, nothing the search
// handler's `searcher` offers.
//
// One interface, two indexes: sitemapHandlers holds a jobs-bound and a
// companies-bound implementation rather than a single client with four methods, so
// the two halves cannot be wired to each other's index by mistake.
type sitemapLister interface {
	ListSitemapPage(ctx context.Context, offset, limit int) ([]search.SitemapDocument, int64, error)
	CountSitemapDocuments(ctx context.Context) (int64, error)
}

// companySitemapIndex adapts the client's companies-index methods to sitemapLister.
type companySitemapIndex struct{ c *search.Client }

func (a companySitemapIndex) ListSitemapPage(ctx context.Context, offset, limit int) ([]search.SitemapDocument, int64, error) {
	return a.c.ListCompanySitemapPage(ctx, offset, limit)
}

func (a companySitemapIndex) CountSitemapDocuments(ctx context.Context) (int64, error) {
	return a.c.CountCompanySitemapDocuments(ctx)
}

// sitemapHandlers serves the XML sitemaps for jobs and companies. The company
// sitemap literals are registered before the /companies/:slug param route (in
// Register) so they are not read as slugs; likewise /jobs/sitemap precedes
// /jobs/:slug.
//
// Both halves page a Meilisearch index by offset, so neither touches Postgres: what
// each sitemap covers is decided by what cmd/reindex and cmd/reindex-companies put
// in their indexes.
type sitemapHandlers struct {
	jobs      sitemapLister
	companies sitemapLister
}

func newSitemapHandlers(jobs, companies sitemapLister) *sitemapHandlers {
	return &sitemapHandlers{jobs: jobs, companies: companies}
}

func (h *sitemapHandlers) register(api fiber.Router) {
	api.Get("/jobs/sitemap", h.JobSitemap)
	api.Get("/jobs/sitemap/boundaries", h.JobSitemapBoundaries)
	api.Get("/companies/sitemap", h.CompanySitemap)
	api.Get("/companies/sitemap/boundaries", h.CompanySitemapBoundaries)
}

// sitemapMaxURLs is the sitemap-protocol per-file cap — the hard ceiling an
// untrusted ?limit= / ?chunk= is clamped to, so a served chunk can never exceed the
// protocol limit however it is asked for.
const sitemapMaxURLs = 50000

// companySitemapChunk is how many companies one sub-sitemap holds: the default for
// both ?limit= and ?chunk=, and the value web/src/lib/sitemap.ts SITEMAP_CHUNK must
// equal so each offset in the index opens exactly one file's worth.
//
// Kept at the size the Postgres-backed version needed rather than raised to match
// the job chunk: a company document is a tenth of a job's, so 10k is already a
// cheap page, and re-tiling the files would invalidate every sub-sitemap URL
// Google currently holds for no measured gain.
const companySitemapChunk = 10000

// jobSitemapChunk is how many jobs one sub-sitemap holds: the default for both
// ?limit= and ?chunk=, and the value web/src/lib/sitemap.ts JOB_SITEMAP_CHUNK must
// equal so each offset in the index opens exactly one file's worth.
//
// Sized by the SSR fetch timeout, not by the protocol cap. At 25k a page measured
// ~2.5s warm against the prod index but 8s on the deepest offset with the host under
// load — against a 10s timeout, i.e. a file that renders fine until the box is busy
// and then 500s. 10k restores the margin at ~1-3s per page.
//
// The cost is 127 sub-sitemaps instead of 51, which a sitemap index carries for free
// (its own cap is 50,000 entries). Cheap files beat a narrow deadline: a slow page is
// one missing file, but there is no partial credit — the crawler either gets it or
// gets an error.
const jobSitemapChunk = 10000

// sitemapEntry is the slim wire shape a sitemap URL needs — the slug and a lastmod.
// Nothing wider (no full job or company document) crosses the wire.
//
// `omitzero`, not `omitempty`: a document indexed before updated_at joined the
// company shape has no lastmod, and omitempty does NOTHING to a time.Time (a struct
// is never "empty"), so the field would ship as the year-1 zero instant and the SPA
// would emit <lastmod>0001-01-01T00:00:00Z</lastmod> — a date Google reads as
// "not modified since year 1" rather than as absent. omitzero drops it properly.
type sitemapEntry struct {
	Slug      string    `json:"slug"`
	UpdatedAt time.Time `json:"updated_at,omitzero"`
}

// minSitemapChunk floors ?chunk= so the boundary list stays small. Without a floor,
// an unauthenticated `?chunk=1` asks for one offset per indexed document — 1.26M
// int64s allocated and serialized in a single public response, scaling with the
// catalogue. At this floor the same request yields ~1.3k offsets and still covers
// the whole index, which is also well inside the sitemap index's own 50,000-entry
// cap. It is below every chunk size we actually serve (10k companies, 25k jobs), so
// it only ever binds a hand-crafted request.
const minSitemapChunk = 1000

// sitemapChunk clamps ?chunk= to [minSitemapChunk, sitemapMaxURLs], defaulting to
// `fallback`.
func sitemapChunk(c *fiber.Ctx, fallback int) int64 {
	return int64(min(max(c.QueryInt("chunk", fallback), minSitemapChunk), sitemapMaxURLs))
}

// servePage serves one page of sitemap entries at ?offset=<n> from `idx`.
//
// Reads limit and offset through the shared pageParamsBounded, because that parse
// belongs in exactly one place (see the helper, and the test that pins it). Absent,
// unparseable, or out-of-range values collapse to the first page, and an offset past
// the end is a valid empty page: a crawler holding a stale sitemap index must never
// be answered with an error.
func servePage(c *fiber.Ctx, idx sitemapLister, chunk int) error {
	if idx == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "search is not available")
	}
	limit, offset := pageParamsBounded(c, chunk, sitemapMaxURLs)
	docs, _, err := idx.ListSitemapPage(c.Context(), offset, limit)
	if err != nil {
		return err
	}
	entries := make([]sitemapEntry, len(docs))
	for i, d := range docs {
		entries[i] = sitemapEntry{Slug: d.Slug, UpdatedAt: d.UpdatedAt}
	}
	return c.JSON(fiber.Map{"data": entries})
}

// serveBoundaries returns the offset opening each ?chunk=<n>-sized page of `idx`,
// for building the sitemap index — [0, chunk, 2*chunk, ...]. Same source as the page
// endpoint, so an offset always opens a page that has URLs in it.
//
// This is arithmetic over one number the engine reports for free. The jobs table
// needed a `row_number()` walk of the whole catalogue to find the same boundaries —
// the walk that grew to 64s and started timing out (see search.Client.sitemapPage).
func serveBoundaries(c *fiber.Ctx, idx sitemapLister, fallback int) error {
	if idx == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "search is not available")
	}
	total, err := idx.CountSitemapDocuments(c.Context())
	if err != nil {
		return err
	}
	chunk := sitemapChunk(c, fallback)
	offsets := make([]int64, 0, total/chunk+1)
	for off := int64(0); off < total; off += chunk {
		offsets = append(offsets, off)
	}
	return c.JSON(fiber.Map{"data": offsets})
}

// JobSitemap serves one page of job sitemap entries from the jobs index.
func (h *sitemapHandlers) JobSitemap(c *fiber.Ctx) error {
	return servePage(c, h.jobs, jobSitemapChunk)
}

// JobSitemapBoundaries lists the offset opening each page of the jobs index.
func (h *sitemapHandlers) JobSitemapBoundaries(c *fiber.Ctx) error {
	return serveBoundaries(c, h.jobs, jobSitemapChunk)
}

// CompanySitemap serves one page of company sitemap entries, covering the hiring
// companies the /companies catalog lists — the scope cmd/reindex-companies indexes.
func (h *sitemapHandlers) CompanySitemap(c *fiber.Ctx) error {
	return servePage(c, h.companies, companySitemapChunk)
}

// CompanySitemapBoundaries lists the offset opening each page of the companies index.
func (h *sitemapHandlers) CompanySitemapBoundaries(c *fiber.Ctx) error {
	return serveBoundaries(c, h.companies, companySitemapChunk)
}
