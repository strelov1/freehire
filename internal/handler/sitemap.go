package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/db"
)

// sitemapHandlers serves the XML sitemaps for jobs and companies. The company
// sitemap literals are registered before the /companies/:slug param route (in
// Register) so they are not read as slugs; likewise /jobs/sitemap precedes
// /jobs/:slug.
type sitemapHandlers struct {
	queries *db.Queries
}

func newSitemapHandlers(queries *db.Queries) *sitemapHandlers {
	return &sitemapHandlers{queries: queries}
}

func (h *sitemapHandlers) register(api fiber.Router) {
	api.Get("/jobs/sitemap", h.JobSitemap)
	api.Get("/companies/sitemap", h.CompanySitemap)
	api.Get("/companies/sitemap/boundaries", h.CompanySitemapBoundaries)
}

// sitemapMaxURLs is the sitemap-protocol per-file cap. It bounds the company slice
// and the chunk size the company index uses for keyset boundaries, so a served
// chunk can never exceed the protocol limit.
const sitemapMaxURLs = 50000

// jobSitemapFreshest is how many of the newest open jobs the sitemap ships. The
// jobs table is far too large (millions of rows) to enumerate per request without
// a heap-bound scan that also evicts the buffer cache, so the sitemap covers the
// freshest slice (ordered by id DESC, a cache-warm scan); fuller coverage would
// need a precomputed narrow table. Held below the 50k protocol cap so that even
// during the periodic reindex's I/O contention the one file builds well under the
// 60s proxy timeout (50k measured ~30-40s under load; 25k halves that), while still
// fitting a single sub-sitemap with no chunking.
const jobSitemapFreshest = 25000

// sitemapEntry is the slim wire shape a sitemap URL needs — the public slug and a
// lastmod. Nothing wider (no full job row, no search engine) crosses the wire.
// updated_at is NOT NULL on both tables, so it is always a real instant.
type sitemapEntry struct {
	Slug      string    `json:"slug"`
	UpdatedAt time.Time `json:"updated_at"`
}

// sitemapLimit clamps ?limit= to [1, sitemapMaxURLs], defaulting to the protocol cap.
func sitemapLimit(c *fiber.Ctx) int32 {
	return int32(min(max(c.QueryInt("limit", sitemapMaxURLs), 1), sitemapMaxURLs))
}

// sitemapChunk clamps ?chunk= to [1, sitemapMaxURLs], defaulting to the protocol cap.
func sitemapChunk(c *fiber.Ctx) int64 {
	return int64(min(max(c.QueryInt("chunk", sitemapMaxURLs), 1), sitemapMaxURLs))
}

// JobSitemap serves the freshest open-job sitemap entries (newest id first).
func (h *sitemapHandlers) JobSitemap(c *fiber.Ctx) error {
	rows, err := h.queries.ListJobSitemapFreshest(c.Context(), jobSitemapFreshest)
	if err != nil {
		return err
	}
	entries := make([]sitemapEntry, len(rows))
	for i, r := range rows {
		entries[i] = sitemapEntry{Slug: r.PublicSlug, UpdatedAt: r.UpdatedAt.Time}
	}
	return c.JSON(fiber.Map{"data": entries})
}

// CompanySitemap serves one keyset page of company sitemap entries after ?after=<slug>.
func (h *sitemapHandlers) CompanySitemap(c *fiber.Ctx) error {
	rows, err := h.queries.ListCompanySitemap(c.Context(), db.ListCompanySitemapParams{
		AfterSlug: c.Query("after"),
		BatchSize: sitemapLimit(c),
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
// companies, for building the sitemap index.
func (h *sitemapHandlers) CompanySitemapBoundaries(c *fiber.Ctx) error {
	cursors, err := h.queries.CompanySitemapBoundaries(c.Context(), sitemapChunk(c))
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": cursors})
}
