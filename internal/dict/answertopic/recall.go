package answertopic

import "strings"

// workAuthorizationTerms names a question about being allowed to work somewhere — the one
// kind of screening question a banked answer may never be RECALLED for.
//
// "Are you authorized to work in the country in which this position is located?" folds to
// ONE topic across postings, so a "Yes" banked on a US posting would be re-asserted on a
// Brazilian one. Answering needs the posting's country, which a topic does not carry.
// Deliberately narrow: only authorization depends on a fact the bank lacks.
//
// Every recalling surface (internal/api/atsapply, internal/ai/autofillagent) asks
// Recallable. The review screen keeps the same list in TypeScript
// (web/src/lib/answerBank.ts WORK_AUTHORIZATION_TERMS); keep the two in sync.
var workAuthorizationTerms = []string{"authoriz", "right to work", "sponsor", "visa"}

// Recallable reports whether a banked answer may be recalled for this question. A question
// about work authorization is refused whatever the bank holds; see workAuthorizationTerms.
func Recallable(question string) bool {
	lower := strings.ToLower(question)
	for _, term := range workAuthorizationTerms {
		if strings.Contains(lower, term) {
			return false
		}
	}
	return true
}
