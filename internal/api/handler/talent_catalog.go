package handler

import (
	"errors"
	"net/url"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/candidate/headshot"
	"github.com/strelov1/freehire/internal/candidate/talentnetwork"
	"github.com/strelov1/freehire/internal/search/search"
)

// talentCatalogHandlers serves the PUBLIC Talent Network catalogue: the filtered list of
// members, one member's card, and — when a member has uploaded one — a heavily blurred
// rendering of their photo. All three are unauthenticated by design — the catalogue is
// the front door of the feature, and a signed-out visitor is exactly who it is for — so
// none of the routes take any auth middleware.
type talentCatalogHandlers struct {
	catalogue *talentnetwork.Catalogue
	photos    *headshot.Store
}

func newTalentCatalogHandlers(catalogue *talentnetwork.Catalogue, photos *headshot.Store) *talentCatalogHandlers {
	return &talentCatalogHandlers{catalogue: catalogue, photos: photos}
}

func (h *talentCatalogHandlers) register(api fiber.Router, mw middleware) {
	api.Get("/talent", h.List)
	// Before the parametrised route, or `/talent/:handle` swallows it and every request
	// for the facets is answered as a lookup of a member called "facets".
	api.Get("/talent/facets", h.Facets)
	api.Get("/talent/:handle", h.Get)
	api.Get("/talent/:handle/photo", h.GetPhoto)
}

// Facets serves how many members stand behind each filter value, for the filter the
// caller currently has.
//
// It is what makes the open vocabularies usable: skills are thousands of canonicals, and
// a control offering them without counts cannot tell a value nobody carries from one
// whose members have all left. Same query vocabulary as the list, same unread-param
// report.
func (h *talentCatalogHandlers) Facets(c *fiber.Ctx) error {
	vals := queryValues(c)
	q, unreadable := talentnetwork.QueryFromValues(vals)

	counts, err := h.catalogue.Facets(c.Context(), q)
	if err != nil {
		return err
	}
	return dataResponseWithIgnored(c, counts, ignoredTalentParams(vals, unreadable), nil)
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
		ignoredTalentParams(vals, unreadable), nil)
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

	// A card is a person, and a cache of one outlives their decision to leave.
	//
	// `private`, not `public`, even though the response carries no session and no
	// personal data in the ordinary sense: `public` is the directive that authorises a
	// SHARED cache — a CDN, a corporate proxy — to hold and re-serve it. This route
	// promises that leaving takes effect on the next request, and an intermediary
	// holding a departed member's card for a minute is exactly that promise broken by
	// somebody we cannot ask to stop.
	c.Set("Cache-Control", "private, max-age=60")
	c.Set("X-Robots-Tag", "noindex")
	return c.JSON(fiber.Map{"data": member})
}

// GetPhoto serves a member's photo, blurred to the point of being unrecognisable —
// never the stored original, which no public route ever serves. Every reason the route
// might not have an image to serve — the handle is not a current member, the member has
// no headshot uploaded, or headshot storage is unavailable — answers identically, the
// same 404 `Get` gives a handle nobody holds: a caller must not be able to tell "not a
// member" apart from "a member with no photo" by the shape of the response.
func (h *talentCatalogHandlers) GetPhoto(c *fiber.Ctx) error {
	userID, err := h.catalogue.HeadshotOwner(c.Context(), c.Params("handle"))
	if err != nil {
		if errors.Is(err, talentnetwork.ErrNotFound) {
			return fiber.NewError(fiber.StatusNotFound, "not found")
		}
		return err
	}

	data, err := h.photos.Get(c.Context(), userID)
	if err != nil {
		// ErrNotStored (no headshot) and ErrStorageDisabled (storage unconfigured) both
		// collapse into the same 404 a non-member gets — never the 501 the OWNER's own
		// /me/photo/image route uses, which would tell a public caller storage is down
		// as a fact distinct from "this member has no photo". Anything ELSE (a DB or
		// blob-store fault) falls through to the ordinary error path instead of joining
		// that same silent 404: collapsing those too would make a real outage
		// indistinguishable from "no headshot" in every log and every alert, the same
		// error-swallowing photo.go's mapPhotoError deliberately avoids.
		if errors.Is(err, headshot.ErrNotStored) || errors.Is(err, headshot.ErrStorageDisabled) {
			return fiber.NewError(fiber.StatusNotFound, "not found")
		}
		return err
	}

	blurred, err := headshot.Blur(data)
	if err != nil {
		return err
	}

	c.Set(fiber.HeaderContentType, "image/jpeg")
	// Same reasoning as Get's Cache-Control: private, not public, so a shared cache
	// never outlives a member's decision to leave.
	c.Set("Cache-Control", "private, max-age=60")
	c.Set("X-Robots-Tag", "noindex")
	return c.Send(blurred)
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
