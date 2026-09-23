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

// register mounts the five public sitemap reads behind the shared public-read budget.
//
// They carried no limiter at all until the deep-offset change, which is the same omission
// GET /companies/:slug/feedback carried and the reason both are now a documented rule:
// an unauthenticated list registered without a limiter is a defect whatever its query
// costs. The four /documents-backed reads are cheap per request — the pages come from
// Meilisearch, not from a table scan, and their cost is flat in the offset — but "cheap"
// is not "free", and an unthrottled route is simply the door a throttled caller walks
// through instead (the argument jobsHandlers.register already makes about /jobs versus
// /jobs/search).
//
// The fresh read is the one whose cost is NOT flat: it sorts, so it rises with depth —
// 1.3s to 2.6-4.9s end to end across its four chunks, and once 8.5s cold. It is admitted to the same
// budget on the strength of its own bound rather than on the paragraph above — a
// per-minute budget says nothing about an endpoint whose per-request work the caller
// sets (the lesson pageParamsWindowed records from 2026-09-14), so what makes 600/min
// safe here is that freshSitemapMaxOffset caps that work at a measured figure. See the
// constant.
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
// while the paged read does not. Two measurements, and only the second one bounds
// anything. The engine alone, warm, at the page size this route serves (limit=10,000,
// projected to slug and lastmod): 716ms at offset 0 rising to 1,776ms at 30,000 —
// which shows the SHAPE of the cost but not what the 10s fetch timeout is measured
// against. End to end through the live sub-sitemap after deploy: 1.3s, 1.6s, 1.9s and
// 2.6-4.9s for the four chunks, with ONE cold reading of 8.5s on the deepest chunk at
// the first request after a release — a 1.2x margin, not the 5.6x the engine timing
// alone suggests.
//
// Four chunks stands on that number rather than in spite of it: the cold peak was
// seen once, on a cold route and a cold index at once, and the cost of exceeding the
// timeout is ONE missing sub-sitemap on ONE fetch, which the crawler retries. The
// deepest chunk is also the least valuable — the oldest ~day of the window. If cold
// readings turn out to be common rather than a post-release artefact, drop to three
// chunks: offset 20,000 measured 1.9-2.0s end to end, and the change is
// freshSitemapMaxOffset here plus FRESH_SITEMAP_CHUNKS in web/src/lib/sitemap.ts —
// both, or the index names a file the API deliberately answers empty.
//
// What the bound has to cover is offset PLUS limit, not the offset alone, and that
// distinction is the whole guarantee: pageParamsBounded clamps ?limit= to the ceiling
// it is given, so passing sitemapMaxURLs there would admit
// `?offset=30000&limit=50000` — a sorted read to hit 80,000, 2.7x past this bound,
// on a public route inside the shared 600/min budget. The ceiling is therefore
// jobSitemapChunk, which is all the sitemap index ever asks for, and the deepest
// reachable hit is freshSitemapMaxOffset+jobSitemapChunk = 40,000.
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
