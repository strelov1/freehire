package skilltag

import (
	"strings"

	"github.com/strelov1/freehire/internal/stringset"
)

// phraseExact keys phraseAliases by their separator-collapsed lowercase form, for
// whole-token equality rather than the containment Parse needs. "react native",
// "react-native" and "React_Native" therefore share one key, while a sentence that
// merely CONTAINS the alias does not match — the difference between resolving a
// token and mining a text.
var phraseExact = func() map[string]string {
	out := make(map[string]string, len(phraseAliases))
	for _, p := range phraseAliases {
		out[collapseSeparators(strings.ToLower(p.alias))] = p.canonical
	}
	return out
}()

// collapseSeparators reduces every run of '-'/'_'/whitespace to a single space, so
// the three spellings of a multi-word term share one lookup key. Punctuation that
// belongs to a canonical token (., #, +, /) is untouched, exactly as in the phrase
// matcher.
func collapseSeparators(s string) string {
	return strings.TrimSpace(sepRE.ReplaceAllString(s, " "))
}

// canonicalSet is every canonical slug the dictionary can emit, so a caller may
// hand over a canonical form directly.
//
// This is NOT a convenience: a canonical is not automatically an alias of itself.
// "golang" resolves to "go", but the bare token "go" appears nowhere in the alias
// tables — deliberately, because Parse reads prose where "go" is an English verb.
// The assistant, however, is instructed in its system prompt to speak canonical
// slugs ("go, react, kubernetes"), so without this tier the single most common
// backend skill would be dropped from every atom the agent recorded, silently.
// Parse is unaffected: it never consults this set.
var canonicalSet = func() map[string]struct{} {
	out := make(map[string]struct{}, len(wordAliases))
	for _, c := range wordAliases {
		out[c] = struct{}{}
	}
	for _, p := range phraseAliases {
		out[p.canonical] = struct{}{}
	}
	for _, c := range sharedAcronyms {
		out[c] = struct{}{}
	}
	for _, c := range resumeAcronyms {
		out[c] = struct{}{}
	}
	return out
}()

// Canonicalize resolves caller-supplied skill tokens to canonical slugs, sorted and
// deduplicated, returning nil when none resolve. It is the explicit-assertion entry
// to the same dictionary Parse mines free text with, and it differs from Parse in
// exactly one way: the corroboration rule does not apply.
//
// That rule exists because Parse reads prose, where "react" is more likely the
// English verb than the framework unless a strong tech token corroborates it. A
// token a caller hands over — a user ticking a skill, an agent recording an atom's
// skill list — is an assertion about a skill, not a sentence to be interpreted, so
// requiring corroboration would silently drop skills their owner explicitly stated.
//
// The "never guess" rule is unchanged: a token outside the dictionary emits nothing
// rather than passing through as a pseudo-canonical. Nor does Canonicalize mine: a
// token must resolve as a WHOLE (as an acronym, a word alias, or a phrase alias),
// so handing it a sentence yields nothing — that is Parse's job.
func Canonicalize(tokens []string) []string {
	out := make(map[string]struct{}, len(tokens))
	for _, tok := range tokens {
		if c, ok := canonicalizeToken(tok); ok {
			out[c] = struct{}{}
		}
	}
	return stringset.Sorted(out)
}

// canonicalizeToken resolves one whole token, trying the tiers in the same order
// Parse does — case-preserved acronyms first (both tiers: a candidate's own skill
// list is résumé-scoped by definition), then the lowercase word aliases, then the
// separator-insensitive phrases — and finally accepting an already-canonical slug.
func canonicalizeToken(tok string) (string, bool) {
	cased := strings.TrimSpace(stripMarkup(tok))
	if cased == "" {
		return "", false
	}
	if c, ok := sharedAcronyms[cased]; ok {
		return c, true
	}
	if c, ok := resumeAcronyms[cased]; ok {
		return c, true
	}

	norm := normalize(tok)
	if c, ok := wordAliases[norm]; ok {
		return c, true
	}
	if c, ok := phraseExact[collapseSeparators(norm)]; ok {
		return c, true
	}
	if _, ok := canonicalSet[norm]; ok {
		return norm, true
	}
	return "", false
}
