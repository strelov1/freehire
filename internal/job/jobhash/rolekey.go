package jobhash

import (
	"regexp"
	"strings"
	"unicode"
)

// RoleKey returns a CROSS-SOURCE role identity: the company slug and the
// normalized, trailing-clause-stripped title, joined. Two postings of one role
// share a key however differently their descriptions read.
//
// It is deliberately weaker than RoleFingerprint, which also hashes the
// description. That strength is right for clustering reposts WITHIN a source,
// where the same adapter produced both texts. Across sources it is wrong:
// aggregators truncate and rewrite descriptions, so a fingerprint match between an
// aggregator listing and the employer's own board would be a coincidence rather
// than a rule — the cross-check would find nothing in common and report every
// posting as missing from its own company's board.
//
// What survives rewriting is the company and the title, so that is the key. It
// reuses the same normalization RoleFingerprint applies to a title, so a per-city
// variant collapses onto its base role here exactly as it does there.
//
// Parentheses are UNWRAPPED rather than kept, because an aggregator that adds or drops
// them is describing the same role: measured on prod, "Data Engineer (Semi Senior)" and
// "Data Engineer Semi Senior" counted as two roles and inflated the cross-check's absent
// count by 13%. Unwrapping keeps the words — dropping the contents would collapse
// "Go Developer (Remote)" onto "Rust Developer (Remote)".
//
// A title that normalizes to nothing yields the empty string, and a caller MUST
// treat that as "no key" rather than as a key — every blank title would otherwise
// match every other one.
func RoleKey(companySlug, title string) string {
	normalized := normalizeRoleText(stripTrailingClause(unwrapParens(title)))
	if normalized == "" {
		return ""
	}
	return companySlug + "\x1e" + normalized
}

// parens matches one parenthesised group.
var parens = regexp.MustCompile(`[()]`)

// unwrapParens removes the brackets while keeping what they enclose, so a role reads
// the same whether or not the source wrapped its qualifier. normalizeRoleText collapses
// the whitespace this leaves behind.
func unwrapParens(title string) string {
	return parens.ReplaceAllString(title, " ")
}

// TitleKeys returns every key under which a title may appear on ANOTHER source for
// the same company — the looser, punctuation-blind sibling of RoleKey that the ghost
// cross-check matches on. A title is the same role as another when their key sets
// intersect.
//
// RoleKey alone is too strict for that question, and the error is not symmetric: a
// key that fails to match makes the cross-check ACCUSE the employer of advertising a
// role it does not have. Measured on prod (2026-11-01), aggregators routinely drop
// the board's punctuation — jobstash lists Plaid's "Account Executive - Fraud" as
// "Account Executive Fraud" — and RoleKey strips " - Fraud" as a trailing clause on
// one side only, so the same role produced two keys. Two keys per title cover it:
//
//   - the whole title, with every run of punctuation folded to a space, so a dash, a
//     comma or a slash the other source dropped does not matter;
//   - the same with its trailing clause stripped first, so a per-city variant still
//     meets its base role, as it does under RoleKey.
//
// Erring toward a match is deliberate: a missed absence costs one uncounted ghost
// criterion, a false one accuses an honest employer. A title that normalizes to
// nothing yields no keys.
func TitleKeys(title string) []string {
	title = unwrapParens(title)
	full := foldPunctuation(normalizeRoleText(title))
	if full == "" {
		return nil
	}
	stripped := foldPunctuation(normalizeRoleText(stripTrailingClause(title)))
	if stripped == "" || stripped == full {
		return []string{full}
	}
	return []string{full, stripped}
}

// foldPunctuation replaces every run of characters that is neither a letter, a digit
// nor a combining mark with one space, so "front-end/back-end" and "front end back
// end" agree. Marks are kept because scripts such as Devanagari write vowels with
// them, and splitting a word on its own vowel would be no fold at all.
func foldPunctuation(s string) string {
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !unicode.IsMark(r)
	}), " ")
}
