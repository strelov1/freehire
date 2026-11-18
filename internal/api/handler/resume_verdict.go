package handler

import (
	"context"
	"encoding/json"
	"maps"
	"net/url"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/candidate/cvsection"
	"github.com/strelov1/freehire/internal/identity/userprofile"
	"github.com/strelov1/freehire/internal/job/verdict"
	"github.com/strelov1/freehire/internal/search/search"
)

// GetResumeVerdict serves the market-coverage verdict for the caller's profile: how many of
// the role's open vacancies the profile's skills reach, and which missing skill unlocks the
// most. The role is the request's facet params (defaulting to the profile's specializations);
// the profile's skills are always the measured set, never a filter. No profile is a 404; 503
// when search is unconfigured.
func (h *resumeHandlers) GetResumeVerdict(ctx context.Context) (*apigen.GetResumeVerdictOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	profile, err := h.userProfile.Get(ctx, userID)
	if err != nil {
		return nil, profileError(err)
	}
	if h.facets == nil {
		return nil, fiber.NewError(fiber.StatusServiceUnavailable, "search is not available")
	}

	v, err := h.computeCoverage(ctx, userID, profile)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return &apigen.GetResumeVerdictOK{Data: raw}, nil
}

// computeCoverage builds the coverage verdict for the caller's profile: the covered count uses
// the profile's skills, the role-skill breakdown the CV's parsed declared/body/all sets.
func (h *resumeHandlers) computeCoverage(ctx context.Context, userID int64, profile userprofile.Profile) (verdict.Verdict, error) {
	roleFilter := search.FilterFromValues(roleValues(contractQuery(ctx), profile))
	declared, body, all := h.cvSkillSets(ctx, userID)
	return h.coverageFor(ctx, roleFilter, profile.Skills, declared, body, all)
}

// coverageFor runs the three facet queries behind the coverage verdict and folds them through
// verdict.Compute. A is the role's open-vacancy total and skill distribution; B is the
// "uncovered" set (the role filtered to vacancies listing none of coverageSkills), giving the
// covered count and per-skill unlock; C is the skill-bearing total. coverageSkills drives
// covered/uncovered; declared/body/all score the breakdown.
//
// Query A is split into coverageWithRole so a caller that already holds the role's skill
// distribution (the public roast) supplies it: two answers to the same question can differ
// while the index is being written.
func (h *resumeHandlers) coverageFor(ctx context.Context, roleFilter any, coverageSkills, declared, body, all []string) (verdict.Verdict, error) {
	role, err := h.facets.FacetCounts(ctx, search.FacetParams{
		Filter: roleFilter,
		Facets: []string{"skills"},
	})
	if err != nil {
		return verdict.Verdict{}, err
	}
	return h.coverageWithRole(ctx, roleFilter, role, coverageSkills, declared, body, all)
}

// coverageWithRole is coverageFor with query A already answered: it issues the uncovered
// (B) and skill-bearing-total (C) queries and computes the verdict.
func (h *resumeHandlers) coverageWithRole(ctx context.Context, roleFilter any, role search.FacetResult, coverageSkills, declared, body, all []string) (verdict.Verdict, error) {
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

// cvSkillSets parses the caller's stored CV into declared (Skills-section), body and union
// skill sets. Best-effort: with no CV or a read error it returns empty sets, so the breakdown
// degrades to all-missing rather than failing the verdict.
func (h *resumeHandlers) cvSkillSets(ctx context.Context, userID int64) (declared, body, all []string) {
	text, ok, err := h.storedCVText(ctx, userID)
	if err != nil || !ok {
		return nil, nil, nil
	}
	return cvsection.Parse(text)
}

// roleValues builds the coverage role's facet query: it strips the `skills` facet (the
// profile's skills are the measured set) and defaults `category` to the profile's
// specializations.
func roleValues(query url.Values, profile userprofile.Profile) url.Values {
	vals := maps.Clone(query)
	stripSkillParams(vals)
	if !hasNonEmpty(vals["category"]) {
		vals["category"] = profile.Specializations
	}
	return vals
}

// skillParams are the skills facet's query params, shared by the strip below and the coverage
// endpoint's report of what it discarded.
var skillParams = []string{"skills", "skills_exclude", "skills_mode"}

// stripSkillParams removes the skills facet params: the caller's skills are the measured set,
// never a filter that would narrow the market to jobs already listing them.
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
