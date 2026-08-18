package normalize

import (
	"strings"
	"unicode"
)

// legalSuffixes are the corporate-form words dropped from the tail of a company name before
// two names are compared. This is the module's ONLY such list — three packages used to keep
// their own and disagreed, silently, in both directions.
//
// Matched as whole trailing WORDS reduced to their ASCII letters, so one entry covers every
// spelling of a form: "bv" catches "BV", "B.V." and "b.v.". That is also why there is no entry
// for the punctuated Nordic and Romance forms ("A/S", "S.A.") — reducing the word to its
// letters already makes them "as" and "sa".
//
// A TOKEN EARNS ITS PLACE IN THE DATA, not by looking safe. The test is not whether a word
// seems dangerous but whether stripping it lands on a DIFFERENT existing employer:
//
//   - "co" was once excluded for colliding with ordinary words. All 297 catalogue companies
//     whose slug ends in it are "& Co." forms, and of the 25 largest merges the wider tokens
//     produce, every one lands on the right employer (Accenture GmbH → Accenture, Goldman
//     Sachs & Co. → Goldman Sachs).
//   - "pte" is small — 7 companies — and earns its place on one of them: without it,
//     "epam systems pte. ltd." keys apart from the EPAM Systems that holds 1,172 open jobs.
//
// Two tails that look like they belong here and deliberately do not:
//
//   - "spa" is the Italian S.p.A. AND the literal word. "Hilton Luxor Resort & Spa" carries
//     more open jobs than any genuine S.p.A. in the catalogue, so stripping it would merge a
//     hotel into a resort chain — the collision the excluded-"co" comment feared, in the one
//     place it is real.
//   - "group" is not a corporate form at all, it is part of a brand. Every "X Group" / "X"
//     pair in the catalogue does look like one employer (Thales, Securitas, Bosch), but
//     "group" is an ordinary word by which two businesses legitimately differ, so merging on
//     it is a judgement. Judgements belong to cmd/merge-companies, which shows a dry run,
//     elects by job count and records a reversible alias — not to a pure function applied
//     silently to every new posting. cmd/harvest-ats keeps both as board-name guesses, where
//     an extra candidate costs one lookup rather than an employer.
var legalSuffixes = map[string]struct{}{
	"corporation": {}, "limited": {}, "gmbh": {}, "corp": {}, "llc": {}, "ltd": {},
	"inc": {}, "incorporated": {}, "plc": {}, "llp": {}, "lp": {}, "cic": {}, "cio": {},
	"srl": {}, "pty": {}, "pte": {}, "bv": {}, "nv": {}, "aps": {},
	"ab": {}, "ag": {}, "kg": {}, "oy": {}, "sa": {}, "as": {}, "se": {}, "co": {},
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
	words := strings.FieldsFunc(dropApostrophes(name), isWordBreak)
	for len(words) > 1 {
		if _, isForm := legalSuffixes[letters(words[len(words)-1])]; !isForm {
			break
		}
		words = words[:len(words)-1]
	}
	return Slug(strings.Join(words, " "))
}

// dropApostrophes removes the possessive/elision marks a name carries so they cannot become
// word breaks in the key. "Kohl's" is one word and belongs at `kohls`, which is also what a
// person types; [Slug] alone renders it `kohl-s`.
//
// It matters more than it looks. Once the canonical slug is DERIVED from a name rather than
// chosen among stored slugs, that artefact can no longer lose to the clean sibling spelling —
// it elected `kohl-s` over `kohls` across 2,939 postings. Both the ASCII and the typographic
// mark are covered, since sources use either.
func dropApostrophes(name string) string {
	return strings.NewReplacer("'", "", "\u2019", "", "\u02bc", "").Replace(name)
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

// IsLegalForm reports whether a single word of a company name is a corporate form ("Ltd.",
// "GmbH", "B.V."), by the same letters-only comparison [CompanySlug] strips with.
//
// It is exported for callers that need the judgement without the slug — collections'
// RequireCountry counts how many WHITESPACE tokens of a register name are the name proper,
// and must not use [CompanySlug]'s word breaks, which split "T-Mobile" in two and would make
// a single-token name look specific enough to skip its headquarters check.
func IsLegalForm(word string) bool {
	_, ok := legalSuffixes[letters(word)]
	return ok
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
	return FoldSlug(CompanySlug(name))
}

// FoldSlug removes a slug's word breaks, turning it into the comparison key two sources that
// separated the words differently both land on: "dollar-tree" and "dollartree" fold alike.
//
// It is the same fold jobs.company_slug_folded stores (migration 0109) and the same one
// company_slug_aliases.folded_key is keyed by, which is what lets a spelling nobody ever
// merged reach a canon its folded form already owns. Take [CompanyKey] when you hold a NAME;
// this is for when the slug is already in hand.
func FoldSlug(slug string) string {
	return strings.ReplaceAll(slug, "-", "")
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
