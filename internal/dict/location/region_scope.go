package location

import (
	"strings"

	"github.com/strelov1/freehire/internal/platform/htmltext"
	"github.com/strelov1/freehire/internal/platform/stringset"
)

// regionScopePhrases anchor an explicit "this role is restricted to a named place"
// statement, as opposed to EligibilityFromDescription's closed per-nationality
// citizenship/work-authorization phrases: these introduce an open-ended list of
// countries/regions that the curated dictionaries resolve, the same way a `location`
// string's tokens do. Deliberately a short, anchored set — precision over recall,
// the same stance eligibility.go's phrase lists take — tuned against the reported
// cases rather than an exhaustive sweep of every ATS's phrasing.
//
// "based in"/"located in" are deliberately ROLE- or CANDIDATE-qualified rather than
// bare: an unqualified "based in"/"located in" is ambiguous between a role
// restriction and a company-HQ "About Us" mention ("Our company is based in Berlin,
// Germany, but this role is fully remote and open worldwide" — a real, common ATS
// boilerplate pattern found in code review), and a bare anchor would read the HQ
// sentence as the restriction regardless of what the actual role statement says
// elsewhere in the same description. "restricted to" likewise needs a role or
// candidate subject: access to employee records "restricted to HR and IT" describes
// departments, whose codes also name countries. "remote role within"/"hiring in"/
// "open to candidates|applicants in" already qualify the role or its candidates.
var regionScopePhrases = []string{
	"remote role within",
	"candidates based in",
	"applicants based in",
	"employees based in",
	"role is based in",
	"you must be based in",
	"candidates located in",
	"applicants located in",
	"role is located in",
	"role is restricted to",
	"candidates are restricted to",
	"open to candidates in",
	"open to applicants in",
	"hiring in",
}

// clauseWindow bounds how much text after an asserted anchor phrase is read as the
// scoping clause, so one long unpunctuated block of text cannot pull an unrelated
// paragraph in as if it were part of the list. A run-on sentence with no punctuation
// before the cap could still pull in a following unrelated clause, the same trade-off
// negationWindow makes in eligibility.go — mitigated in practice because only tokens
// the curated dictionaries actually resolve contribute, so trailing noise words are
// silently dropped rather than misread as a place.
const clauseWindow = 200

// RegionScopeFromDescription reports the geography an explicit "role is based/located
// within/restricted to <place list>" statement in a job description restricts a
// posting to. It is additive to, and does not replace, EligibilityFromDescription's
// citizenship-phrase rules: this function targets open-ended place lists
// ("...within New Zealand, Australia, or nearby time zones"), which a closed
// per-nationality phrase table cannot enumerate.
//
// It never guesses: the clause following an asserted anchor is tokenized the same way
// a `location` string is, and only tokens the curated dictionaries resolve contribute
// — a noise phrase like "nearby time zones" or "East Coast" attached to a real country
// name is silently skipped rather than producing a wrong or partial guess. A phrase
// found in a sentence that denies it ("not restricted to the US") is not a match, the
// same negation handling EligibilityFromDescription uses.
func RegionScopeFromDescription(desc string) (countries, regions []string) {
	lower := strings.ToLower(htmltext.ToTextFull(desc))
	lower = strings.NewReplacer("u.s.a.", "usa", "u.s.", "us", "u.k.", "uk").Replace(lower)
	// Keep phrase priority, but let an unresolved clause yield to a later anchor.
	for _, p := range regionScopePhrases {
		for offset := 0; ; {
			i := strings.Index(lower[offset:], p)
			if i < 0 {
				break
			}
			start := offset + i
			end := start + len(p)
			if wholeWordMatch(lower, start, end) && !negatedSentence(sentenceAround(lower, start, end)) {
				countries, regions := regionScopeFromClause(clauseAfterPhrase(lower, end))
				if len(countries) > 0 || len(regions) > 0 {
					return countries, regions
				}
			}
			offset = end
		}
	}
	return nil, nil
}

func regionScopeFromClause(clause string) (countries, regions []string) {
	countrySet := map[string]struct{}{}
	regionSet := map[string]struct{}{}
	for _, tok := range strings.Split(separatorReplacer.Replace(clause), ",") {
		resolveRestrictionToken(tok, countrySet, regionSet)
	}
	return stringset.Sorted(countrySet), stringset.Sorted(regionSet)
}

// resolveRestrictionToken keeps names such as "Bosnia and Herzegovina" whole,
// splitting a conjunction only when the full token names no known place. Each
// remaining prefix is tried longest-first so a list can contain compound names.
func resolveRestrictionToken(tok string, countrySet, regionSet map[string]struct{}) {
	tok = strings.TrimSuffix(strings.Trim(tok, "* "), " only")
	tok = strings.TrimPrefix(tok, "the ")
	// No neighboring token is supplied: unlike Parse, a restriction list cannot
	// disambiguate colliding subdivision codes from cross-token context.
	if resolveGeoToken(tok, "", countrySet, regionSet) {
		return
	}
	parts := strings.Split(strings.ReplaceAll(tok, " & ", " and "), " and ")
	if len(parts) < 2 {
		return
	}
	for len(parts) > 0 {
		consumed := 1
		for end := len(parts); end > 0; end-- {
			part := strings.TrimPrefix(strings.Trim(strings.Join(parts[:end], " and "), "* "), "the ")
			if resolveGeoToken(part, "", countrySet, regionSet) {
				consumed = end
				break
			}
		}
		parts = parts[consumed:]
	}
}

// clauseAfterPhrase returns the text following an asserted anchor phrase, trimmed to
// the next sentence boundary (., !, ?, or newline) and capped at clauseWindow bytes.
func clauseAfterPhrase(lower string, pos int) string {
	hi := pos + clauseWindow
	if hi > len(lower) {
		hi = len(lower)
	}
	rest := lower[pos:hi]
	if i := strings.IndexAny(rest, ".!?\n"); i >= 0 {
		rest = rest[:i]
	}
	return rest
}
