package skilltag

import (
	"strings"

	"github.com/strelov1/freehire/internal/platform/stringset"
)

// phraseExact keys phraseAliases by their separator-collapsed lowercase form, for whole-token
// equality rather than the containment Parse needs: "react native", "react-native" and
// "React_Native" share one key, while a sentence merely CONTAINING the alias does not match.
var phraseExact = func() map[string]string {
	out := make(map[string]string, len(phraseAliases))
	for _, p := range phraseAliases {
		out[collapseSeparators(strings.ToLower(p.alias))] = p.canonical
	}
	return out
}()

// collapseSeparators reduces every run of '-'/'_'/whitespace to a single space. Punctuation that
// belongs to a canonical token (., #, +, /) is untouched, as in the phrase matcher.
func collapseSeparators(s string) string {
	return strings.TrimSpace(sepRE.ReplaceAllString(s, " "))
}

// canonicalSet is every canonical slug reachable through an unambiguous route (a word alias, a
// phrase alias, or a shared acronym), so a caller may hand over that canonical form directly.
//
// A canonical is not automatically an alias of itself: "golang" resolves to "go", but the bare
// token "go" is in no alias table, because Parse reads prose where "go" is a verb. The assistant
// speaks canonical slugs ("go, react, kubernetes"), so without this tier "go" would be silently
// dropped. Parse never consults this set.
//
// resumeAcronyms' canonicals are excluded even when a phrase alias names the same canonical ("rag"
// via "retrieval augmented generation"): passing the BARE canonical here would bypass the
// résumé-only acronym gate, since a job-side n-gram caller (cvmatch) can hand over the bare token.
// The long phrase stays reachable through phraseExact; canonicalSetResume covers the bare form,
// gated behind WithResumeAcronyms.
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
		delete(out, c)
	}
	return out
}()

// canonicalSetResume is the résumé-scoped counterpart of canonicalSet: the canonicals reachable
// ONLY through resumeAcronyms, gated behind WithResumeAcronyms.
var canonicalSetResume = func() map[string]struct{} {
	out := make(map[string]struct{}, len(resumeAcronyms))
	for _, c := range resumeAcronyms {
		out[c] = struct{}{}
	}
	return out
}()

// Canonicalize resolves caller-supplied skill tokens to canonical slugs, sorted and deduplicated,
// returning nil when none resolve. It is the explicit-assertion entry to the dictionary Parse
// mines free text with, and differs from Parse in one way: the corroboration rule does not apply,
// because a token a caller hands over (a user ticking a skill, an agent recording an atom) is an
// assertion about a skill, not prose, and requiring corroboration would drop skills their owner
// stated.
//
// "Never guess" is unchanged: a token outside the dictionary emits nothing. Nor does it mine: a
// token must resolve as a WHOLE (acronym, word alias or phrase alias); a sentence yields nothing.
//
// Like Parse, the zero set is job-safe: the résumé-scoped acronym tier (resumeAcronyms, e.g. RAG)
// is OFF unless WithResumeAcronyms() is passed, because cvmatch canonicalizes n-grams mined from
// a VACANCY's requirement text, where a résumé-only acronym would collide with its job-side meaning.
func Canonicalize(tokens []string, opts ...Option) []string {
	var o options
	for _, fn := range opts {
		fn(&o)
	}
	out := make(map[string]struct{}, len(tokens))
	for _, tok := range tokens {
		if c, ok := canonicalizeToken(tok, o); ok {
			out[c] = struct{}{}
		}
	}
	return stringset.Sorted(out)
}

// canonicalizeToken resolves one whole token, trying the tiers in Parse's order: case-preserved
// acronyms (shared always, résumé only when o.resumeAcronyms), lowercase word aliases,
// separator-insensitive phrases, then an already-canonical slug.
func canonicalizeToken(tok string, o options) (string, bool) {
	cased := strings.TrimSpace(stripMarkup(tok))
	if cased == "" {
		return "", false
	}
	if c, ok := sharedAcronyms[cased]; ok {
		return c, true
	}
	if o.resumeAcronyms {
		if c, ok := resumeAcronyms[cased]; ok {
			return c, true
		}
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
	if o.resumeAcronyms {
		if _, ok := canonicalSetResume[norm]; ok {
			return norm, true
		}
	}
	return "", false
}

// emittable is Canonicals as a set, built once for Resolve.
var emittable = func() map[string]struct{} {
	out := map[string]struct{}{}
	for _, c := range Canonicals() {
		out[c] = struct{}{}
	}
	return out
}()

// Resolve reads one search-filter value as the skill a posting's `skills` facet would carry: a
// canonical this package emits passes as itself, anything else goes through Canonicalize (so
// "golang" reaches postings tagged "go"). ok is false for a value that is neither.
//
// Every emitted canonical passes, résumé-scoped and category-scoped acronyms included: a filter
// value is not prose, and "rag" is a tag postings carry, so refusing it would report a working
// filter as unknown.
func Resolve(value string) (string, bool) {
	norm := normalize(value)
	if _, ok := emittable[norm]; ok {
		return norm, true
	}
	for surface, canonical := range sharedAcronyms {
		if strings.EqualFold(norm, surface) {
			return canonical, true
		}
	}
	// A filter selects a skill rather than mining prose: accept the acronyms the glossary advertises
	// even when Parse needs a category to disambiguate them (TPM can mean technical program manager).
	for surface, scoped := range categoryScopedAcronyms {
		if strings.EqualFold(norm, surface) {
			return scoped.canonical, true
		}
	}
	return canonicalizeToken(value, options{})
}
