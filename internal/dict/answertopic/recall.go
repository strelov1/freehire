package answertopic

import "strings"

// workAuthorizationTerms names a question about being allowed to work somewhere — the one
// kind of screening question a banked answer may never be RECALLED for.
//
// "Are you authorized to work in the country in which this position is located?" is one
// question text across every posting worded that way, so it folds to ONE topic — and a
// "Yes" the candidate banked while looking at a US posting would then be re-asserted, in
// their name, on a Brazilian one. Answering it needs the posting's country, which a topic
// does not carry.
//
// Deliberately narrow. Salary is the case the bank exists for, and a demographic question is
// the candidate's own answer to give; only authorization depends on a fact the bank lacks.
//
// Every surface that recalls from the bank asks Recallable — internal/api/atsapply on the
// headless path, internal/ai/autofillagent in the browser — so the rule lives here, below
// both. The review screen keeps the same list in TypeScript (web/src/lib/answerBank.ts's
// WORK_AUTHORIZATION_TERMS) so it never offers an input the server will refuse to recall;
// the two cannot share a definition across languages, so each names the other.
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
