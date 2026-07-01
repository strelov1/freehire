package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/db"
)

// sitemapMaxURLs is the sitemap-protocol per-file cap. It doubles as the default
// and hard limit for a slice request and the chunk size the index uses to compute
// keyset boundaries, so a served chunk can never exceed the protocol limit.
const sitemapMaxURLs = 50000

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

// JobSitemap serves one keyset page of open-job sitemap entries after ?after=<id>.
func (a *API) JobSitemap(c *fiber.Ctx) error {
	rows, err := a.queries.ListJobSitemap(c.Context(), db.ListJobSitemapParams{
		AfterID:   int64(c.QueryInt("after", 0)),
		BatchSize: sitemapLimit(c),
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

// JobSitemapBoundaries returns the keyset cursor (job id) ending each ?chunk=<n> of
// open jobs, for building the sitemap index.
func (a *API) JobSitemapBoundaries(c *fiber.Ctx) error {
	cursors, err := a.queries.JobSitemapBoundaries(c.Context(), sitemapChunk(c))
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": cursors})
}

// CompanySitemap serves one keyset page of company sitemap entries after ?after=<slug>.
func (a *API) CompanySitemap(c *fiber.Ctx) error {
	rows, err := a.queries.ListCompanySitemap(c.Context(), db.ListCompanySitemapParams{
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
func (a *API) CompanySitemapBoundaries(c *fiber.Ctx) error {
	cursors, err := a.queries.CompanySitemapBoundaries(c.Context(), sitemapChunk(c))
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": cursors})
}
