package skilltag

import (
	"strings"

	"github.com/strelov1/freehire/internal/platform/stringset"
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

// canonicalSet is every canonical slug reachable through an unambiguous route (a word
// alias, a phrase alias, or a shared acronym), so a caller may hand over that canonical
// form directly.
//
// This is NOT a convenience: a canonical is not automatically an alias of itself.
// "golang" resolves to "go", but the bare token "go" appears nowhere in the alias
// tables — deliberately, because Parse reads prose where "go" is an English verb.
// The assistant, however, is instructed in its system prompt to speak canonical
// slugs ("go, react, kubernetes"), so without this tier the single most common
// backend skill would be dropped from every atom the agent recorded, silently.
// Parse is unaffected: it never consults this set.
//
// resumeAcronyms' canonicals are deliberately excluded, even when a phrase alias also
// names the same canonical (e.g. "rag", reachable via the safe, unambiguous long form
// "retrieval augmented generation"): letting the BARE canonical pass through here would
// bypass the résumé-only acronym gate exactly the way the cased acronym pass is gated,
// since a job-side n-gram caller (cvmatch) can hand over the bare token just as easily
// as the acronym itself. The long phrase itself stays reachable through phraseExact,
// untouched by this — it is unambiguous in either context. canonicalSetResume covers
// the bare form, gated behind WithResumeAcronyms.
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

// canonicalSetResume is the résumé-scoped counterpart of canonicalSet: the canonical
// slugs reachable ONLY through resumeAcronyms, gated behind WithResumeAcronyms for the
// same reason the acronym itself is.
var canonicalSetResume = func() map[string]struct{} {
	out := make(map[string]struct{}, len(resumeAcronyms))
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
//
// The zero set is job-safe (default), mirroring Parse: the résumé-scoped acronym
// tier (resumeAcronyms, e.g. RAG) is OFF unless a caller passes WithResumeAcronyms(),
// because tokens Canonicalize resolves are not always a candidate's own — cvmatch
// canonicalizes n-grams mined from a VACANCY's requirement text, where a résumé-only
// acronym would collide with the same letters' job-side meaning.
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

// canonicalizeToken resolves one whole token, trying the tiers in the same order
// Parse does — case-preserved acronyms first (shared tier always, résumé tier only
// when o.resumeAcronyms), then the lowercase word aliases, then the
// separator-insensitive phrases — and finally accepting an already-canonical slug.
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

// Resolve reads one search-filter value as the skill a posting's `skills` facet would
// carry: a canonical this package emits passes as itself, and anything else goes through
// Canonicalize, so "golang" reaches the postings tagged "go". ok is false for a value
// that is neither — no posting is tagged with it, so filtering on it answers nothing.
//
// Every emitted canonical passes, résumé-scoped and category-scoped acronyms included.
// Canonicalize alone would refuse those (it guards against reading their letters as
// prose), but a filter value is not prose: "rag" is a tag that postings carry, reached
// through "retrieval augmented generation", and refusing it would report a working
// filter as unknown.
func Resolve(value string) (string, bool) {
	norm := normalize(value)
	if _, ok := emittable[norm]; ok {
		return norm, true
	}
	return canonicalizeToken(value, options{})
}
