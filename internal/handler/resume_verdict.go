package handler

import (
	"encoding/json"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/search"
	"github.com/strelov1/freehire/internal/verdict"
)

// storedAnalysis is the derived résumé-analysis blob persisted on a profile
// (search_profiles.resume_analysis JSONB): the coherence score, per-gap advice, and
// when it was produced. The raw résumé text is deliberately never part of it.
type storedAnalysis struct {
	Coherence  int               `json:"coherence"`
	Advice     map[string]string `json:"advice"`
	AnalyzedAt string            `json:"analyzed_at"`
}

// GetResumeVerdict serves the résumé verdict for one of the caller's profiles: the
// deterministic market comparison computed live from the profile's skills +
// specializations, merged with any previously stored coherence/advice. Cookie-only,
// owner-scoped (missing/non-owned profile → 404); 503 when search is unconfigured.
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

	v, err := a.computeVerdict(c, profile)
	if err != nil {
		return err
	}
	applyStoredAnalysis(&v, profile.ResumeAnalysis)
	return c.JSON(fiber.Map{"data": v})
}

// ResumeVerdict analyzes an uploaded résumé (PDF multipart "file" or JSON {text})
// against one of the caller's profiles: it computes the deterministic verdict, asks the
// LLM for a coherence score + gap advice over the résumé text, persists ONLY that
// derived analysis (never the text — the privacy invariant from resume.go), and returns
// the full verdict. Best-effort AI: an unconfigured or failing model degrades to the
// deterministic verdict (still 200). Cookie-only, owner-scoped.
func (a *API) ResumeVerdict(c *fiber.Ctx) error {
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

	text, err := resumeText(c)
	if err != nil {
		return err
	}
	if strings.TrimSpace(text) == "" {
		return fiber.NewError(fiber.StatusBadRequest, "resume is empty")
	}

	v, err := a.computeVerdict(c, profile)
	if err != nil {
		return err
	}

	analysis, err := a.verdictAnalyzer.Analyze(c.Context(), text, v.MustHaveGaps())
	if err != nil {
		// Best-effort: log the error (never the résumé text) and serve the deterministic
		// verdict. The AI layer is an enhancement, not a gate.
		log.Printf("verdict: résumé analysis failed for profile %d: %v", id, err)
		return c.JSON(fiber.Map{"data": v})
	}
	if analysis != nil {
		blob, err := json.Marshal(storedAnalysis{
			Coherence:  analysis.Coherence,
			Advice:     analysis.Advice,
			AnalyzedAt: time.Now().UTC().Format(time.RFC3339),
		})
		if err != nil {
			return err
		}
		if err := a.searchProfile.SetResumeAnalysis(c.Context(), userID, id, blob); err != nil {
			return searchProfileError(err)
		}
		applyStoredAnalysis(&v, blob)
	}
	return c.JSON(fiber.Map{"data": v})
}

// computeVerdict builds the deterministic verdict from a profile's skills against the
// market facet distribution for its specialization(s). 503 when search is unconfigured
// — the market data is the verdict's one hard dependency.
func (a *API) computeVerdict(c *fiber.Ctx, profile db.SearchProfile) (verdict.Verdict, error) {
	if a.facets == nil {
		return verdict.Verdict{}, fiber.NewError(fiber.StatusServiceUnavailable, "search is not available")
	}
	filter := search.FilterFromValues(url.Values{"category": profile.Specializations})
	res, err := a.facets.FacetCounts(c.Context(), search.FacetParams{
		Filter: filter,
		Facets: []string{"skills"},
	})
	if err != nil {
		return verdict.Verdict{}, err
	}
	market := verdict.MarketSkills{Counts: res.Facets["skills"], Total: res.Total}
	return verdict.Compute(market, profile.Skills), nil
}

// applyStoredAnalysis merges a persisted analysis blob into a verdict: the coherence
// score, the analysis timestamp, and advice attached to matching skill rows. A nil,
// empty, or unparseable blob leaves the verdict deterministic-only.
func applyStoredAnalysis(v *verdict.Verdict, blob json.RawMessage) {
	if len(blob) == 0 {
		return
	}
	var stored storedAnalysis
	if err := json.Unmarshal(blob, &stored); err != nil {
		return
	}
	coherence := stored.Coherence
	v.Coherence = &coherence
	if stored.AnalyzedAt != "" {
		at := stored.AnalyzedAt
		v.AnalyzedAt = &at
	}
	for i := range v.Skills {
		if adv, ok := stored.Advice[v.Skills[i].Name]; ok && adv != "" {
			text := adv
			v.Skills[i].Advice = &text
		}
	}
}
