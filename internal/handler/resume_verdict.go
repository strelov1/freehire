package handler

import (
	"context"
	"net/url"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/cvsection"
	"github.com/strelov1/freehire/internal/search"
	"github.com/strelov1/freehire/internal/userprofile"
	"github.com/strelov1/freehire/internal/verdict"
)

// GetResumeVerdict serves the market-coverage verdict for the caller's profile:
// how many of the selected role's open vacancies the profile's skills reach, and
// which missing skill unlocks the most new vacancies. The role is the request's
// facet params (defaulting to the profile's specializations when no category is
// given); the profile's skills are always the measured set, never a filter.
// Cookie-only, session-scoped (no profile → 404); 503 when search is unconfigured.
func (h *resumeHandlers) GetResumeVerdict(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	profile, err := h.userProfile.Get(c.Context(), userID)
	if err != nil {
		return profileError(err)
	}
	if h.facets == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "search is not available")
	}

	v, err := h.computeCoverage(c, userID, profile)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": v})
}

// computeCoverage builds the coverage verdict for the caller's profile: the role
// filter is the request facets (defaulting category to the profile), the covered
// count is measured against the profile's structured skills, and the role-skill
// breakdown is scored against the CV's parsed declared/body/all sets.
func (h *resumeHandlers) computeCoverage(c *fiber.Ctx, userID int64, profile userprofile.Profile) (verdict.Verdict, error) {
	roleFilter := search.FilterFromValues(roleValues(c, profile))
	declared, body, all := h.cvSkillSets(c, userID)
	return h.coverageFor(c.Context(), roleFilter, profile.Skills, declared, body, all)
}

// coverageFor runs the three facet queries behind the coverage verdict for a role
// filter and folds them through the pure verdict.Compute. Query A is the role's
// open-vacancy total (plus its full skill distribution, ranked into the
// breakdown); query B is the "uncovered" set — the same role filtered to vacancies
// listing none of coverageSkills — whose total and skill distribution give the
// covered count and the per-skill new-vacancy unlock; query C is the skill-bearing
// total (see below). coverageSkills drives covered/uncovered; declared/body/all
// score the role-skill breakdown — the two sets differ for the CV verdict (profile
// skills vs parsed CV) and coincide for a stateless skill list.
func (h *resumeHandlers) coverageFor(ctx context.Context, roleFilter any, coverageSkills, declared, body, all []string) (verdict.Verdict, error) {
	role, err := h.facets.FacetCounts(ctx, search.FacetParams{
		Filter: roleFilter,
		Facets: []string{"skills"},
	})
	if err != nil {
		return verdict.Verdict{}, err
	}
	uncovered, err := h.facets.FacetCounts(ctx, search.FacetParams{
		Filter: search.AndNotSkills(roleFilter, coverageSkills),
		Facets: []string{"skills"},
	})
	if err != nil {
		return verdict.Verdict{}, err
	}
	// Skill-bearing total: the role's vacancies that list at least one tagged skill.
	// Skill frequency (and the must-have flag) is measured against this, not the raw
	// role total, so postings the tagger left skill-less don't deflate frequencies.
	skilled, err := h.facets.FacetCounts(ctx, search.FacetParams{
		Filter: search.AndSkillsPresent(roleFilter),
	})
	if err != nil {
		return verdict.Verdict{}, err
	}
	return verdict.Compute(verdict.Input{
		Total:           role.Total,
		SkilledTotal:    skilled.Total,
		UncoveredTotal:  uncovered.Total,
		UncoveredSkills: uncovered.Facets["skills"],
		RoleSkills:      role.Facets["skills"],
		Declared:        declared,
		Body:            body,
		All:             all,
	}), nil
}

// cvSkillSets parses the caller's stored CV into its declared (Skills-section), body,
// and union skill sets for the role breakdown and bundle coverage. Best-effort: with
// no CV stored (or a read error) it returns empty sets so the breakdown degrades to
// all-missing rather than failing the verdict.
func (h *resumeHandlers) cvSkillSets(c *fiber.Ctx, userID int64) (declared, body, all []string) {
	text, ok, err := h.storedCVText(c, userID)
	if err != nil || !ok {
		return nil, nil, nil
	}
	return cvsection.Parse(text)
}

// roleValues builds the facet query for the coverage role from the request. It
// strips the `skills` facet (the profile's skills are the measured set, not a role
// filter) and defaults `category` to the profile's specializations when the caller
// selected no category — so an unfiltered verdict scores the profile's own role.
func roleValues(c *fiber.Ctx, profile userprofile.Profile) url.Values {
	vals := queryValues(c)
	stripSkillParams(vals)
	if !hasNonEmpty(vals["category"]) {
		vals["category"] = profile.Specializations
	}
	return vals
}

// skillParams are the skills facet's query params (bare + _exclude/_mode). Named
// once because two places need the same list: the strip below, and the coverage
// endpoint's report of what it discarded.
var skillParams = []string{"skills", "skills_exclude", "skills_mode"}

// stripSkillParams removes the skills facet params from a query set. In the
// coverage endpoints the caller's skills are the measured set, never a market
// filter that would narrow the market to jobs already listing them.
func stripSkillParams(vals url.Values) {
	for _, param := range skillParams {
		delete(vals, param)
	}
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
