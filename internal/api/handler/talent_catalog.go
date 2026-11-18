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

// talentCatalogHandlers serves the PUBLIC Talent Network catalogue: the filtered member list,
// one member's card, and a heavily blurred photo when uploaded. All are unauthenticated by
// design (signed-out visitors are the audience), so no route takes auth middleware.
type talentCatalogHandlers struct {
	catalogue *talentnetwork.Catalogue
	photos    *headshot.Store
}

func newTalentCatalogHandlers(catalogue *talentnetwork.Catalogue, photos *headshot.Store) *talentCatalogHandlers {
	return &talentCatalogHandlers{catalogue: catalogue, photos: photos}
}

func (h *talentCatalogHandlers) register(api fiber.Router, contract fiber.Handler) {
	// Contract: typespec/talent.tsp. Route order does not matter; the generated router matches /talent/facets first.
	api.Get("/talent", contract)
	api.Get("/talent/facets", contract)
	api.Get("/talent/:handle", contract)
	api.Get("/talent/:handle/photo", contract)
}

// GetTalentFacets serves how many members stand behind each filter value, for the caller's
// current filter. Counts make the open vocabularies (thousands of skills) usable. Same query
// vocabulary and unread-param report as the list.
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
			// One 404 for every way of being absent: distinguishing them would reveal whether an account exists.
			return nil, fiber.NewError(fiber.StatusNotFound, "not found")
		}
		return nil, err
	}

	// `private`, not `public`: `public` lets a SHARED cache (CDN, corporate proxy) re-serve the
	// card, and leaving must take effect on the next request.
	return &apigen.GetTalentMemberOKHeaders{
		CacheControl: talentCacheControl,
		XRobotsTag:   "noindex",
		Response:     apigen.GetTalentMemberOK{Data: talentMemberOut(member)},
	}, nil
}

// talentCacheControl is the one caching rule both per-member routes answer with.
const talentCacheControl = "private, max-age=60"

// GetTalentPhoto serves a member's photo, blurred to be unrecognisable; no public route serves
// the stored original. Not a member, no headshot, or storage unavailable all answer with the
// same 404 as GetTalentMember, so a caller cannot tell "not a member" from "no photo".
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
		// ErrNotStored and ErrStorageDisabled collapse into the non-member 404, not the 501 the
		// owner's /me/photo/image uses, which would reveal storage is down. Any other fault
		// (DB, blob store) keeps the ordinary error path so an outage stays visible in logs and alerts.
		if errors.Is(err, headshot.ErrNotStored) || errors.Is(err, headshot.ErrStorageDisabled) {
			return nil, fiber.NewError(fiber.StatusNotFound, "not found")
		}
		return nil, err
	}

	blurred, err := headshot.Blur(data)
	if err != nil {
		return nil, err
	}
	// Private for the same reason as GetTalentMember.
	return &apigen.GetTalentPhotoOKHeaders{
		CacheControl: talentCacheControl,
		XRobotsTag:   "noindex",
		Response:     apigen.GetTalentPhotoOK{Data: bytes.NewReader(blurred)},
	}, nil
}

// ignoredTalentParams reports what this listing did not read: unknown params plus recognised
// ones it could not READ (`min_years=lots`). One report, because the cap and ordering must
// cover both; appending after SortAndCap would exceed the bound.
//
// The vocabulary is talentnetwork's, not search's: passing these facets to search.UnknownParams
// as `alsoKnown` would accept every job-search facet here and report none.
func ignoredTalentParams(vals url.Values, unreadable []string) []search.UnknownParam {
	ignored := search.UnknownParamsAgainst(vals, talentnetwork.KnownParams())
	for _, param := range unreadable {
		ignored = append(ignored, search.UnknownParam{Param: param})
	}
	return search.SortAndCap(ignored)
}

// talentMemberOut is a catalogue member in its wire shape; fields empty on the card are left unset.
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
