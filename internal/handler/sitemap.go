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

// jobSitemapFreshest is how many of the newest open jobs the sitemap ships. The
// jobs table is far too large (millions of rows) to enumerate per request without
// a heap-bound scan that also evicts the buffer cache, so the sitemap covers the
// freshest slice (ordered by id DESC, a cache-warm scan); fuller coverage would
// need a precomputed narrow table. Held below the 50k protocol cap so the one file
// builds under the 60s proxy timeout even against I/O contention — 50k measured
// ~30-40s under load, and 25k still measured 29s and 57s on two prod probes minutes
// apart, so this is the margin that band needs, not the row count we'd prefer.
const jobSitemapFreshest = 15000

// sitemapEntry is the slim wire shape a sitemap URL needs — the public slug and a
// lastmod. Nothing wider (no full job row, no search engine) crosses the wire.
// updated_at is NOT NULL on both tables, so it is always a real instant.
type sitemapEntry struct {
	Slug      string    `json:"slug"`
	UpdatedAt time.Time `json:"updated_at"`
}

// sitemapLimit clamps ?limit= to [1, sitemapMaxURLs], defaulting to the chunk size.
func sitemapLimit(c *fiber.Ctx) int32 {
	return int32(min(max(c.QueryInt("limit", companySitemapChunk), 1), sitemapMaxURLs))
}

// sitemapChunk clamps ?chunk= to [1, sitemapMaxURLs], defaulting to the chunk size.
func sitemapChunk(c *fiber.Ctx) int64 {
	return int64(min(max(c.QueryInt("chunk", companySitemapChunk), 1), sitemapMaxURLs))
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

// CompanySitemap serves one keyset page of company sitemap entries after ?after=<slug>,
// covering the hiring companies the /companies catalog lists (see ListCompanySitemap).
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
// hiring companies, for building the sitemap index. Same scope as CompanySitemap, so a
// cursor always opens a chunk that has URLs in it.
func (h *sitemapHandlers) CompanySitemapBoundaries(c *fiber.Ctx) error {
	cursors, err := h.queries.CompanySitemapBoundaries(c.Context(), sitemapChunk(c))
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": cursors})
}
