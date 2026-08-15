package handler

import (
	"math"
	"strconv"
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
// equal so each boundary cursor opens exactly one file's worth.
//
// The sitemap used to ship only the freshest 15k of the catalogue, because the read
// was heap-bound: 50k measured 30-40s on prod and 25k twice hit 57s against nginx's
// 60s ceiling. jobs_sitemap_idx (0107) makes a chunk an index-only range scan, so
// the whole open set can be paged by cursor instead. Sized above the company chunk
// because that index covers everything these queries read, and kept below the
// protocol's 50k so a chunk is never near either ceiling.
const jobSitemapChunk = 25000

// firstChunkCursor opens the first keyset chunk: ids are read newest-first with a
// plain `id < cursor`, so the opening cursor has to be larger than any real id.
// Keeping it a bound (rather than a nullable comparison) is what keeps the scan
// index-only.
const firstChunkCursor = int64(math.MaxInt64)

// sitemapEntry is the slim wire shape a sitemap URL needs — the public slug and a
// lastmod. Nothing wider (no full job row, no search engine) crosses the wire.
// updated_at is NOT NULL on both tables, so it is always a real instant.
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

// sitemapAfterID reads the ?after= keyset cursor. Absent or unparseable means the
// first chunk, which starts above every id — a crawler following a stale index can
// only ever get a valid page, never an error.
func sitemapAfterID(c *fiber.Ctx) int64 {
	raw := c.Query("after")
	if raw == "" {
		return firstChunkCursor
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return firstChunkCursor
	}
	return id
}

// JobSitemap serves one keyset page of open-job sitemap entries after ?after=<id>,
// newest id first.
func (h *sitemapHandlers) JobSitemap(c *fiber.Ctx) error {
	rows, err := h.queries.ListJobSitemapChunk(c.Context(), db.ListJobSitemapChunkParams{
		AfterID:  sitemapAfterID(c),
		RowLimit: sitemapLimit(c, jobSitemapChunk),
	})
	if err != nil {
		return err
	}
	entries := make([]sitemapEntry, len(rows))
	for i, r := range rows {
		entries[i] = sitemapEntry{Slug: r.PublicSlug, UpdatedAt: r.UpdatedAt.Time}
	}
	return c.JSON(fiber.Map{"data": entries})
}

// JobSitemapBoundaries returns the keyset cursor (id) ending each ?chunk=<n> of
// sitemap-eligible jobs, for building the sitemap index. Same scope as JobSitemap,
// so a cursor always opens a chunk that has URLs in it.
func (h *sitemapHandlers) JobSitemapBoundaries(c *fiber.Ctx) error {
	cursors, err := h.queries.JobSitemapBoundaries(c.Context(), sitemapChunk(c, jobSitemapChunk))
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": cursors})
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
