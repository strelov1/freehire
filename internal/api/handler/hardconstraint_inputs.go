package handler

import (
	"context"
	"encoding/json"
	"time"

	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/candidate/hardconstraint"
	"github.com/strelov1/freehire/internal/candidate/matchanalysis"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/identity/userprofile"
	"github.com/strelov1/freehire/internal/job/jobfacts"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// capServedAnalysis attaches the caller's hard-constraint blockers to a fit analysis and
// clamps its overall score to the deterministic ceiling. Recomputed on every read so a
// dictionary change needs no cache stamp; the cache keeps the uncapped analysis. No-op on
// nil. Used on the GET path, which has no pre-computed blockers.
func (h *matchHandlers) capServedAnalysis(ctx context.Context, userID int64, job db.Job, analysis *matchanalysis.Analysis) {
	if analysis == nil {
		return
	}
	if h.userProfile == nil { // profile use case not wired (e.g. a minimal test app) → no blockers
		applyBlockers(analysis, nil)
		return
	}
	profile, _ := h.userProfile.Get(ctx, userID)
	applyBlockers(analysis, h.jobBlockers(ctx, userID, job, profile))
}

// applyBlockers attaches blockers to the served analysis and clamps its score. Split out
// so the POST path reuses the blockers it computed for the prompt. No-op on nil.
func applyBlockers(analysis *matchanalysis.Analysis, blockers []hardconstraint.Blocker) {
	if analysis == nil {
		return
	}
	if blockers == nil {
		blockers = []hardconstraint.Blocker{}
	}
	analysis.Blockers = blockers
	matchanalysis.ApplyCeiling(analysis, hardconstraint.OverallCap(blockers))
}

// jobBlockers evaluates the caller's hard constraints against a job. It degrades to no
// blockers (never an error) when the CV service is off or there is no structured CV.
// profile is the already-loaded caller profile. Advisory: nothing is hidden.
func (h *matchHandlers) jobBlockers(ctx context.Context, userID int64, job db.Job, profile userprofile.Profile) []hardconstraint.Blocker {
	if h.resume == nil || !h.resume.Enabled() {
		return nil
	}
	cv, ok, err := h.resume.Structured(ctx, userID)
	if err != nil || !ok {
		return nil
	}
	// The years shown on the candidate's profile: their own figure if given, else the dated roles.
	if owned, err := h.resume.CandidateOwned(ctx, userID); err == nil {
		owned.ApplyTotalYears(&cv, time.Now())
	} else {
		cv.TotalYears = cv.EffectiveYears(time.Now())
	}
	var loc userprofile.LocationPreferences
	if len(profile.LocationPreferences) > 0 {
		_ = json.Unmarshal(profile.LocationPreferences, &loc) // best-effort; empty loc simply skips geo checks
	}
	// Fall back to the CV-derived country when the caller asserted none. Best-effort: a
	// failing read leaves the country empty.
	var derived []string
	if geo, ok, err := h.resume.Geography(ctx, userID); err == nil && ok {
		derived = geo.Countries
	}
	jr, ev := buildHardConstraintInputs(job, cv.Professional(), loc, derived)
	return hardconstraint.Evaluate(jr, ev)
}

// buildHardConstraintInputs assembles the pure evaluator's inputs from a job row, the
// caller's contact-free CV projection and their location preferences. The projection, not
// the full structure, because the blockers' reasons go into the fit prompt, so the CV side
// takes the same typed seam as the candidate context. The job side reads the requirement
// columns, two compute-at-read jobfacts derivations and the enrichment visa flag.
// derivedCountries is the geography read off the CV; see candidateCountry.
func buildHardConstraintInputs(job db.Job, cv resumeextract.Professional, loc userprofile.LocationPreferences, derivedCountries []string) (hardconstraint.JobRequirements, hardconstraint.CVEvidence) {
	jr := hardconstraint.JobRequirements{
		ExperienceYearsMin:     pgconv.IntPtr(job.ExperienceYearsMin),
		EducationLevel:         job.EducationLevel,
		DegreeOptional:         jobfacts.DegreeOptional(job.Description),
		EnglishLevel:           job.EnglishLevel,
		VisaSponsorship:        jobVisaSponsorship(job.Enrichment),
		WorkMode:               job.WorkMode,
		Countries:              job.Countries,
		RequiredCertifications: jobfacts.RequiredCertifications(job.Description),
	}
	ev := hardconstraint.CVEvidence{
		TotalYears:     cv.TotalYears,
		Degrees:        degreeNames(cv.Education),
		Languages:      cv.Languages,
		Certifications: cv.Certifications,
		CountryCode:    candidateCountry(loc.Base.Country, derivedCountries),
		PrefersRemote:  prefersRemote(loc.WorkModes),
	}
	return jr, ev
}

// degreeNames pulls the non-empty degree strings out of the résumé's education entries.
func degreeNames(education []resumeextract.Education) []string {
	var out []string
	for _, e := range education {
		if e.Degree != "" {
			out = append(out, e.Degree)
		}
	}
	return out
}

// prefersRemote reports a strict remote preference: remote listed and neither onsite nor
// hybrid accepted, so an on-site posting is a genuine conflict.
func prefersRemote(workModes []string) bool {
	remote := false
	for _, m := range workModes {
		switch m {
		case "remote":
			remote = true
		case "onsite", "hybrid":
			return false
		}
	}
	return remote
}

// jobVisaSponsorship reads the visa_sponsorship flag out of the enrichment jsonb,
// returning nil when absent or unparseable (unknown).
func jobVisaSponsorship(raw json.RawMessage) *bool {
	if len(raw) == 0 {
		return nil
	}
	var e enrich.Enrichment
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil
	}
	return e.VisaSponsorship
}

// candidateCountry decides which country the evaluator judges the caller by: the one they
// ASSERTED (location_preferences.base.country), or only when none, the one DERIVED from
// their CV. Same shape as jobview.geoFacet: the candidate's own statement is the authority.
//
// A derivation naming more than one country counts as no answer: never guess, and an
// ambiguous derivation would produce a blocker with no evidence (never emit a false blocker).
func candidateCountry(asserted string, derived []string) string {
	if asserted != "" {
		return asserted
	}
	if len(derived) != 1 {
		return ""
	}
	return derived[0]
}
