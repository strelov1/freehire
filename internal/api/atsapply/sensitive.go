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

// workAuthorizationTerms is the SUBSET of sensitiveTerms that names a question about being
// allowed to work somewhere. It is separate because the two lists answer different
// questions: sensitiveTerms decides what a model may never DRAFT, this decides what the
// answer bank may never RECALL, and only the second is about geography.
//
// "Are you authorized to work in the country in which this position is located?" is one
// question text across every posting worded that way, so it folds to ONE topic
// (internal/dict/answertopic) — and a "Yes" the candidate banked while looking at a US
// posting would then be re-asserted, in their name, on a Brazilian one. The candidate need
// never see it happen: an answer banked after the review screen's preview was persisted is
// filled at submit time, and the preview pass only resolves entries that have none yet.
// labelAnswerKeyFor's own doc comment states the same invariant for the same reason — that
// answering this needs the JOB's location, which nothing in this package holds.
//
// Deliberately NOT all of sensitiveTerms. Salary is on that list and the salary case is what
// the bank exists for; a demographic question is on it too and is the candidate's own answer
// to give. Only authorization depends on a fact this package does not have.
//
// The review screen keeps the same list in TypeScript, in web/src/lib/answerBank.ts's
// WORK_AUTHORIZATION_TERMS — it must not offer an input for a question the server will
// refuse to recall. The two cannot share a definition across languages, so each names the
// other, the way bankAnswerKeyPrefix already does.
var workAuthorizationTerms = []string{"authoriz", "right to work", "sponsor", "visa"}

// isWorkAuthorizationLabel reports whether a question's label asks whether the candidate may
// work in this posting's country. See workAuthorizationTerms.
func isWorkAuthorizationLabel(label string) bool {
	return containsAnyTerm(label, workAuthorizationTerms)
}

// isSensitiveLabel reports whether a question's label text concerns compensation, work
// authorization/visa sponsorship, or an equal-opportunity/demographic category — the
// categories a candidate's answer must never be guessed or drafted for, only ever taken
// from a fact the candidate stated directly (see labelAnswerKeyFor's visa_sponsorship_needed
// case) or left to park.
func isSensitiveLabel(label string) bool {
	return containsAnyTerm(label, sensitiveTerms)
}

// containsAnyTerm reports whether the label contains any of the terms, case-insensitively.
// Substring, not whole-word: every term here is a stem ("authoriz" for
// "authorized"/"authorization", "disab" for "disability"/"disabled") chosen so a partial
// spelling still matches.
func containsAnyTerm(label string, terms []string) bool {
	lower := strings.ToLower(label)
	for _, term := range terms {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return false
}
