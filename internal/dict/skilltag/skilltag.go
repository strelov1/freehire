// Package skilltag derives a job's technology tags deterministically from its
// free-text (HTML) description.
//
// Like internal/dict/location, it is a curated dictionary, not an extractor: it
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

	"github.com/strelov1/freehire/internal/dict/wordmatch"
	"github.com/strelov1/freehire/internal/platform/stringset"
)

// htmlTagRE matches an HTML tag; descriptions are raw ATS HTML, so tags are
// replaced with a space before matching to keep markup tokens (div, href) out of
// the result and to avoid gluing words across a tag boundary.
var htmlTagRE = regexp.MustCompile(`<[^>]*>`)

// wordTokenRE splits normalized text into bare alphanumeric tokens for the word
// pass. Punctuated terms (c++, node.js) are handled separately by the phrase pass.
//
// The class is Unicode, not [a-z0-9], even though every alias it is looked up
// against is ASCII: an ASCII-only class ends a token at the first accented letter,
// so an inflected foreign word DECOMPOSES into ASCII fragments and a fragment can
// be an alias — Hungarian "elkészítése" tokenized to elk/sz/t/se and tagged ELK.
// Widening the class only ever makes a token longer, so it removes fragment
// matches and can never lose a term the text really states.
var wordTokenRE = regexp.MustCompile(`[\p{L}\p{N}]+`)

// sepRE matches a run of the word-joiners '-'/'_' and whitespace. It is used to
// split a multi-word alias into its segments; the text itself is NOT rewritten, so
// the punctuation that is part of a canonical token (., #, +, /) and the boundary
// guards (a leading '-' is not a word start) are preserved.
var sepRE = regexp.MustCompile(`[-_\s]+`)

// urlRE matches an absolute URL — a scheme-prefixed or bare "www." link — up to the
// next whitespace. Descriptions embed apply-links whose host and path segments
// tokenize into skill aliases: the ".html" in "about-us.html", a ".php" query. Those
// name a location, not a tech requirement, so URLs are dropped before matching (after
// HTML tags, so hrefs inside <a …> are already gone and only visible link text remains).
var urlRE = regexp.MustCompile(`(?i)\b(?:https?://|www\.)\S+`)

// stripMarkup drops HTML tags and URLs from raw description text, replacing each
// with a space (not empty) to preserve word boundaries so "<b>Go</b>Engineer"
// cannot fuse and a URL's segments cannot tokenize into skill aliases. Used by
// both the case-preserved acronym pass and normalize.
func stripMarkup(text string) string {
	return urlRE.ReplaceAllString(htmlTagRE.ReplaceAllString(text, " "), " ")
}

// normalize strips markup (see stripMarkup), lowercases the text, and trims.
// Separators are deliberately left intact — the phrase matcher makes
// '-'/'_'/space equivalent inside multi-word terms without losing the boundary
// information that keeps "objective-c" from leaking a bare "c".
func normalize(text string) string {
	return strings.TrimSpace(strings.ToLower(stripMarkup(text)))
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
// hits are boundary-checked with the same TechTermBoundary rule as the substring
// path, so a leading '-' (e.g. the "c" in "objective-c") is not a word start.
func (m phraseMatcher) matches(norm string) bool {
	if m.re == nil {
		return wordmatch.Contains(norm, m.token, wordmatch.TechTermBoundary)
	}
	for _, loc := range m.re.FindAllStringIndex(norm, -1) {
		if wordmatch.TechTermBoundary(norm, loc[0], loc[1]) {
			return true
		}
	}
	return false
}

// compilePhraseMatcher turns one lowercase alias into its matcher: a multi-word alias
// (split on '-'/'_'/space) becomes a separator-insensitive regex; a single token stays a
// substring match. Only the match key is transformed — the canonical is unchanged.
func compilePhraseMatcher(canonical, alias string) phraseMatcher {
	segs := nonEmpty(sepRE.Split(alias, -1))
	if len(segs) <= 1 {
		return phraseMatcher{canonical: canonical, token: alias}
	}
	quoted := make([]string, len(segs))
	for i, s := range segs {
		quoted[i] = regexp.QuoteMeta(s)
	}
	return phraseMatcher{canonical: canonical, re: regexp.MustCompile(strings.Join(quoted, `[-_\s]+`))}
}

// phraseMatchers compiles phraseAliases once at startup.
var phraseMatchers = func() []phraseMatcher {
	out := make([]phraseMatcher, 0, len(phraseAliases))
	for _, p := range phraseAliases {
		out = append(out, compilePhraseMatcher(p.canonical, strings.ToLower(p.alias)))
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

// Option configures a Parse or Canonicalize call. The zero set is job-safe (default).
type Option func(*options)

type options struct {
	resumeAcronyms  bool
	acronymCategory string
}

// WithResumeAcronyms enables the résumé-scoped acronym tier (resumeAcronyms, e.g.
// RAG) for a Parse or Canonicalize call. Job/vacancy callers omit it so those
// acronyms never tag job facets; a caller resolving a candidate's own asserted
// skills (handler.ExtractResumeProfile, an experience atom's Skills/Stack, a
// profile's claimed skills) sets it.
func WithResumeAcronyms() Option {
	return func(o *options) { o.resumeAcronyms = true }
}

// WithAcronymCategory enables the category-scoped acronym tier
// (categoryScopedAcronyms, e.g. RAG) for a Parse call, resolving an acronym
// only when category is on that acronym's own allow-list. The job ingest path
// (internal/job/jobderive) passes the job's already-resolved category; every other
// caller omits it, so the tier stays off by default.
func WithAcronymCategory(category string) Option {
	return func(o *options) { o.acronymCategory = category }
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
	// Emitted unconditionally, but NOT counted as corroboration. A discipline phrase
	// is certain enough to tag on its own, yet it is a concept rather than a named
	// technology, so it cannot vouch for the gated single-word canonicals.
	standalone := map[string]struct{}{}

	// Acronym pass: case-sensitive whole-word match over case-preserved text, so an
	// UPPERCASE acronym resolves while its ambiguous lowercase form does not. It takes
	// the plain word boundary rather than TechTermBoundary because an acronym surface
	// carries no punctuation, so the dotted/hyphenated-suffix guard has nothing to
	// protect here.
	cased := stripMarkup(text)
	matchAcronyms(cased, sharedAcronyms, strong)
	if o.resumeAcronyms {
		matchAcronyms(cased, resumeAcronyms, strong)
	}
	// Inlined rather than routed through matchAcronyms: its map value is a bare
	// canonical string, but a categoryScopedAcronym also carries the allow-list
	// matchAcronyms has no way to consult.
	if o.acronymCategory != "" {
		for surface, ca := range categoryScopedAcronyms {
			if ca.allowedCategories[o.acronymCategory] && wordmatch.Contains(cased, surface, wordmatch.UnicodeBoundary) {
				strong[ca.canonical] = struct{}{}
			}
		}
	}

	norm := normalize(text)
	for _, m := range phraseMatchers {
		if m.matches(norm) {
			// A phrase that names a discipline rather than a technology tags itself
			// but must not rescue the gated single-word canonicals: "AI-powered
			// content marketing" describes the prose, not an AI requirement.
			if nonCorroboratingPhrases[m.canonical] {
				standalone[m.canonical] = struct{}{}
			} else {
				strong[m.canonical] = struct{}{}
			}
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
	// token in the same text; alone it is English-word noise and is dropped. The
	// standalone set is added afterwards, so it can never act as that corroborator.
	if len(strong) > 0 {
		for c := range weak {
			strong[c] = struct{}{}
		}
	}
	for c := range standalone {
		strong[c] = struct{}{}
	}
	return stringset.Sorted(strong)
}

// HasEngineering reports whether any of the canonicals in skills names engineering
// work. It exists because "carries a tagged skill" is not the same claim as "is a
// technical posting": the dictionary deliberately covers the recruiting, HR, finance,
// legal, operations and customer-success roles a technical company hires for, so a
// recruiting coordinator comes back tagged {stakeholder-management,
// candidate-experience} — a correct facet, and no evidence at all about the employer.
//
// A canonical the dictionary does not place is treated as engineering. Every caller so
// far is deciding whether to act against a company on the grounds that it has never
// posted anything technical, and for that decision an unrecognised skill must not read
// as proof of absence.
func HasEngineering(skills []string) bool {
	for _, s := range skills {
		if !nonEngineeringCanonicals[s] {
			return true
		}
	}
	return false
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
