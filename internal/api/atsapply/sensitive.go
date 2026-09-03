package atsapply

import "strings"

// sensitiveTerms is a direct port of freehire-apply/internal/drafting's isSensitive list
// (a sibling, more mature paid repo — already measured against real Ashby postings there,
// not invented fresh here), with one fix a live smoke check (task 5.1) found: the ported
// "work authoriz" is a FIXED-ORDER phrase and never matches "authorization to work" — a
// real Greenhouse posting's exact wording, the more common of the two orderings in
// practice. Replaced with the standalone "authoriz", which catches "authorization"/
// "authorized" in either order and is, if anything, the more defensible term: in an
// application form's context there is no ordinary (non-sensitive) reason a question would
// use that word at all.
//
// A question matching any of these is never drafted, regardless of how confident a draft
// would be — draftable (draft.go) checks isSensitiveLabel before Drafter.Draft is ever
// invoked, so a sensitive question never reaches the model at all.
var sensitiveTerms = []string{
	"salary", "compensation", "sponsor", "visa", "authoriz", "right to work",
	"gender", "race", "ethnic", "veteran", "disab", "demographic", "sexual orientation",
	// The remaining US EEOC-adjacent categories a live posting could ask about as a
	// custom question, found missing by a PR review pass: religion, national origin,
	// date of birth (age-discrimination-adjacent), and genetic information (GINA).
	"religio", "national origin", "date of birth", "genetic",
}

// isSensitiveLabel reports whether a question's label text concerns compensation, work
// authorization/visa sponsorship, or an equal-opportunity/demographic category — the
// categories a candidate's answer must never be guessed or drafted for, only ever taken
// from a fact the candidate stated directly (see labelAnswerKeyFor's visa_sponsorship_needed
// case) or left to park.
func isSensitiveLabel(label string) bool {
	lower := strings.ToLower(label)
	for _, term := range sensitiveTerms {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return false
}
