package location

import (
	"strings"

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
// elsewhere in the same description. "remote role within"/"restricted to"/"hiring
// in"/"open to candidates|applicants in" carry no such ambiguity — they can only
// describe the role, never the employer's office — so they stay bare.
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
	"restricted to",
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
	lower := strings.ToLower(desc)
	countrySet := map[string]struct{}{}
	regionSet := map[string]struct{}{}
	end, ok := firstAssertedRegionScopePhrase(lower)
	if !ok {
		return nil, nil
	}
	clause := clauseAfterPhrase(lower, end)
	s := separatorReplacer.Replace(clause)
	for _, tok := range strings.Split(s, ",") {
		tok = strings.TrimSpace(tok)
		tok = strings.TrimPrefix(tok, "the ")
		// A trailing qualifier ("...Brazil only") sits in the same comma segment as
		// the last country in the list ("hiring in the US, Canada, ... or Brazil
		// only"), so it is stripped here rather than treated as a separator.
		tok = strings.TrimSuffix(tok, " only")
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		// prevTok is always "" — a colliding subdivision code cannot be
		// disambiguated by a neighboring token the way Parse's own comma-token
		// loop does. Deliberate (see title_restriction.go's identical note): a
		// small, safe recall gap, never a guess.
		resolveGeoToken(tok, "", countrySet, regionSet)
	}
	return stringset.Sorted(countrySet), stringset.Sorted(regionSet)
}

// firstAssertedRegionScopePhrase reports the end offset of the first
// regionScopePhrases entry asserted (whole-word, unnegated) in lower, scanning the
// phrases in order — mirrors eligibility.go's phraseAsserted, but returns a position
// instead of a boolean since the caller needs to read the clause that follows.
func firstAssertedRegionScopePhrase(lower string) (end int, ok bool) {
	for _, p := range regionScopePhrases {
		for offset := 0; ; {
			i := strings.Index(lower[offset:], p)
			if i < 0 {
				break
			}
			start := offset + i
			end := start + len(p)
			if wholeWordMatch(lower, start, end) && !negatedSentence(sentenceAround(lower, start, end)) {
				return end, true
			}
			offset = end
		}
	}
	return 0, false
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
