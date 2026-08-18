package normalize

import (
	"strings"
	"unicode"
)

// legalSuffixes are the corporate-form words dropped from the tail of a company name before
// two names are compared. Matched as whole trailing WORDS reduced to their ASCII letters, so
// one entry covers every spelling of a form: "bv" catches "BV", "B.V." and "b.v.".
//
// There is deliberately no entry for the punctuated Nordic and Romance forms ("A/S", "S.A.")
// — reducing the word to its letters already makes them "as" and "sa".
var legalSuffixes = map[string]struct{}{
	"corporation": {}, "limited": {}, "gmbh": {}, "corp": {}, "llc": {}, "ltd": {},
	"inc": {}, "plc": {}, "llp": {}, "srl": {}, "pty": {}, "bv": {}, "nv": {},
	"ab": {}, "ag": {}, "kg": {}, "oy": {}, "sa": {}, "as": {}, "co": {},
}

// CompanySlug is [Slug] with any trailing corporate forms removed: "Arch Capital Group Ltd."
// becomes "arch-capital-group". An empty or untransliterable name yields "".
//
// Stripping repeats rather than stopping at one match: compound forms are ordinary ("Acme
// GmbH & Co. KG", "Atlassian Pty Ltd"), and a single pass would leave half the form behind.
//
// The match runs on the name's own words rather than on [Slug]'s output, because Slug has
// already turned "B.V." into two word breaks ("booking-b-v") where no form can be recognised.
// Reducing each word to its ASCII letters instead sees the form whatever its punctuation.
//
// A single-word name is never stripped: "Limited" stays "limited", because an empty slug
// silently matches nothing while a visibly odd company row can be found and fixed.
func CompanySlug(name string) string {
	words := strings.FieldsFunc(name, isWordBreak)
	for len(words) > 1 {
		if _, isForm := legalSuffixes[letters(words[len(words)-1])]; !isForm {
			break
		}
		words = words[:len(words)-1]
	}
	return Slug(strings.Join(words, " "))
}

// isWordBreak reports whether a rune separates two words of a company name. Everything [Slug]
// would drop separates, EXCEPT "." and "/" — those two live inside the very forms this is
// looking for ("B.V.", "A/S"), so splitting on them would hide the form instead of finding it.
//
// Whitespace alone is not enough: "Sun Technologies,Inc." is how aggregators routinely write
// the name, and 13,730 companies in the catalogue carry a form with no space before it.
// Splitting here rather than on Slug's output is also what keeps "Foo.com" one word.
func isWordBreak(r rune) bool {
	return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '.' && r != '/'
}

// letters lowercases a word down to its ASCII letters, so the punctuated and bare spellings
// of a legal form ("B.V.", "BV", "Ltd.") compare as one. It runs on the raw word rather than
// on its slug because Slug turns "B.V." into "b-v" — two tokens, matching no form — while
// leaving a name like "Foo.com" intact, which stripping dots globally would not.
func letters(word string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(word) {
		if r >= 'a' && r <= 'z' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// CompanyKey folds a company name to a comparable core: [CompanySlug] with the word breaks
// removed as well, so two sources that separate the words differently still agree.
//
// This is a comparison key, not a display or URL key — unlike [Slug] it joins the words with
// nothing and strips legal forms, both of which would be wrong in a path segment.
func CompanyKey(name string) string {
	return strings.ReplaceAll(CompanySlug(name), "-", "")
}

// SameCompany reports whether two company names denote the same employer.
//
// The comparison is deliberately mild — case, punctuation, script and a trailing corporate
// form are noise between one source's label and another's. It is equally deliberately not
// fuzzy: substring or prefix matching would treat every employer whose name contains a short
// common word as the same company ("Base" would match "Basecamp"). Two names that both fold
// to nothing are not a match, since that says nothing about either.
func SameCompany(a, b string) bool {
	ka := CompanyKey(a)
	return ka != "" && ka == CompanyKey(b)
}
