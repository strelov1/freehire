package atsapply

import "strings"

// geographyTerms identifies a required question about the candidate's residency or where
// they are based. It differs from sensitiveTerms (sensitive.go), which parks on policy:
// this parks because the answer cannot be verified. candidateprofile has one free-text
// `location`, no discrete country/state fact matching a platform's options, so any answer
// would be invented. Narrower than sensitiveTerms' "authoriz"/"visa"/"sponsor", which park
// work-authorization questions separately.
var geographyTerms = []string{
	"state of residence", "country of residence", "residency",
	"currently reside", "currently located", "where are you based",
	"must be based in", "must reside", "must be located",
}

// isGeographyLabel reports whether a label asks about residency or physical base; draftable
// (draft.go) checks it so such a question parks instead of getting an invented value.
func isGeographyLabel(label string) bool {
	lower := strings.ToLower(label)
	for _, term := range geographyTerms {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return false
}
