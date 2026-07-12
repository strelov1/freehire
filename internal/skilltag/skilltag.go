// Package skilltag derives a job's technology tags deterministically from its
// free-text (HTML) description.
//
// Like internal/location, it is a curated dictionary, not an extractor: it
// resolves a known vocabulary of languages, frameworks, datastores, and infra by
// alias, and emits nothing for anything it cannot resolve (it never guesses). No
// fuzzy or semantic matching — recall grows by curating the dictionary, not by
// similarity. Tokens are lowercase slugs (go, postgresql, react, kubernetes), the
// same shape the enrichment contract's skills field uses, so the parser and the
// LLM payload speak one vocabulary and union cleanly at read time.
//
// Two resolution rules keep exact matching robust:
//   - Separator-insensitive phrases: a multi-word alias matches its hyphenated,
//     underscored, and spaced forms alike ("distributed-systems" ==
//     "distributed systems"), without collapsing the text — so boundary guards
//     that keep "objective-c" from leaking a bare "c" are preserved.
//   - Case-preserving acronyms: an UPPERCASE acronym resolves while its ambiguous
//     lowercase form does not (ML → machine-learning; ml stays millilitre). A
//     shared tier applies to all text; a résumé-scoped tier (WithResumeAcronyms,
//     e.g. RAG) applies only to résumés so it never tags job facets ("RAG status").
package skilltag

import (
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/stringset"
	"github.com/strelov1/freehire/internal/wordmatch"
)

// htmlTagRE matches an HTML tag; descriptions are raw ATS HTML, so tags are
// replaced with a space before matching to keep markup tokens (div, href) out of
// the result and to avoid gluing words across a tag boundary.
var htmlTagRE = regexp.MustCompile(`<[^>]*>`)

// wordTokenRE splits normalized text into bare alphanumeric tokens for the word
// pass. Punctuated terms (c++, node.js) are handled separately by the phrase pass.
var wordTokenRE = regexp.MustCompile(`[a-z0-9]+`)

// sepRE matches a run of the word-joiners '-'/'_' and whitespace. It is used to
// split a multi-word alias into its segments; the text itself is NOT rewritten, so
// the punctuation that is part of a canonical token (., #, +, /) and the boundary
// guards (a leading '-' is not a word start) are preserved.
var sepRE = regexp.MustCompile(`[-_\s]+`)

// normalize strips HTML tags, lowercases the text, and trims. Tags are replaced
// with a space (not empty) to preserve word boundaries so "<b>Go</b>Engineer"
// cannot fuse. Separators are deliberately left intact — the phrase matcher makes
// '-'/'_'/space equivalent inside multi-word terms without losing the boundary
// information that keeps "objective-c" from leaking a bare "c".
func normalize(text string) string {
	return strings.TrimSpace(strings.ToLower(htmlTagRE.ReplaceAllString(text, " ")))
}

// phraseMatcher resolves one phrase alias against normalized text. A multi-word
// alias compiles to a regex whose inter-segment separators match any run of
// '-'/'_'/whitespace, so "distributed-systems", "distributed_systems", and
// "distributed systems" all resolve to one canonical; a single-token alias
// (c++, node.js, ci/cd) keeps the cheaper substring path.
type phraseMatcher struct {
	canonical string
	re        *regexp.Regexp // multi-segment alias; nil for a single token
	token     string         // single-token alias (used when re == nil)
}

// matches reports whether the alias occurs in norm as a standalone term. Regex
// hits are boundary-checked with the same ASCIIBoundary rule as the substring
// path, so a leading '-' (e.g. the "c" in "objective-c") is not a word start.
func (m phraseMatcher) matches(norm string) bool {
	if m.re == nil {
		return wordmatch.Contains(norm, m.token, wordmatch.ASCIIBoundary)
	}
	for _, loc := range m.re.FindAllStringIndex(norm, -1) {
		if wordmatch.ASCIIBoundary(norm, loc[0], loc[1]) {
			return true
		}
	}
	return false
}

// phraseMatchers compiles phraseAliases once at startup: a multi-word alias (split
// on '-'/'_'/space) becomes a separator-insensitive regex; a single token stays a
// substring match. Only the match key is transformed — canonicals are unchanged.
var phraseMatchers = func() []phraseMatcher {
	out := make([]phraseMatcher, 0, len(phraseAliases))
	for _, p := range phraseAliases {
		segs := nonEmpty(sepRE.Split(strings.ToLower(p.alias), -1))
		if len(segs) <= 1 {
			out = append(out, phraseMatcher{canonical: p.canonical, token: strings.ToLower(p.alias)})
			continue
		}
		quoted := make([]string, len(segs))
		for i, s := range segs {
			quoted[i] = regexp.QuoteMeta(s)
		}
		re := regexp.MustCompile(strings.Join(quoted, `[-_\s]+`))
		out = append(out, phraseMatcher{canonical: p.canonical, re: re})
	}
	return out
}()

// nonEmpty drops empty segments (a leading/trailing separator splits to "").
func nonEmpty(in []string) []string {
	out := in[:0]
	for _, s := range in {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// wordTokens returns the alphanumeric tokens of already-normalized text, in order.
func wordTokens(norm string) []string {
	return wordTokenRE.FindAllString(norm, -1)
}

// Option configures a Parse call. The zero set is job-safe (default).
type Option func(*options)

type options struct {
	resumeAcronyms bool
}

// WithResumeAcronyms enables the résumé-scoped acronym tier (resumeAcronyms, e.g.
// RAG) for a Parse call. Job callers omit it so those acronyms never tag job
// facets; the résumé path (handler.ExtractResumeProfile) sets it.
func WithResumeAcronyms() Option {
	return func(o *options) { o.resumeAcronyms = true }
}

// Parse scans free text and returns the curated canonical skill slugs it contains,
// sorted and deduplicated. Returns nil when nothing resolves. It runs three passes
// that union into a set: a case-preserving acronym pass over the HTML-stripped
// original-case text (shared tier always, résumé tier when opted in), then a phrase
// pass (separator-insensitive) and a word pass over the lowercased, normalized text.
//
// Matches split into two tiers. A "strong" match is any acronym, any phrase, or an
// unambiguous word alias; it always tags. A "weak" match is a word alias listed in
// ambiguousWords (react/swift/spring/networking/…) — an English word that doubles
// as a tech name — and it tags ONLY when the text also carries at least one strong
// match (corroboration). So "must react to changes" on a non-tech post drops react,
// while "React and TypeScript" keeps it. Unambiguous forms (reactjs, "react native",
// "spring boot") stay strong, so a genuinely-named stack never needs corroboration.
func Parse(text string, opts ...Option) []string {
	var o options
	for _, fn := range opts {
		fn(&o)
	}
	strong := map[string]struct{}{}
	weak := map[string]struct{}{}

	// Acronym pass: case-sensitive whole-word match over case-preserved text, so an
	// UPPERCASE acronym resolves while its ambiguous lowercase form does not. Uses a
	// Unicode word boundary because the text is not lowercased here (ASCIIBoundary's
	// word test is lowercase-only and would misjudge uppercase neighbours).
	cased := htmlTagRE.ReplaceAllString(text, " ")
	matchAcronyms(cased, sharedAcronyms, strong)
	if o.resumeAcronyms {
		matchAcronyms(cased, resumeAcronyms, strong)
	}

	norm := normalize(text)
	for _, m := range phraseMatchers {
		if m.matches(norm) {
			strong[m.canonical] = struct{}{}
		}
	}
	for _, tok := range wordTokens(norm) {
		if c, ok := wordAliases[tok]; ok {
			if ambiguousWords[tok] {
				weak[c] = struct{}{}
			} else {
				strong[c] = struct{}{}
			}
		}
	}
	// A weak (ambiguous-word) match survives only when corroborated by a strong tech
	// token in the same text; alone it is English-word noise and is dropped.
	if len(strong) > 0 {
		for c := range weak {
			strong[c] = struct{}{}
		}
	}
	return stringset.Sorted(strong)
}

// matchAcronyms adds the canonical of each acronym whose exact surface form occurs
// as a standalone token in cased (case-preserved) text.
func matchAcronyms(cased string, acronyms map[string]string, set map[string]struct{}) {
	for surface, canonical := range acronyms {
		if wordmatch.Contains(cased, surface, wordmatch.UnicodeBoundary) {
			set[canonical] = struct{}{}
		}
	}
}
