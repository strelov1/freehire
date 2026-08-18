package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/location"
)

// citySearchLimit bounds a single /geo/cities response, matching the cap the other
// remote-search facets (company, subindustry) already apply to their result lists.
const citySearchLimit = 20

// geoHandlers serves public geography reference reads — currently just city search,
// backing the profile's base-city and relocation-cities autocomplete. Unauthenticated,
// like the other public facet/reference endpoints (company subindustries): this is
// dictionary data, not anything user-scoped.
type geoHandlers struct{}

func newGeoHandlers() *geoHandlers {
	return &geoHandlers{}
}

func (h *geoHandlers) register(api fiber.Router, mw middleware) {
	api.Get("/geo/cities", publicReadLimiter(mw.throttler), h.SearchCities)
}

// cityMatch is the wire shape of one city-search result. country is a raw ISO
// 3166-1 alpha-2 code, not a composed display label — the caller already has
// countryLabel() (web/src/lib/facets.ts, Intl.DisplayNames-backed) to render it,
// so the backend doesn't need a second, hand-maintained country-name table.
type cityMatch struct {
	Value   string `json:"value"`
	Country string `json:"country"`
}

// SearchCities backs GET /geo/cities?q=&country=: a population-ranked prefix search
// over the embedded GeoNames city dictionary (location.SearchCities), optionally
// narrowed to one country. A blank q yields an empty (not null) list.
func (h *geoHandlers) SearchCities(c *fiber.Ctx) error {
	matches := location.SearchCities(c.Query("q"), c.Query("country"), citySearchLimit)
	out := make([]cityMatch, len(matches))
	for i, m := range matches {
		out[i] = cityMatch{Value: m.Name, Country: m.Country}
	}
	return c.JSON(fiber.Map{"data": out})
}
