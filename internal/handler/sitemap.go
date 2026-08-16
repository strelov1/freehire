package handler

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/search"
)

// sitemapLister is the search-index read the job sitemap pages over: one
// offset-addressed page of indexed jobs plus the index's total document count.
// Narrow on purpose — the sitemap needs a slug and a lastmod, nothing the search
// handler's `searcher` offers.
type sitemapLister interface {
	ListSitemapPage(ctx context.Context, offset, limit int) ([]search.SitemapDocument, int64, error)
	CountSitemapDocuments(ctx context.Context) (int64, error)
}

// sitemapHandlers serves the XML sitemaps for jobs and companies. The company
// sitemap literals are registered before the /companies/:slug param route (in
// Register) so they are not read as slugs; likewise /jobs/sitemap precedes
// /jobs/:slug.
//
// Jobs and companies are paged differently because their sources are: companies
// are keyset-paged out of Postgres by slug, while jobs are offset-paged out of the
// Meilisearch index (see jobs below).
type sitemapHandlers struct {
	queries *db.Queries
	jobs    sitemapLister
}

func newSitemapHandlers(queries *db.Queries, jobs sitemapLister) *sitemapHandlers {
	return &sitemapHandlers{queries: queries, jobs: jobs}
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
// equal so each boundary cursor opens exactly one file's worth.
//
// Deliberately far below the protocol cap. These reads compete with the ingest for
// the buffer cache and their latency swings with it by more than an order of
// magnitude: on prod (2026-07-29) the same 50k-row chunk measured 0.9s warm and past
// 60s — an nginx 504 — while an ingest run was evicting the cache. A tenth of the
// rows keeps the slow end inside the proxy timeout; the cost is ~21 files instead of
// ~5, which a sitemap index carries for free (its own cap is 50k entries).
const companySitemapChunk = 10000

// jobSitemapChunk is how many jobs one sub-sitemap holds: the default for both
// ?limit= and ?chunk=, and the value web/src/lib/sitemap.ts JOB_SITEMAP_CHUNK must
// equal so each offset in the index opens exactly one file's worth.
//
// Kept below the protocol's 50k cap with room to spare: a 25k page measured ~2.5s
// against the prod index, and doubling it would put a single sub-sitemap's render
// near the SSR timeout for no gain — a sitemap index carries the extra files for
// free (its own cap is 50k entries).
const jobSitemapChunk = 25000

// sitemapEntry is the slim wire shape a sitemap URL needs — the public slug and a
// lastmod. Nothing wider (no full job row) crosses the wire. updated_at is NOT NULL
// on both tables, so it is always a real instant.
type sitemapEntry struct {
	Slug      string    `json:"slug"`
	UpdatedAt time.Time `json:"updated_at"`
}

// sitemapLimit clamps ?limit= to [1, sitemapMaxURLs], defaulting to `fallback`.
func sitemapLimit(c *fiber.Ctx, fallback int) int32 {
	return int32(min(max(c.QueryInt("limit", fallback), 1), sitemapMaxURLs))
}

// sitemapChunk clamps ?chunk= to [1, sitemapMaxURLs], defaulting to `fallback`.
func sitemapChunk(c *fiber.Ctx, fallback int) int64 {
	return int64(min(max(c.QueryInt("chunk", fallback), 1), sitemapMaxURLs))
}

// JobSitemap serves one page of job sitemap entries at ?offset=<n>, read from the
// search index (see search.Client.ListSitemapPage for why the index and not the
// jobs table, and why an offset and not a keyset cursor).
//
// Reads its page through the shared pageParamsBounded rather than the sitemapLimit
// helper below, because it is the one sitemap endpoint that takes an offset — and
// that parse belongs in exactly one place (see the helper, and the test that pins
// it). Absent, unparseable, or out-of-range values collapse to the first page, and
// an offset past the end is a valid empty page: a crawler holding a stale sitemap
// index must never be answered with an error.
func (h *sitemapHandlers) JobSitemap(c *fiber.Ctx) error {
	if h.jobs == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "search is not available")
	}
	limit, offset := pageParamsBounded(c, jobSitemapChunk, sitemapMaxURLs)
	docs, _, err := h.jobs.ListSitemapPage(c.Context(), offset, limit)
	if err != nil {
		return err
	}
	entries := make([]sitemapEntry, len(docs))
	for i, d := range docs {
		entries[i] = sitemapEntry{Slug: d.Slug, UpdatedAt: d.UpdatedAt}
	}
	return c.JSON(fiber.Map{"data": entries})
}

// JobSitemapBoundaries returns the offset opening each ?chunk=<n>-sized page of the
// jobs index, for building the sitemap index — [0, chunk, 2*chunk, ...]. Same source
// as JobSitemap, so an offset always opens a page that has URLs in it.
//
// This is arithmetic over one number the engine reports for free, where the jobs
// table needed a `row_number()` walk of the whole catalogue to find the same
// boundaries — the walk that grew to 64s and started timing out (see
// search.Client.ListSitemapPage).
func (h *sitemapHandlers) JobSitemapBoundaries(c *fiber.Ctx) error {
	if h.jobs == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "search is not available")
	}
	total, err := h.jobs.CountSitemapDocuments(c.Context())
	if err != nil {
		return err
	}
	chunk := sitemapChunk(c, jobSitemapChunk)
	offsets := make([]int64, 0, total/chunk+1)
	for off := int64(0); off < total; off += chunk {
		offsets = append(offsets, off)
	}
	return c.JSON(fiber.Map{"data": offsets})
}

// CompanySitemap serves one keyset page of company sitemap entries after ?after=<slug>,
// covering the hiring companies the /companies catalog lists (see ListCompanySitemap).
func (h *sitemapHandlers) CompanySitemap(c *fiber.Ctx) error {
	rows, err := h.queries.ListCompanySitemap(c.Context(), db.ListCompanySitemapParams{
		AfterSlug: c.Query("after"),
		BatchSize: sitemapLimit(c, companySitemapChunk),
	})
	if err != nil {
		return err
	}
	entries := make([]sitemapEntry, len(rows))
	for i, r := range rows {
		entries[i] = sitemapEntry{Slug: r.Slug, UpdatedAt: r.UpdatedAt.Time}
	}
	return c.JSON(fiber.Map{"data": entries})
}

// CompanySitemapBoundaries returns the keyset cursor (slug) ending each ?chunk=<n> of
// hiring companies, for building the sitemap index. Same scope as CompanySitemap, so a
// cursor always opens a chunk that has URLs in it.
func (h *sitemapHandlers) CompanySitemapBoundaries(c *fiber.Ctx) error {
	cursors, err := h.queries.CompanySitemapBoundaries(c.Context(), sitemapChunk(c, companySitemapChunk))
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": cursors})
}
