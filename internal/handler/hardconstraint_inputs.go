package handler

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/enrich"
	"github.com/strelov1/freehire/internal/hardconstraint"
	"github.com/strelov1/freehire/internal/jobfacts"
	"github.com/strelov1/freehire/internal/matchanalysis"
	"github.com/strelov1/freehire/internal/resumeextract"
	"github.com/strelov1/freehire/internal/userprofile"
)

// capServedAnalysis attaches the caller's hard-constraint blockers to a fit analysis
// and clamps its overall score to the deterministic ceiling. Recomputed on every read
// so a dictionary change takes effect with no cache stamp; the uncapped analysis stays
// in the cache and only the served copy is capped. No-op on a nil analysis. Used on the
// GET path, which has no pre-computed blockers.
func (a *API) capServedAnalysis(ctx context.Context, userID int64, job db.Job, analysis *matchanalysis.Analysis) {
	if analysis == nil {
		return
	}
	if a.userProfile == nil { // profile use case not wired (e.g. a minimal test app) → no blockers
		applyBlockers(analysis, nil)
		return
	}
	profile, _ := a.userProfile.Get(ctx, userID)
	applyBlockers(analysis, a.jobBlockers(ctx, userID, job, profile))
}

// applyBlockers attaches blockers to the served analysis and clamps its score to the
// deterministic ceiling. Split out so the POST path can reuse the blockers it already
// computed for the prompt instead of evaluating them twice. No-op on a nil analysis.
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

// jobBlockers evaluates the caller's hard constraints against a job. It degrades to
// no blockers (never an error) when the résumé service is off or the caller has no
// structured résumé — the profile-match bar then shows skill coverage only. profile
// is the already-loaded caller profile (its location preferences drive the geo/work
// checks). Advisory: the caller decides what to do, nothing is hidden.
func (a *API) jobBlockers(ctx context.Context, userID int64, job db.Job, profile userprofile.Profile) []hardconstraint.Blocker {
	if a.resume == nil || !a.resume.Enabled() {
		return nil
	}
	cv, ok, err := a.resume.Structured(ctx, userID)
	if err != nil || !ok {
		return nil
	}
	var loc userprofile.LocationPreferences
	if len(profile.LocationPreferences) > 0 {
		_ = json.Unmarshal(profile.LocationPreferences, &loc) // best-effort; empty loc simply skips geo checks
	}
	jr, ev := buildHardConstraintInputs(job, cv, loc)
	return hardconstraint.Evaluate(jr, ev)
}

// buildHardConstraintInputs assembles the pure evaluator's inputs from a job row,
// the caller's structured résumé, and their location preferences. The job side reads
// the deterministic requirement columns plus two compute-at-read jobfacts derivations
// (required certifications and degree-optional); visa sponsorship comes from the
// enrichment jsonb. The CV side reads the structured résumé and the profile's base
// country / remote preference. Pure so it is unit-testable.
func buildHardConstraintInputs(job db.Job, cv resumeextract.Structured, loc userprofile.LocationPreferences) (hardconstraint.JobRequirements, hardconstraint.CVEvidence) {
	jr := hardconstraint.JobRequirements{
		ExperienceYearsMin:     int4Ptr(job.ExperienceYearsMin),
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
		CountryCode:    loc.Base.Country,
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

// prefersRemote reports a strict remote preference: the caller lists remote and does
// NOT also accept onsite or hybrid, so an on-site posting is a genuine conflict.
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

// int4Ptr converts a pgtype.Int4 to an optional int, mapping SQL NULL to nil.
func int4Ptr(v pgtype.Int4) *int {
	if !v.Valid {
		return nil
	}
	n := int(v.Int32)
	return &n
}
