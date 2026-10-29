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
// guess: an anchor-matched suffix that resolves nothing yields no geography.
// Only when no anchored suffix resolves does it read a place the title is merely
// labelled with (placeLabelFromTitle).
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
	if len(countrySet) == 0 && len(regionSet) == 0 {
		placeLabelFromTitle(title, countrySet, regionSet)
	}
	return stringset.Sorted(countrySet), stringset.Sorted(regionSet)
}

// titleTrailingBracket is a bracket that ends a title ("Frontend Engineer (LATAM)"), and
// titleTrailingSegment its last dash-separated segment ("QA Engineer - LATAM",
// "Automation Engineer (Java-Playwright)-India").
var (
	titleTrailingBracket = regexp.MustCompile(`[(\[]([^()\[\]]+)[)\]]\s*$`)
	titleTrailingSegment = regexp.MustCompile(`[-–—]\s*([^-–—()\[\]]+)$`)
)

// placeLabelFromTitle reads a place a title is labelled with in its own right, no anchor
// word needed: a closing bracket that holds nothing but a place ("Frontend Engineer
// (LATAM)"), a trailing dash segment that is nothing but one ("QA Engineer - Germany"),
// or a leading macro-region word ("LATAM Senior QA Analyst"). A label names where the
// role is; a title that is open everywhere has no reason to carry one.
//
// Only the bracket that ENDS the title: one inside it usually qualifies the word before
// it — a language's variety, "Spanish (Latin America) Language Data Contributor", "Tonga
// (Zambia) Interpreter" — which names where a language is spoken, not where the role is.
//
// Each candidate must resolve WHOLE, by name only (resolveGeoName — no ISO or
// subdivision code, so "(Go)" or "- AI" cannot misfire), and to somewhere narrower than global:
// "(Remote - Global)" is the anchor rule's business, and "(100% remote, worldwide)"
// restricts nothing. A leading word is read only as a macro-region, since a leading
// country name is as often the market a role sells into ("Germany Sales Manager").
func placeLabelFromTitle(title string, countrySet, regionSet map[string]struct{}) {
	var labels []string
	if m := titleTrailingBracket.FindStringSubmatch(title); m != nil {
		labels = append(labels, m[1])
	}
	if m := titleTrailingSegment.FindStringSubmatch(title); m != nil {
		labels = append(labels, m[1])
	}
	for _, label := range labels {
		tok := strings.ToLower(strings.TrimSpace(label))
		if r, ok := nameToRegion[tok]; ok && r == globalRegion {
			continue
		}
		resolveGeoName(tok, countrySet, regionSet)
	}
	if fields := strings.Fields(title); len(fields) > 1 {
		if r, ok := nameToRegion[strings.ToLower(fields[0])]; ok && r != globalRegion {
			regionSet[r] = struct{}{}
		}
	}
}
