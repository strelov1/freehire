package normalize

import "strings"

// legalSuffixes are corporate-form words dropped from the tail of a company name before two
// names are compared. Ordered longest-first so the most specific one strips first. The
// single-letter entries (" a s", " s a") are the punctuated Nordic and Romance forms after
// punctuation has already been reduced to word breaks: "Trafalgar A/S" arrives as
// "trafalgar a s".
var legalSuffixes = []string{
	" corporation", " limited", " gmbh", " corp", " llc", " ltd", " inc", " plc", " llp",
	" srl", " pty", " a s", " s a", " bv", " nv", " ab", " ag", " kg", " oy", " sa",
	" as", " co",
}

// CompanySlug is [Slug] with any trailing corporate forms removed: "Arch Capital Group Ltd."
// becomes "arch-capital-group". An empty or untransliterable name yields "".
//
// Stripping repeats rather than stopping at one match: compound forms are ordinary ("Acme
// GmbH & Co. KG", "Atlassian Pty Ltd"), and a single pass would leave half the form behind.
// The word breaks must survive until the strip — folding "Derq, Inc." to "derqinc" first
// would glue the suffix on and hide it.
func CompanySlug(name string) string {
	s := strings.Join(strings.Fields(strings.ReplaceAll(Slug(name), "-", " ")), " ")
	for stripped := true; stripped; {
		stripped = false
		for _, suf := range legalSuffixes {
			if strings.HasSuffix(s, suf) {
				s, stripped = strings.TrimSuffix(s, suf), true
				break
			}
		}
	}
	return strings.ReplaceAll(s, " ", "-")
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
