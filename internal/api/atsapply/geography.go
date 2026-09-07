package atsapply

import "strings"

// geographyTerms identifies a required question asking about the candidate's residency,
// citizenship-adjacent location, or where they are based — distinct from sensitiveTerms
// (sensitive.go), which parks on POLICY grounds (a category the model must never be
// trusted to answer regardless of confidence). A geography question parks because the
// pipeline cannot VERIFY an answer against it: candidateprofile carries one free-text
// `location` string, never a discrete country/state-of-residence fact matching a
// platform's own enumerated options, so any answer here would be the drafter inventing a
// value rather than reporting one the candidate actually gave (see
// openspec/changes/add-auto-apply-eligibility-gate).
//
// Seeded from the live Garner Health Greenhouse posting's own wording ("Current State of
// Residence") plus the country/residency equivalents the same shape of question takes on
// other platforms. Deliberately narrower than sensitiveTerms' "authoriz"/"visa"/"sponsor"
// terms, which already park a work-authorization question through a different rule.
var geographyTerms = []string{
	"state of residence", "country of residence", "residency",
	"currently reside", "currently located", "where are you based",
	"must be based in", "must reside", "must be located",
}

// isGeographyLabel reports whether a question's label asks about the candidate's
// residency or physical base — checked by draftable (draft.go) ahead of drafting, so a
// required geography question a candidate has not answered parks instead of receiving an
// invented value.
func isGeographyLabel(label string) bool {
	lower := strings.ToLower(label)
	for _, term := range geographyTerms {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return false
}
