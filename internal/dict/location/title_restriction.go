package location

import (
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/platform/stringset"
)

// bracketSuffixPattern extracts the content of a single-level bracketed or
// parenthetical title suffix, e.g. "[Remote-US]" -> "Remote-US", "(Location -
// Australia or New Zealand)" -> "Location - Australia or New Zealand".
var bracketSuffixPattern = regexp.MustCompile(`\[([^\[\]]*)\]|\(([^()]*)\)`)

// titleRestrictionAnchor matches the ATS convention that states a geography
// restriction inside a title's bracket/parenthetical suffix, as opposed to an
// unrelated bracket ("(React)", "(Contract)", a seniority band). Deliberately
// narrow — see design.md's precision-over-recall rationale, the same stance
// eligibility.go's phrase lists take.
var titleRestrictionAnchor = regexp.MustCompile(`\b(remote|location)\b`)

// RestrictionFromTitle reports the geography a job title's bracketed or
// parenthetical suffix restricts a posting to — the ATS convention a bare
// "Remote" location string alone cannot see (e.g. "Senior Engineer
// [Remote-US]", "(Location - Australia or New Zealand)"). It scans only a
// suffix that contains an anchor word, strips that word, and resolves the
// remainder through the same separator normalization and curated
// country/region dictionaries Parse uses — never a second dictionary, never a
// guess: an anchor-matched suffix that resolves nothing yields no geography,
// and a bracket without an anchor is never scanned at all.
func RestrictionFromTitle(title string) (countries, regions []string) {
	countrySet := map[string]struct{}{}
	regionSet := map[string]struct{}{}
	for _, m := range bracketSuffixPattern.FindAllStringSubmatch(title, -1) {
		inner := m[1]
		if inner == "" {
			inner = m[2]
		}
		normalized := strings.ReplaceAll(strings.ToLower(inner), "-", " ")
		if !titleRestrictionAnchor.MatchString(normalized) {
			continue
		}
		rest := strings.TrimSpace(titleRestrictionAnchor.ReplaceAllString(normalized, " "))
		if rest == "" {
			continue
		}
		s := separatorReplacer.Replace(rest)
		for _, tok := range strings.Split(s, ",") {
			tok = strings.TrimSpace(tok)
			if tok == "" {
				continue
			}
			// prevTok is always "" here — a colliding subdivision code (e.g. "LA")
			// cannot be disambiguated by a neighboring token the way Parse's own
			// comma-token loop does. Deliberate, not an oversight: it costs a small,
			// safe recall gap (never guesses) in exchange for not threading
			// cross-token state through a bracket suffix this narrow.
			resolveGeoToken(tok, "", countrySet, regionSet)
		}
	}
	return stringset.Sorted(countrySet), stringset.Sorted(regionSet)
}
