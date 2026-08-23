package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"sort"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/candidate/atscheck"
	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/dict/skilltag"
	"github.com/strelov1/freehire/internal/identity/userprofile"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/llm"
	"github.com/strelov1/freehire/internal/search/search"
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
func (h *resumeHandlers) GetATSReport(c *fiber.Ctx) error {
	userID, profile, err := h.atsContext(c)
	if err != nil {
		return err
	}
	report, _, hasCV, err := h.deterministicReport(c, userID, profile)
	if err != nil {
		return err
	}
	if !hasCV {
		return c.JSON(fiber.Map{"data": atsResponse{HasCV: false}})
	}
	if review := h.cachedReview(c, userID); review != nil {
		report.ApplyReview(review)
	}
	return c.JSON(fiber.Map{"data": atsResponse{HasCV: true, Report: report}})
}

// PostATSReport runs the optional LLM qualitative review over the caller's stored
// CV, caches it per user, and returns the report with it folded in. Best-effort: an
// unconfigured or failing LLM returns the deterministic report (200). Cookie-only,
// owner-scoped.
func (h *resumeHandlers) PostATSReport(c *fiber.Ctx) error {
	userID, profile, err := h.atsContext(c)
	if err != nil {
		return err
	}
	report, _, hasCV, err := h.deterministicReport(c, userID, profile)
	if err != nil {
		return err
	}
	if !hasCV {
		return c.JSON(fiber.Map{"data": atsResponse{HasCV: false}})
	}

	// The qualitative review reads the de-identified structure OF THE UPLOADED FILE, never
	// the raw CV — and deliberately not the experience bank. This report judges the
	// document, not the person: feeding it banked evidence would have it praise a CV for
	// experience that appears nowhere in the CV.
	candidate, ok := reviewableResume(h.resume, c, userID)
	if !ok {
		// No current structure to review: serve the deterministic report rather than
		// asking the model to judge a document it cannot see.
		return c.JSON(fiber.Map{"data": atsResponse{HasCV: true, Report: report}})
	}
	analyzer := h.atsAnalyzer.As(h.llm.bind(c.Context(), userID, llm.Feature(tagATSReview)))
	review, err := analyzer.Analyze(c.Context(), candidate)
	if err != nil {
		// Best-effort: log (never the CV text) and serve the deterministic report.
		log.Printf("atscheck: review failed for user %d: %v", userID, err)
		return c.JSON(fiber.Map{"data": atsResponse{HasCV: true, Report: report}})
	}
	if review != nil {
		if blob, err := json.Marshal(review); err == nil {
			if err := h.atsCache.SetUserATSAnalysis(c.Context(), db.SetUserATSAnalysisParams{
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

// atsContext resolves the authenticated caller, their profile (404 when none), and
// enforces that search is configured (503).
func (h *resumeHandlers) atsContext(c *fiber.Ctx) (int64, userprofile.Profile, error) {
	userID, err := requireUserID(c)
	if err != nil {
		return 0, userprofile.Profile{}, err
	}
	profile, err := h.userProfile.Get(c.Context(), userID)
	if err != nil {
		return 0, userprofile.Profile{}, profileError(err)
	}
	if h.facets == nil {
		return 0, userprofile.Profile{}, fiber.NewError(fiber.StatusServiceUnavailable, "search is not available")
	}
	return userID, profile, nil
}

// deterministicReport builds the live deterministic report from the stored CV and
// the selected role. hasCV is false (no error) when no CV is stored; cvText is
// returned for the LLM path.
func (h *resumeHandlers) deterministicReport(c *fiber.Ctx, userID int64, profile userprofile.Profile) (*atscheck.Report, string, bool, error) {
	cvText, ok, err := h.storedCVText(c, userID)
	if err != nil || !ok {
		return nil, "", ok, err
	}
	roleFilter := search.FilterFromValues(roleValues(c, profile))
	res, err := h.facets.FacetCounts(c.Context(), search.FacetParams{
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
func (h *resumeHandlers) cachedReview(c *fiber.Ctx, userID int64) *atscheck.Review {
	blob, err := h.atsCache.GetUserATSAnalysis(c.Context(), userID)
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
func (h *resumeHandlers) storedCVText(c *fiber.Ctx, userID int64) (string, bool, error) {
	if !h.resume.Enabled() {
		return "", false, nil
	}
	text, err := h.resume.Text(c.Context(), userID)
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

// reviewableResume reads the structure of the user's stored CV as its contact-free projection,
// reporting false when there is none, none current, or storage is unconfigured. The projection
// is what a model may see; returning it rather than a serialization means no caller can hand
// the contact-bearing structure to one.
//
// This is the FILE's structure, and it is what surfaces judging the document read. The fit
// chain reads matchHandlers.candidateProfile instead — a composition of the experience bank
// and this structure — because it judges the candidate rather than their CV.
func reviewableResume(resumeStore *resume.Store, c *fiber.Ctx, userID int64) (resumeextract.Professional, bool) {
	if resumeStore == nil || !resumeStore.Enabled() {
		return resumeextract.Professional{}, false
	}
	st, ok, err := resumeStore.Structured(c.Context(), userID)
	if err != nil || !ok {
		return resumeextract.Professional{}, false
	}
	return st.Professional(), true
}
