package handler

import (
	"errors"
	"sort"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/atscheck"
	"github.com/strelov1/freehire/internal/resume"
	"github.com/strelov1/freehire/internal/search"
	"github.com/strelov1/freehire/internal/skilltag"
)

// atsRoleTopN is how many of the role's most in-demand skills the CV keyword-match
// is scored against.
const atsRoleTopN = 20

// atsResponse is the wire shape for the CV ATS report. HasCV is false when the
// caller has no stored CV (storage off or none uploaded) — the SPA then prompts an
// upload instead of showing an empty report; Report is nil in that case.
type atsResponse struct {
	HasCV  bool             `json:"has_cv"`
	Report *atscheck.Report `json:"report"`
}

// GetATSReport serves the deterministic CV ATS-readiness report for one of the
// caller's profiles: structural checks over the stored CV text plus a keyword-match
// against the selected role's top skills. The role is the request's facet params
// (same as the verdict). Cookie-only, owner-scoped (missing/non-owned → 404); 503
// when search is unconfigured; 200 with has_cv=false when no CV is stored.
func (a *API) GetATSReport(c *fiber.Ctx) error {
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

	cvText, ok, err := a.storedCVText(c, userID)
	if err != nil {
		return err
	}
	if !ok {
		return c.JSON(fiber.Map{"data": atsResponse{HasCV: false}})
	}

	roleFilter := search.FilterFromValues(roleValues(c, profile))
	res, err := a.facets.FacetCounts(c.Context(), search.FacetParams{
		Filter: roleFilter,
		Facets: []string{"skills"},
	})
	if err != nil {
		return err
	}
	cvSkills := skilltag.Parse(cvText, skilltag.WithResumeAcronyms())
	report := atscheck.Score(cvText, cvSkills, topRoleSkills(res.Facets["skills"], atsRoleTopN))
	return c.JSON(fiber.Map{"data": atsResponse{HasCV: true, Report: &report}})
}

// storedCVText returns the caller's stored CV text; ok=false (no error) when CV
// storage is disabled or the caller has none stored.
func (a *API) storedCVText(c *fiber.Ctx, userID int64) (string, bool, error) {
	if !a.resume.Enabled() {
		return "", false, nil
	}
	text, err := a.resume.Text(c.Context(), userID)
	if errors.Is(err, resume.ErrNotStored) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return text, true, nil
}

// topRoleSkills ranks a skills facet distribution by demand (count desc, slug asc)
// and returns the top n slugs — the role's most in-demand skills.
func topRoleSkills(facet map[string]int64, n int) []string {
	type skillCount struct {
		slug  string
		count int64
	}
	ranked := make([]skillCount, 0, len(facet))
	for slug, count := range facet {
		ranked = append(ranked, skillCount{slug, count})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].count != ranked[j].count {
			return ranked[i].count > ranked[j].count
		}
		return ranked[i].slug < ranked[j].slug
	})
	if len(ranked) > n {
		ranked = ranked[:n]
	}
	out := make([]string, len(ranked))
	for i, r := range ranked {
		out[i] = r.slug
	}
	return out
}
