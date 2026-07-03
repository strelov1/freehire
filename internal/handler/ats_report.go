package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"sort"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/atscheck"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/resume"
	"github.com/strelov1/freehire/internal/search"
	"github.com/strelov1/freehire/internal/skilltag"
)

// atsReviewStore reads/writes the per-user cached CV ATS review. *db.Queries
// satisfies it; a fake backs the DB-less handler tests.
type atsReviewStore interface {
	GetUserATSAnalysis(ctx context.Context, id int64) ([]byte, error)
	SetUserATSAnalysis(ctx context.Context, arg db.SetUserATSAnalysisParams) error
}

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

// GetATSReport serves the CV ATS-readiness report for one of the caller's profiles:
// the deterministic structural + keyword score merged with any cached LLM review.
// Cookie-only, owner-scoped (404); 503 when search is unconfigured; 200 with
// has_cv=false when no CV is stored.
func (a *API) GetATSReport(c *fiber.Ctx) error {
	userID, profile, err := a.atsContext(c)
	if err != nil {
		return err
	}
	report, _, hasCV, err := a.deterministicReport(c, userID, profile)
	if err != nil {
		return err
	}
	if !hasCV {
		return c.JSON(fiber.Map{"data": atsResponse{HasCV: false}})
	}
	if review := a.cachedReview(c, userID); review != nil {
		report.ApplyReview(review)
	}
	return c.JSON(fiber.Map{"data": atsResponse{HasCV: true, Report: report}})
}

// PostATSReport runs the optional LLM qualitative review over the caller's stored
// CV, caches it per user, and returns the report with it folded in. Best-effort: an
// unconfigured or failing LLM returns the deterministic report (200). Cookie-only,
// owner-scoped.
func (a *API) PostATSReport(c *fiber.Ctx) error {
	userID, profile, err := a.atsContext(c)
	if err != nil {
		return err
	}
	report, cvText, hasCV, err := a.deterministicReport(c, userID, profile)
	if err != nil {
		return err
	}
	if !hasCV {
		return c.JSON(fiber.Map{"data": atsResponse{HasCV: false}})
	}

	review, err := a.atsAnalyzer.Analyze(c.Context(), cvText)
	if err != nil {
		// Best-effort: log (never the CV text) and serve the deterministic report.
		log.Printf("atscheck: review failed for user %d: %v", userID, err)
		return c.JSON(fiber.Map{"data": atsResponse{HasCV: true, Report: report}})
	}
	if review != nil {
		if blob, err := json.Marshal(review); err == nil {
			if err := a.atsCache.SetUserATSAnalysis(c.Context(), db.SetUserATSAnalysisParams{
				ID:                userID,
				ResumeAtsAnalysis: blob,
			}); err != nil {
				log.Printf("atscheck: cache review for user %d: %v", userID, err)
			}
		}
		report.ApplyReview(review)
	}
	return c.JSON(fiber.Map{"data": atsResponse{HasCV: true, Report: report}})
}

// atsContext resolves the authenticated caller, the owner-scoped profile (404), and
// enforces that search is configured (503).
func (a *API) atsContext(c *fiber.Ctx) (int64, db.SearchProfile, error) {
	userID, err := requireUserID(c)
	if err != nil {
		return 0, db.SearchProfile{}, err
	}
	id, err := pathID(c)
	if err != nil {
		return 0, db.SearchProfile{}, err
	}
	profile, err := a.searchProfile.Get(c.Context(), userID, id)
	if err != nil {
		return 0, db.SearchProfile{}, searchProfileError(err)
	}
	if a.facets == nil {
		return 0, db.SearchProfile{}, fiber.NewError(fiber.StatusServiceUnavailable, "search is not available")
	}
	return userID, profile, nil
}

// deterministicReport builds the live deterministic report from the stored CV and
// the selected role. hasCV is false (no error) when no CV is stored; cvText is
// returned for the LLM path.
func (a *API) deterministicReport(c *fiber.Ctx, userID int64, profile db.SearchProfile) (*atscheck.Report, string, bool, error) {
	cvText, ok, err := a.storedCVText(c, userID)
	if err != nil || !ok {
		return nil, "", ok, err
	}
	roleFilter := search.FilterFromValues(roleValues(c, profile))
	res, err := a.facets.FacetCounts(c.Context(), search.FacetParams{
		Filter: roleFilter,
		Facets: []string{"skills"},
	})
	if err != nil {
		return nil, "", true, err
	}
	cvSkills := skilltag.Parse(cvText, skilltag.WithResumeAcronyms())
	report := atscheck.Score(cvText, cvSkills, topRoleSkills(res.Facets["skills"], atsRoleTopN))
	return &report, cvText, true, nil
}

// cachedReview reads the caller's cached LLM review, or nil when none/invalid.
func (a *API) cachedReview(c *fiber.Ctx, userID int64) *atscheck.Review {
	blob, err := a.atsCache.GetUserATSAnalysis(c.Context(), userID)
	if err != nil || len(blob) == 0 {
		return nil
	}
	var rv atscheck.Review
	if err := json.Unmarshal(blob, &rv); err != nil {
		return nil
	}
	return &rv
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
