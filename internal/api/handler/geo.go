package handler

import (
	"context"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/dict/location"
)

// citySearchLimit bounds a single /geo/cities response.
const citySearchLimit = 20

// geoHandlers serves public geography reference reads (city autocomplete). Unauthenticated:
// it is dictionary data, not user-scoped.
type geoHandlers struct{}

func newGeoHandlers() *geoHandlers {
	return &geoHandlers{}
}

func (h *geoHandlers) register(api fiber.Router, contract fiber.Handler) {
	api.Get("/geo/cities", contract)
}

// SearchCities backs GET /geo/cities?q=&country=: a population-ranked prefix search
// over the embedded GeoNames city dictionary (location.SearchCities), optionally
// narrowed to one country. A blank q yields an empty (not null) list.
func (h *geoHandlers) SearchCities(_ context.Context, params apigen.SearchCitiesParams) (*apigen.SearchCitiesOK, error) {
	matches := location.SearchCities(params.Q.Value, params.Country.Value, citySearchLimit)
	out := make([]apigen.CityMatch, len(matches))
	for i, m := range matches {
		out[i] = apigen.CityMatch{Value: m.Name, Country: m.Country}
	}
	return &apigen.SearchCitiesOK{Data: out}, nil
}
