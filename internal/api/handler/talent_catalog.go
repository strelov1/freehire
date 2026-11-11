package handler

import (
	"bytes"
	"context"
	"errors"
	"net/url"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/candidate/headshot"
	"github.com/strelov1/freehire/internal/candidate/perioddate"
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

func (h *talentCatalogHandlers) register(api fiber.Router, contract fiber.Handler) {
	// The handlers below implement the contract (typespec/talent.tsp). The generated
	// router matches /talent/facets before /talent/{handle} on its own, so the order here
	// no longer matters.
	api.Get("/talent", contract)
	api.Get("/talent/facets", contract)
	api.Get("/talent/:handle", contract)
	api.Get("/talent/:handle/photo", contract)
}

// GetTalentFacets serves how many members stand behind each filter value, for the
// filter the caller currently has.
//
// It is what makes the open vocabularies usable: skills are thousands of canonicals, and
// a control offering them without counts cannot tell a value nobody carries from one
// whose members have all left. Same query vocabulary as the list, same unread-param
// report.
func (h *talentCatalogHandlers) GetTalentFacets(ctx context.Context, _ apigen.GetTalentFacetsParams) (*apigen.GetTalentFacetsOK, error) {
	vals := contractQuery(ctx)
	q, unreadable := talentnetwork.QueryFromValues(vals)

	counts, err := h.catalogue.Facets(ctx, q)
	if err != nil {
		return nil, err
	}
	out := &apigen.GetTalentFacetsOK{Data: talentFacetsOut(counts)}
	if ignored := contractIgnoredParams(ignoredTalentParams(vals, unreadable)); ignored != nil {
		out.Meta = apigen.NewOptIgnoredMeta(apigen.IgnoredMeta{IgnoredParams: ignored})
	}
	return out, nil
}

// ListTalent serves one filtered, ordered page of the catalogue.
func (h *talentCatalogHandlers) ListTalent(ctx context.Context, _ apigen.ListTalentParams) (*apigen.ListTalentOK, error) {
	vals := contractQuery(ctx)
	q, unreadable := talentnetwork.QueryFromValues(vals)

	page, err := h.catalogue.List(ctx, q)
	if err != nil {
		return nil, err
	}
	members := make([]apigen.TalentMember, len(page.Members))
	for i, m := range page.Members {
		members[i] = talentMemberOut(m)
	}
	return &apigen.ListTalentOK{Data: members, Meta: apigen.ListMeta{
		Total:         int64(page.Total),
		Limit:         int64(q.Limit),
		Offset:        int64(q.Offset),
		IgnoredParams: contractIgnoredParams(ignoredTalentParams(vals, unreadable)),
	}}, nil
}

// GetTalentMember serves one member's card by the handle in their public URL.
func (h *talentCatalogHandlers) GetTalentMember(ctx context.Context, params apigen.GetTalentMemberParams) (*apigen.GetTalentMemberOKHeaders, error) {
	member, err := h.catalogue.ByHandle(ctx, params.Handle)
	if err != nil {
		if errors.Is(err, talentnetwork.ErrNotFound) {
			// The one 404 every way of being absent produces — gone, stale, never
			// existed, not even a handle. Distinguishing them would turn this route
			// into a way of asking whether an account exists.
			return nil, fiber.NewError(fiber.StatusNotFound, "not found")
		}
		return nil, err
	}

	// A card is a person, and a cache of one outlives their decision to leave.
	//
	// `private`, not `public`, even though the response carries no session and no
	// personal data in the ordinary sense: `public` is the directive that authorises a
	// SHARED cache — a CDN, a corporate proxy — to hold and re-serve it. This route
	// promises that leaving takes effect on the next request, and an intermediary
	// holding a departed member's card for a minute is exactly that promise broken by
	// somebody we cannot ask to stop.
	return &apigen.GetTalentMemberOKHeaders{
		CacheControl: talentCacheControl,
		XRobotsTag:   "noindex",
		Response:     apigen.GetTalentMemberOK{Data: talentMemberOut(member)},
	}, nil
}

// talentCacheControl is the one caching rule both per-member routes answer with.
const talentCacheControl = "private, max-age=60"

// GetTalentPhoto serves a member's photo, blurred to the point of being unrecognisable —
// never the stored original, which no public route ever serves. Every reason the route
// might not have an image to serve — the handle is not a current member, the member has
// no headshot uploaded, or headshot storage is unavailable — answers identically, the
// same 404 GetTalentMember gives a handle nobody holds: a caller must not be able to tell
// "not a member" apart from "a member with no photo" by the shape of the response.
func (h *talentCatalogHandlers) GetTalentPhoto(ctx context.Context, params apigen.GetTalentPhotoParams) (*apigen.GetTalentPhotoOKHeaders, error) {
	userID, err := h.catalogue.HeadshotOwner(ctx, params.Handle)
	if err != nil {
		if errors.Is(err, talentnetwork.ErrNotFound) {
			return nil, fiber.NewError(fiber.StatusNotFound, "not found")
		}
		return nil, err
	}

	data, err := h.photos.Get(ctx, userID)
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
			return nil, fiber.NewError(fiber.StatusNotFound, "not found")
		}
		return nil, err
	}

	blurred, err := headshot.Blur(data)
	if err != nil {
		return nil, err
	}
	// Same reasoning as GetTalentMember's Cache-Control: private, so a shared cache never
	// outlives a member's decision to leave.
	return &apigen.GetTalentPhotoOKHeaders{
		CacheControl: talentCacheControl,
		XRobotsTag:   "noindex",
		Response:     apigen.GetTalentPhotoOK{Data: bytes.NewReader(blurred)},
	}, nil
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

// talentMemberOut is a catalogue member in its wire shape. Every field the card's JSON
// tags omit when empty is left unset here for the same reason, so the answer keeps the
// shape it had before the contract.
func talentMemberOut(m talentnetwork.CatalogueMember) apigen.TalentMember {
	return apigen.TalentMember{
		Handle:          m.Handle,
		Card:            talentCardOut(m.Card),
		Timezone:        optString(m.Timezone),
		TimezoneRegion:  optString(m.TimezoneRegion),
		Cities:          m.Cities,
		Specializations: m.Specializations,
		UpdatedAt:       m.UpdatedAt,
		HasPhoto:        m.HasPhoto,
	}
}

func talentCardOut(c talentnetwork.CandidateCard) apigen.TalentCard {
	out := apigen.TalentCard{
		Seniority:      optString(c.Seniority),
		Category:       optString(c.Category),
		Skills:         c.Skills,
		Roles:          make([]apigen.TalentRole, len(c.Roles)),
		Certifications: nilIfEmpty(c.Certifications),
	}
	if c.TotalYears != 0 {
		out.TotalYears = apigen.NewOptInt32(int32(c.TotalYears)) //nolint:gosec // years of a career
	}
	for i, r := range c.Roles {
		out.Roles[i] = apigen.TalentRole{
			Title:     optString(r.Title),
			Seniority: optString(r.Seniority),
			Category:  optString(r.Category),
			Start:     optPeriodDate(r.Start),
			End:       optPeriodDate(r.End),
			Stack:     nilIfEmpty(r.Stack),
		}
		if r.Current {
			out.Roles[i].Current = apigen.NewOptBool(true)
		}
	}
	for _, e := range c.Education {
		out.Education = append(out.Education, apigen.TalentEducation{Level: optString(e.Level), Year: optPeriodDate(e.Year)})
	}
	return out
}

func optPeriodDate(d *perioddate.PeriodDate) apigen.OptPeriodDate {
	if d == nil {
		return apigen.OptPeriodDate{}
	}
	out := apigen.PeriodDate{Year: int32(d.Year)} //nolint:gosec // a calendar year
	if d.Month != 0 {
		out.Month = apigen.NewOptInt32(int32(d.Month)) //nolint:gosec // 1..12
	}
	return apigen.NewOptPeriodDate(out)
}

func talentFacetsOut(c talentnetwork.Counts) apigen.TalentFacets {
	out := apigen.TalentFacets{
		Total:  int32(c.Total), //nolint:gosec // a membership count
		Facets: make(apigen.TalentFacetsFacets, len(c.Facets)),
		Stats:  make(apigen.TalentFacetsStats, len(c.Stats)),
	}
	for facet, values := range c.Facets {
		counts := make(apigen.TalentFacetsFacetsItem, len(values))
		for v, n := range values {
			counts[v] = int32(n) //nolint:gosec // a membership count
		}
		out.Facets[facet] = counts
	}
	for facet, r := range c.Stats {
		out.Stats[facet] = apigen.FacetRange{Min: int32(r.Min), Max: int32(r.Max)} //nolint:gosec // years
	}
	return out
}
