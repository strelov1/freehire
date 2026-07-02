package handler

import (
	"net/url"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/search"
	"github.com/strelov1/freehire/internal/verdict"
)

// GetResumeVerdict serves the market-coverage verdict for one of the caller's
// profiles: how many of the selected role's open vacancies the profile's skills
// reach, and which missing skill unlocks the most new vacancies. The role is the
// request's facet params (defaulting to the profile's specializations when no
// category is given); the profile's skills are always the measured set, never a
// filter. Cookie-only, owner-scoped (missing/non-owned profile → 404); 503 when
// search is unconfigured.
func (a *API) GetResumeVerdict(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := pathID(c)
	if err != nil {
		return err
	}

	profile, err := a.searchProfile.Get(c.Context(), userID, id)
	if err != nil {
		return searchProfileError(err)
	}
	if a.facets == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "search is not available")
	}

	v, err := a.computeCoverage(c, profile)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": v})
}

// computeCoverage runs the two facet queries behind the coverage verdict and folds
// them through the pure verdict.Compute. Query A is the role's open-vacancy total;
// query B is the "uncovered" set — the same role filtered to vacancies listing
// none of the profile's skills — whose total and skill distribution give the
// covered count and the per-skill new-vacancy unlock.
func (a *API) computeCoverage(c *fiber.Ctx, profile db.SearchProfile) (verdict.Verdict, error) {
	roleFilter := search.FilterFromValues(roleValues(c, profile))

	role, err := a.facets.FacetCounts(c.Context(), search.FacetParams{Filter: roleFilter})
	if err != nil {
		return verdict.Verdict{}, err
	}
	uncovered, err := a.facets.FacetCounts(c.Context(), search.FacetParams{
		Filter: search.AndNotSkills(roleFilter, profile.Skills),
		Facets: []string{"skills"},
	})
	if err != nil {
		return verdict.Verdict{}, err
	}
	return verdict.Compute(role.Total, uncovered.Total, uncovered.Facets["skills"]), nil
}

// roleValues builds the facet query for the coverage role from the request. It
// strips the `skills` facet (the profile's skills are the measured set, not a role
// filter) and defaults `category` to the profile's specializations when the caller
// selected no category — so an unfiltered verdict scores the profile's own role.
func roleValues(c *fiber.Ctx, profile db.SearchProfile) url.Values {
	vals, _ := url.ParseQuery(string(c.Request().URI().QueryString()))
	delete(vals, "skills")
	delete(vals, "skills_exclude")
	delete(vals, "skills_mode")
	if !hasNonEmpty(vals["category"]) {
		vals["category"] = profile.Specializations
	}
	return vals
}

// hasNonEmpty reports whether a query-param slice carries at least one non-empty
// value, so a bare `?category=` counts as "no category selected".
func hasNonEmpty(vals []string) bool {
	for _, v := range vals {
		if v != "" {
			return true
		}
	}
	return false
}
