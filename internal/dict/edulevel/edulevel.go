// Package edulevel resolves free text naming an education level to
// internal/dict/vocab.EducationLevelValues. Like internal/dict/location it exposes
// two entry points over one vocabulary rather than a flag, because the same kind of
// text means different things on the two sides: ForRequirement reads a job posting's
// requirement prose ("Bachelor's degree required"), ForDegree reads a CV's own degree
// field ("BSc", "MSc Computer Science"). Curated matcher, never a model: it resolves
// explicit signals and emits "" for what it cannot place.
package edulevel

import (
	"regexp"
	"strings"
)

// Requirement-text matchers, moved here from internal/job/jobfacts unchanged. See
// that package's own comment for why bare single-letter abbreviations are excluded
// here: "ms"/"m.s" collide with "MS Office"/"MS SQL" and "bs"/"b.s" with everyday
// text in free-running requirement prose, and bare "master" is excluded because
// "scrum master" is not a degree. The possessive admits both apostrophes for the
// same reason documented there: a rich-text editor's typographic one is common in
// production postings.
var (
	reReqPhD      = regexp.MustCompile(`\b(ph\.?\s?d|phd|doctorate|doctoral)\b`)
	reReqMaster   = regexp.MustCompile(`\b(master['’]?s|master degree|m\.?sc|mba|graduate degree)\b`)
	reReqBachelor = regexp.MustCompile(`\b(bachelor['’]?s|bachelor degree|b\.?sc|undergraduate degree)\b`)
	reReqNoDegree = regexp.MustCompile(`\b(no (?:degree|diploma)|degree not required|without a degree|no degree required)\b`)
)

// ForRequirement resolves a job posting's requirement text to one of
// vocab.EducationLevelValues, or "" when nothing is stated. A named degree wins over
// a "no degree" phrase (a posting that says "Bachelor's or equivalent; no degree
// required for exceptional candidates" still has a degree signal). The caller is
// expected to have already isolated required (non-optional) text — see
// internal/job/jobfacts.hardRequirementText, which stays in that package since it
// depends on internal/job/reqextract, a job-layer package this one may not import.
func ForRequirement(text string) string {
	s := strings.ToLower(text)
	switch {
	case reReqPhD.MatchString(s):
		return "phd"
	case reReqMaster.MatchString(s):
		return "master"
	case reReqBachelor.MatchString(s):
		return "bachelor"
	case reReqNoDegree.MatchString(s):
		return "none"
	}
	return ""
}

// Degree-field matchers, tuned for a CV's own Degree text rather than free-running
// prose. Bare "BS"/"MS"-style forms are safe here in a way they are not for
// ForRequirement: the input is always a dedicated degree field (resumeextract's
// Education.Degree), never a sentence where those letters could mean something
// else. "none" never applies to a candidate's own history — a candidate either has a
// degree that resolves or doesn't — so ForDegree, unlike ForRequirement, never
// returns it.
var (
	reDegPhD      = regexp.MustCompile(`\b(ph\.?\s?d\.?|phd|doctorate|doctoral)\b`)
	reDegMaster   = regexp.MustCompile(`\b(m\.?sc\.?|m\.?s\.?|mba|master(?:['’]s)?(?:\s+degree)?(?:\s+of\s+[a-z]+(?:\s+[a-z]+)*)?)\b`)
	reDegBachelor = regexp.MustCompile(`\b(b\.?sc\.?|b\.?s\.?|bachelor(?:['’]s)?(?:\s+degree)?(?:\s+of\s+[a-z]+(?:\s+[a-z]+)*)?)\b`)
)

// ForDegree resolves a CV's own degree text to one of "bachelor"/"master"/"phd", or
// "" when nothing resolves (a professional certificate, a bare "Diploma", or empty
// text). Checked highest-degree-first, same precedence ForRequirement uses.
func ForDegree(text string) string {
	s := strings.ToLower(text)
	switch {
	case reDegPhD.MatchString(s):
		return "phd"
	case reDegMaster.MatchString(s):
		return "master"
	case reDegBachelor.MatchString(s):
		return "bachelor"
	}
	return ""
}
