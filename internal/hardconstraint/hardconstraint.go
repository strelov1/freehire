// Package hardconstraint deterministically checks a job's structured requirements
// against a caller's structured résumé across six categories and reports typed
// blockers. It is pure (no I/O, no LLM), the same dict-only discipline as
// internal/jobmatch and internal/classify: a category is judged only when BOTH
// sides carry data, so a missing field is skipped, never a false blocker.
//
// The blockers are advisory. They surface on the profile-match bar, cap the
// server-owned overall_score in the LLM fit analysis (a ceiling the model cannot
// exceed), and feed anti-hallucination guardrails into the tailor — but they
// never hide or downrank a job.
package hardconstraint

import (
	"fmt"
	"strings"

	"github.com/strelov1/freehire/internal/hardconstraint/credentials"
)

// tier pins each category's severity and its score-cap ceiling. A lower cap is a
// harder blocker; the caller takes the minimum cap over the unmet blockers.
var tier = map[BlockerCategory]struct {
	severity BlockerSeverity
	scoreCap int
}{
	CategoryWorkAuth:         {SeverityHard, 50},
	CategoryCertification:    {SeverityHard, 60},
	CategoryEducation:        {SeverityMedium, 65},
	CategoryExperience:       {SeverityMedium, 65},
	CategoryLanguage:         {SeveritySoft, 70},
	CategoryLocationWorkMode: {SeveritySoft, 75},
}

// JobRequirements is the job side, read straight from the enrichment columns
// (plus required_certifications from the enrichment jsonb). A nil/zero/empty field
// means "no such requirement" and its category is skipped.
type JobRequirements struct {
	ExperienceYearsMin     *int
	EducationLevel         string   // "", none, bachelor, master, phd
	DegreeOptional         bool     // posting offers "or equivalent experience" → skip education blocker
	EnglishLevel           string   // "" or a CEFR/level string
	VisaSponsorship        *bool    // nil = unknown
	WorkMode               string   // onsite, hybrid, remote, ""
	Countries              []string // ISO-3166 alpha-2 codes the job is bound to
	RequiredCertifications []string // canonical credential slugs
}

// CVEvidence is the résumé side, assembled from the structured résumé and profile.
// A zero/empty field means "no evidence" and its category is skipped.
type CVEvidence struct {
	TotalYears     int      // <=0 treated as unknown
	Degrees        []string // free-text degree names
	Languages      []string // free-text language names
	Certifications []string // free-text credential names
	CountryCode    string   // ISO-3166 alpha-2, "" if unknown
	PrefersRemote  bool     // caller prefers remote work
}

// Evaluate returns one entry per judgeable requirement. Entries with Met==false
// are the blockers; OverallCap turns them into a score ceiling.
func Evaluate(job JobRequirements, cv CVEvidence) []Blocker {
	var out []Blocker
	out = appendExperience(out, job, cv)
	out = appendEducation(out, job, cv)
	out = appendCertifications(out, job, cv)
	out = appendLanguage(out, job, cv)
	out = appendWorkAuth(out, job, cv)
	out = appendLocationWorkMode(out, job, cv)
	return out
}

// OverallCap is the minimum score-cap over the unmet blockers, or 100 when none
// is unmet (no ceiling). Callers clamp their score to this value.
func OverallCap(blockers []Blocker) int {
	ceiling := 100
	for _, b := range blockers {
		if !b.Met && b.ScoreCap < ceiling {
			ceiling = b.ScoreCap
		}
	}
	return ceiling
}

func blocker(cat BlockerCategory, met bool, reason, action string) Blocker {
	t := tier[cat]
	return Blocker{Category: cat, Severity: t.severity, ScoreCap: t.scoreCap, Reason: reason, Action: action, Met: met}
}

func appendExperience(out []Blocker, job JobRequirements, cv CVEvidence) []Blocker {
	// Both sides required: a real minimum on the job and parsed years on the CV.
	if job.ExperienceYearsMin == nil || *job.ExperienceYearsMin <= 0 || cv.TotalYears <= 0 {
		return out
	}
	need := *job.ExperienceYearsMin
	met := cv.TotalYears >= need
	reason := fmt.Sprintf("Requires %d+ years; résumé shows %d.", need, cv.TotalYears)
	action := fmt.Sprintf("Confirm you have %d+ years of relevant experience before applying; do not overstate it.", need)
	return append(out, blocker(CategoryExperience, met, reason, action))
}

func appendEducation(out []Blocker, job JobRequirements, cv CVEvidence) []Blocker {
	if job.DegreeOptional { // posting accepts equivalent experience → never a degree blocker
		return out
	}
	needRank, ok := degreeRank(job.EducationLevel)
	if !ok || needRank == 0 { // "" or "none" is no requirement
		return out
	}
	best, hasDegree := bestDegreeRank(cv.Degrees)
	if !hasDegree { // no parseable degree on the CV → no evidence either way
		return out
	}
	met := best >= needRank
	reason := fmt.Sprintf("Requires a %s degree; résumé's highest is lower.", job.EducationLevel)
	if met {
		reason = fmt.Sprintf("Requires a %s degree; résumé meets it.", job.EducationLevel)
	}
	action := fmt.Sprintf("Only claim a %s degree if you actually hold one.", job.EducationLevel)
	return append(out, blocker(CategoryEducation, met, reason, action))
}

func appendCertifications(out []Blocker, job JobRequirements, cv CVEvidence) []Blocker {
	if len(job.RequiredCertifications) == 0 {
		return out
	}
	held := canonicalSet(cv.Certifications)
	// Both-sides-present: with no recognized certification evidence on the résumé we
	// cannot tell "holds none" from "extraction missed it" (the field is new and
	// often empty), so we skip rather than raise a false blocker. A résumé that DOES
	// list certifications, just not the required one, is real evidence and blocks.
	if len(held) == 0 {
		return out
	}
	for _, req := range job.RequiredCertifications {
		met := held[req]
		reason := fmt.Sprintf("Requires the %s certification.", req)
		action := fmt.Sprintf("Do not claim the %s certification unless you currently hold it.", req)
		out = append(out, blocker(CategoryCertification, met, reason, action))
	}
	return out
}

func appendLanguage(out []Blocker, job JobRequirements, cv CVEvidence) []Blocker {
	// Info-only: a résumé language list rarely carries a proficiency level, so we
	// never block. We can only affirm the language is present; absence is not
	// evidence of its lack, so that case is skipped.
	if job.EnglishLevel == "" || !mentionsEnglish(cv.Languages) {
		return out
	}
	reason := fmt.Sprintf("Requires English (%s); résumé lists English.", job.EnglishLevel)
	action := "Be ready to demonstrate the required English level; do not overstate fluency."
	return append(out, blocker(CategoryLanguage, true, reason, action))
}

func appendWorkAuth(out []Blocker, job JobRequirements, cv CVEvidence) []Blocker {
	// Only when the job explicitly does not sponsor, is bound to countries, and the
	// caller's country is known and outside them.
	if job.VisaSponsorship == nil || *job.VisaSponsorship || len(job.Countries) == 0 || cv.CountryCode == "" {
		return out
	}
	if containsCountry(job.Countries, cv.CountryCode) {
		return out
	}
	reason := fmt.Sprintf("No visa sponsorship and the role is in %s; your résumé location is outside it.", strings.Join(job.Countries, ", "))
	action := "Confirm you already have the right to work there; the role does not sponsor a visa."
	return append(out, blocker(CategoryWorkAuth, false, reason, action))
}

func appendLocationWorkMode(out []Blocker, job JobRequirements, cv CVEvidence) []Blocker {
	// A remote-preference conflict needs no geography.
	if job.WorkMode == "onsite" && cv.PrefersRemote {
		reason := "On-site role, but you prefer remote work."
		action := "Confirm you can work on-site before applying."
		return append(out, blocker(CategoryLocationWorkMode, false, reason, action))
	}
	// A geographic mismatch needs both known country codes.
	if job.WorkMode == "onsite" && len(job.Countries) > 0 && cv.CountryCode != "" && !containsCountry(job.Countries, cv.CountryCode) {
		reason := fmt.Sprintf("On-site in %s; your résumé location is elsewhere.", strings.Join(job.Countries, ", "))
		action := "Confirm you can be on-site there (relocation or local presence)."
		return append(out, blocker(CategoryLocationWorkMode, false, reason, action))
	}
	return out
}

func bestDegreeRank(degrees []string) (int, bool) {
	best, found := 0, false
	for _, d := range degrees {
		if rank, ok := degreeMatch(d); ok {
			found = true
			if rank > best {
				best = rank
			}
		}
	}
	return best, found
}

func canonicalSet(certs []string) map[string]bool {
	held := make(map[string]bool, len(certs))
	for _, c := range certs {
		if slug, ok := credentials.Canonical(c); ok {
			held[slug] = true
		}
	}
	return held
}

func mentionsEnglish(languages []string) bool {
	for _, l := range languages {
		if strings.Contains(strings.ToLower(l), "english") {
			return true
		}
	}
	return false
}

func containsCountry(countries []string, code string) bool {
	for _, c := range countries {
		if strings.EqualFold(c, code) {
			return true
		}
	}
	return false
}
