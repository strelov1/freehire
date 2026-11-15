package handler

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/search/search"
)

// sitemapLister is the search-index read both sitemaps page over: one offset-addressed
// page of indexed documents plus the index's total document count. Narrow on purpose: a
// sitemap needs a slug and a lastmod.
//
// One interface, two indexes: sitemapHandlers holds a jobs-bound and a companies-bound
// implementation, so the two halves cannot be wired to each other's index.
type sitemapLister interface {
	ListSitemapPage(ctx context.Context, offset, limit int) ([]search.SitemapDocument, int64, error)
	CountSitemapDocuments(ctx context.Context) (int64, error)
}

// freshSitemapLister is the jobs index's OTHER read: one page ordered newest first.
// Separate from sitemapLister because only the jobs index offers it; folding it in would
// oblige companySitemapIndex to implement a method that would be a lie.
type freshSitemapLister interface {
	ListFreshSitemapPage(ctx context.Context, offset, limit int) ([]search.SitemapDocument, error)
}

// companySitemapIndex adapts the client's companies-index methods to sitemapLister.
type companySitemapIndex struct{ c *search.Client }

func (a companySitemapIndex) ListSitemapPage(ctx context.Context, offset, limit int) ([]search.SitemapDocument, int64, error) {
	return a.c.ListCompanySitemapPage(ctx, offset, limit)
}

func (a companySitemapIndex) CountSitemapDocuments(ctx context.Context) (int64, error) {
	return a.c.CountCompanySitemapDocuments(ctx)
}

// sitemapHandlers serves the XML sitemaps for jobs and companies. The company sitemap
// literals are registered before the /companies/:slug param route (in Register) so they
// are not read as slugs; likewise /jobs/sitemap precedes /jobs/:slug.
//
// Both halves page a Meilisearch index by offset, so neither touches Postgres: what each
// sitemap covers is decided by what cmd/reindex and cmd/reindex-companies put in their
// indexes.
type sitemapHandlers struct {
	jobs      sitemapLister
	companies sitemapLister
	freshJobs freshSitemapLister
}

func newSitemapHandlers(jobs, companies sitemapLister, freshJobs freshSitemapLister) *sitemapHandlers {
	return &sitemapHandlers{jobs: jobs, companies: companies, freshJobs: freshJobs}
}

// register mounts the five public sitemap reads.
func (h *sitemapHandlers) register(api fiber.Router, mw middleware) {
	api.Get("/jobs/sitemap", h.JobSitemap)
	api.Get("/jobs/sitemap/fresh", h.FreshJobSitemap)
	api.Get("/jobs/sitemap/boundaries", h.JobSitemapBoundaries)
	api.Get("/companies/sitemap", h.CompanySitemap)
	api.Get("/companies/sitemap/boundaries", h.CompanySitemapBoundaries)
}

// sitemapMaxURLs is the sitemap-protocol per-file cap, the ceiling an untrusted ?limit= /
// ?chunk= is clamped to.
const sitemapMaxURLs = 50000

// companySitemapChunk is how many companies one sub-sitemap holds: the default for both
// ?limit= and ?chunk=, and the value web/src/lib/sitemap.ts SITEMAP_CHUNK must equal so
// each offset in the index opens exactly one file's worth.
//
// Not raised to match the job chunk: a company document is a tenth of a job's, and
// re-tiling would invalidate every sub-sitemap URL Google holds.
const companySitemapChunk = 10000

// jobSitemapChunk is how many jobs one sub-sitemap holds: the default for both ?limit= and
// ?chunk=, and the value web/src/lib/sitemap.ts JOB_SITEMAP_CHUNK must equal so each
// offset in the index opens exactly one file's worth.
//
// Sized by the SSR fetch timeout (10s), not the protocol cap: 25k-job pages were near the
// timeout at the deepest offset under load, which renders fine until the box is busy and
// then 500s. More, cheaper sub-sitemaps cost nothing (the sitemap index cap is 50,000
// entries), and a slow page is a missing file with no partial credit.
const jobSitemapChunk = 10000

// sitemapEntry is the slim wire shape a sitemap URL needs: the slug and a lastmod.
//
// `omitzero`, not `omitempty`: a document indexed before updated_at joined the company
// shape has no lastmod, and omitempty does NOTHING to a time.Time, so the field would ship
// as <lastmod>0001-01-01T00:00:00Z</lastmod>, which Google reads as "not modified since
// year 1" rather than absent.
type sitemapEntry struct {
	Slug      string    `json:"slug"`
	UpdatedAt time.Time `json:"updated_at,omitzero"`
}

// minSitemapChunk floors ?chunk= so the boundary list stays small: without a floor, an
// unauthenticated `?chunk=1` asks for one offset per indexed document, a response that
// scales with the catalogue. It is below every chunk size we serve, so it only binds a
// hand-crafted request.
const minSitemapChunk = 1000

// sitemapChunk clamps ?chunk= to [minSitemapChunk, sitemapMaxURLs], defaulting to
// `fallback`.
func sitemapChunk(c *fiber.Ctx, fallback int) int64 {
	return int64(min(max(c.QueryInt("chunk", fallback), minSitemapChunk), sitemapMaxURLs))
}

// servePage serves one page of sitemap entries at ?offset=<n> from `idx`.
//
// Reads limit and offset through the shared pageParamsBounded (see the helper and the test
// that pins it). Absent, unparseable, or out-of-range values collapse to the first page,
// and an offset past the end is a valid empty page: a crawler holding a stale sitemap
// index must never be answered with an error.
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

// serveBoundaries returns the offset opening each ?chunk=<n>-sized page of `idx`, for
// building the sitemap index: [0, chunk, 2*chunk, ...]. Same source as the page endpoint,
// so an offset always opens a page that has URLs in it. It is arithmetic over the one
// number the engine reports for free (a row_number() walk over Postgres timed out).
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

// freshSitemapMaxOffset is the last offset the fresh job sub-sitemap serves: four pages of
// jobSitemapChunk, about 3.5 days of intake, so a crawler that skips two days still misses
// nothing.
//
// It is a BOUND, enforced here rather than left to the sitemap index, because the sorted
// read behind it gets more expensive with depth while the paged read does not. End to end
// the deepest chunk ran ~2.6-4.9s with one cold outlier of 8.5s against the 10s fetch
// timeout, so four chunks is about the limit. Exceeding the timeout costs ONE missing
// sub-sitemap on ONE fetch, which the crawler retries, and the deepest chunk is the least
// valuable. If cold readings turn out common, drop to three chunks: change
// freshSitemapMaxOffset here and FRESH_SITEMAP_CHUNKS in web/src/lib/sitemap.ts, both, or
// the index names a file the API deliberately answers empty.
//
// The bound has to cover offset PLUS limit, not the offset alone: pageParamsBounded clamps
// ?limit= to the ceiling it is given, so passing sitemapMaxURLs would admit
// `?offset=30000&limit=50000`, a sorted read to 80,000 on a public route. The ceiling is
// therefore jobSitemapChunk, which is all the sitemap index asks for; the deepest
// reachable hit is freshSitemapMaxOffset+jobSitemapChunk.
const freshSitemapMaxOffset = 3 * jobSitemapChunk

// FreshJobSitemap serves one page of the newest-first job sub-sitemap.
//
// Past freshSitemapMaxOffset it answers an empty page WITHOUT asking the index: empty
// rather than an error, because a crawler holding a stale sitemap index must not get a
// failure (as servePage); without asking, because the refusal exists to keep a sorted read
// off a depth whose cost was never measured.
func (h *sitemapHandlers) FreshJobSitemap(c *fiber.Ctx) error {
	if h.freshJobs == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "search is not available")
	}
	// The ceiling is the chunk size, not sitemapMaxURLs: on this route the caller
	// controls how deep the engine reads through BOTH parameters. See the constant.
	limit, offset := pageParamsBounded(c, jobSitemapChunk, jobSitemapChunk)
	if offset > freshSitemapMaxOffset {
		return c.JSON(fiber.Map{"data": []sitemapEntry{}})
	}
	docs, err := h.freshJobs.ListFreshSitemapPage(c.Context(), offset, limit)
	if err != nil {
		return err
	}
	entries := make([]sitemapEntry, len(docs))
	for i, d := range docs {
		entries[i] = sitemapEntry{Slug: d.Slug, UpdatedAt: d.UpdatedAt}
	}
	return c.JSON(fiber.Map{"data": entries})
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
