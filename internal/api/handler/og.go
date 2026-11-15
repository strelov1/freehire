package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/ogimage"
	"github.com/strelov1/freehire/internal/ingest/catalogstats"
	"github.com/strelov1/freehire/internal/platform/cache"
	"github.com/strelov1/freehire/internal/platform/db"
)

// ogImageCacheControl matches the other on-demand OG cards; the CDN respects it, so a
// repeat request within the hour is an edge hit.
const ogImageCacheControl = "public, max-age=3600, stale-while-revalidate=86400"

// ogHandlers serves the OG cards for /open and /about, reading the same catalogstats
// snapshot as statsHandlers.CatalogScale so card and page figures agree.
type ogHandlers struct {
	cache     cache.Cache
	estimator catalogstats.Estimator
}

func newOGHandlers(queries *db.Queries, c cache.Cache) *ogHandlers {
	return &ogHandlers{cache: c, estimator: queries}
}

func (h *ogHandlers) register(api fiber.Router) {
	api.Get("/og/open.png", h.Open)
	api.Get("/og/about.png", h.About)
}

// Open serves the OG card for the /open transparency page.
func (h *ogHandlers) Open(c *fiber.Ctx) error {
	return h.render(c, "freehire's numbers, live", "Every figure comes straight from the public API.")
}

// About serves the OG card for the /about page.
func (h *ogHandlers) About(c *fiber.Ctx) error {
	return h.render(c,
		"The open-source search engine for tech jobs",
		"Millions of openings indexed straight from company career boards, deduplicated and tagged by stack and location.",
	)
}

func (h *ogHandlers) render(c *fiber.Ctx, heading, tagline string) error {
	result := catalogstats.Load(c.Context(), h.cache, h.estimator)

	img, err := ogimage.RenderCatalogCard(heading, tagline, ogimage.Stats{
		OpenJobs:         result.OpenJobs,
		UniqueOpenJobs:   result.UniqueOpenJobs,
		Companies:        result.Companies,
		Sources:          result.Sources,
		TelegramChannels: result.TelegramChannels,
		Exact:            result.Exact,
	})
	if err != nil {
		return err
	}

	c.Set(fiber.HeaderContentType, "image/png")
	c.Set(fiber.HeaderCacheControl, ogImageCacheControl)
	return c.Send(img)
}
