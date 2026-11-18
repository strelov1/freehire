package jobhash

import (
	"regexp"
	"strings"
	"unicode"
)

// RoleKey returns a cross-source role identity: the company slug and the
// normalized, trailing-clause-stripped title, joined.
//
// It is deliberately weaker than RoleFingerprint, which also hashes the description.
// Across sources that is wrong: aggregators truncate and rewrite descriptions, so a
// fingerprint match against the employer's own board would be a coincidence. The
// title normalization is shared with RoleFingerprint.
//
// Parentheses are unwrapped, not kept or dropped with their contents: an aggregator
// that adds or drops them describes the same role, but dropping the contents would
// collapse "Go Developer (Remote)" onto "Rust Developer (Remote)".
//
// A title that normalizes to nothing yields "", and a caller MUST treat that as "no
// key": every blank title would otherwise match every other one.
func RoleKey(companySlug, title string) string {
	normalized := normalizeRoleText(stripTrailingClause(unwrapParens(title)))
	if normalized == "" {
		return ""
	}
	return companySlug + "\x1e" + normalized
}

// parens matches one parenthesised group.
var parens = regexp.MustCompile(`[()]`)

// unwrapParens removes the brackets but keeps what they enclose.
// normalizeRoleText collapses the whitespace this leaves behind.
func unwrapParens(title string) string {
	return parens.ReplaceAllString(title, " ")
}

// TitleKeys returns every key under which a title may appear on another source for
// the same company: the looser, punctuation-blind sibling of RoleKey that the ghost
// cross-check matches on. Two titles are the same role when their key sets intersect.
//
// RoleKey alone is too strict: a missed match makes the cross-check accuse the
// employer of advertising a role it does not have (an aggregator drops "Account
// Executive - Fraud" to "Account Executive Fraud", and only one side strips the
// clause). Two keys per title cover it:
//
//   - the whole title, every run of punctuation folded to a space;
//   - the same with its trailing clause stripped first, so a per-city variant still
//     meets its base role, as under RoleKey.
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

// foldPunctuation replaces every run of non-letter, non-digit, non-mark characters
// with one space. Marks are kept because scripts such as Devanagari write vowels
// with them.
func foldPunctuation(s string) string {
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !unicode.IsMark(r)
	}), " ")
}
