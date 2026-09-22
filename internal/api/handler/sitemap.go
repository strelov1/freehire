package handler

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/search/search"
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

// freshSitemapLister is the jobs index's OTHER read: one page ordered newest first.
// Separate from sitemapLister, not a third method on it, because only the jobs index
// offers it — the companies index has no such ordering and no caller asking for one,
// so folding it in would oblige companySitemapIndex to implement a method that would
// be a lie.
type freshSitemapLister interface {
	ListFreshSitemapPage(ctx context.Context, offset, limit int) ([]search.SitemapDocument, int64, error)
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
	freshJobs freshSitemapLister
}

func newSitemapHandlers(jobs, companies sitemapLister, freshJobs freshSitemapLister) *sitemapHandlers {
	return &sitemapHandlers{jobs: jobs, companies: companies, freshJobs: freshJobs}
}

// register mounts the four public sitemap reads behind the shared public-read budget.
//
// They carried no limiter at all until the deep-offset change, which is the same omission
// GET /companies/:slug/feedback carried and the reason both are now a documented rule:
// an unauthenticated list registered without a limiter is a defect whatever its query
// costs. Nothing here is expensive per request — the pages come from Meilisearch, not from
// a table scan — but "cheap" is not "free", and an unthrottled route is simply the door a
// throttled caller walks through instead (the argument jobsHandlers.register already makes
// about /jobs versus /jobs/search).
//
// A real crawler is unaffected: it fetches a sitemap chunk at a time, minutes apart, and
// 600/min is three orders of magnitude above that.
func (h *sitemapHandlers) register(api fiber.Router, mw middleware) {
	readLimit := publicReadLimiter(mw.throttler)
	api.Get("/jobs/sitemap", readLimit, h.JobSitemap)
	api.Get("/jobs/sitemap/fresh", readLimit, h.FreshJobSitemap)
	api.Get("/jobs/sitemap/boundaries", readLimit, h.JobSitemapBoundaries)
	api.Get("/companies/sitemap", readLimit, h.CompanySitemap)
	api.Get("/companies/sitemap/boundaries", readLimit, h.CompanySitemapBoundaries)
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

// freshSitemapMaxOffset is the last offset the fresh job sub-sitemap serves — four
// pages of jobSitemapChunk, so 40,000 URLs, about 3.5 days of this catalogue's intake
// (the live index held 33,929 eligible postings created in the last 3 days when this
// was sized). A crawler that skips two days still misses nothing.
//
// It is a BOUND, not a page count, and it is enforced here rather than left to the
// sitemap index, because the sorted read behind it gets more expensive with depth
// while the paged read does not: measured on prod 2026-09-22, a sorted page costs 2ms
// at offset 0, 527ms at 10,000 and 2.0s at 30,000 warm (7.1s cold) against the SSR
// route's 10s fetch timeout. Without a server-side bound a hand-written
// `?offset=500000` would reach a depth nobody has measured, on a public route.
const freshSitemapMaxOffset = 3 * jobSitemapChunk

// FreshJobSitemap serves one page of the newest-first job sub-sitemap.
//
// Past freshSitemapMaxOffset it answers an empty page WITHOUT asking the index. Both
// halves matter: empty rather than an error, because a crawler holding a stale sitemap
// index must not be answered with a failure (the rule servePage already follows); and
// without asking, because the refusal exists precisely to keep a sorted read off a
// depth its cost was never measured at.
func (h *sitemapHandlers) FreshJobSitemap(c *fiber.Ctx) error {
	if h.freshJobs == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "search is not available")
	}
	limit, offset := pageParamsBounded(c, jobSitemapChunk, sitemapMaxURLs)
	if offset > freshSitemapMaxOffset {
		return c.JSON(fiber.Map{"data": []sitemapEntry{}})
	}
	docs, _, err := h.freshJobs.ListFreshSitemapPage(c.Context(), offset, limit)
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
