package handler

import (
	"errors"
	"net/url"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/candidate/talentnetwork"
	"github.com/strelov1/freehire/internal/search/search"
)

// talentCatalogHandlers serves the PUBLIC Talent Network catalogue: the filtered list of
// members and one member's card. Both are unauthenticated by design — the catalogue is
// the front door of the feature, and a signed-out visitor is exactly who it is for — so
// neither route takes any auth middleware.
//
// What they do take is a rate limiter. This is a small, complete, machine-readable set of
// people, which is precisely the thing worth copying in one evening; the limiter is the
// only thing standing between the catalogue and a copy of it.
type talentCatalogHandlers struct {
	catalogue *talentnetwork.Catalogue
}

func newTalentCatalogHandlers(catalogue *talentnetwork.Catalogue) *talentCatalogHandlers {
	return &talentCatalogHandlers{catalogue: catalogue}
}

func (h *talentCatalogHandlers) register(api fiber.Router, mw middleware) {
	// The limiter is attached to these two routes rather than to a group, so it cannot
	// be lost by a later reshuffle that moves a route out of the group it was assumed to
	// be in. It is the catalogue's OWN budget, not the shared public-read one — see
	// talentCatalogPerMinute for why a read that returns people is bounded separately
	// from one that returns postings.
	limiter := talentCatalogLimiter(mw.throttler)
	api.Get("/talent", limiter, h.List)
	api.Get("/talent/:handle", limiter, h.Get)
}

// List serves one filtered, ordered page of the catalogue.
func (h *talentCatalogHandlers) List(c *fiber.Ctx) error {
	vals := queryValues(c)
	q, unreadable := talentnetwork.QueryFromValues(vals)

	page, err := h.catalogue.List(c.Context(), q)
	if err != nil {
		return err
	}
	return listResponseWithIgnored(c, page.Members, int64(page.Total), q.Limit, q.Offset,
		ignoredTalentParams(vals, unreadable))
}

// Get serves one member's card by the handle in their public URL.
func (h *talentCatalogHandlers) Get(c *fiber.Ctx) error {
	member, err := h.catalogue.ByHandle(c.Context(), c.Params("handle"))
	if err != nil {
		if errors.Is(err, talentnetwork.ErrNotFound) {
			// The one 404 every way of being absent produces — gone, stale, never
			// existed, not even a handle. Distinguishing them would turn this route
			// into a way of asking whether an account exists.
			return fiber.NewError(fiber.StatusNotFound, "not found")
		}
		return err
	}

	// A card is a person, and a cache of one outlives their decision to leave. Short,
	// and marked so a crawler that finds the JSON does not keep it either — the page's
	// own noindex covers the HTML, this covers the endpoint behind it.
	c.Set("Cache-Control", "public, max-age=60")
	c.Set("X-Robots-Tag", "noindex")
	return c.JSON(fiber.Map{"data": member})
}

// ignoredTalentParams reports what this listing did not read: the params it does not
// recognise at all, plus the ones it recognises but could not READ (`min_years=lots`).
//
// Both belong in one report because both have the same consequence — the answer is wider
// than the caller asked for — and one report because the cap and the ordering have to
// cover the two together. Appending after SortAndCap would push the total past the bound
// that exists to enforce it.
//
// The vocabulary is talentnetwork's, not search's. Passing these facets to
// search.UnknownParams as `alsoKnown` would additionally accept every job-search facet
// on an endpoint that reads none of them, and report nothing when one arrives.
func ignoredTalentParams(vals url.Values, unreadable []string) []search.UnknownParam {
	ignored := search.UnknownParamsAgainst(vals, talentnetwork.KnownParams())
	for _, param := range unreadable {
		ignored = append(ignored, search.UnknownParam{Param: param})
	}
	return search.SortAndCap(ignored)
}
