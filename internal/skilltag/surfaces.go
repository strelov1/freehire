package skilltag

import (
	"strings"
	"unicode/utf8"

	"github.com/strelov1/freehire/internal/wordmatch"
)

// PreferredFromText returns each canonical skill found in text mapped to the
// surface form the text actually used, preserving casing. When several aliases of
// one canonical appear, the longest surface wins (rune count). Surfaces come only
// from the curated alias tables — unknown phrases are ignored. Parse's return
// shape is unchanged; this is the invert path for JD-driven wording alignment.
func PreferredFromText(text string) map[string]string {
	cased := strings.TrimSpace(stripMarkup(text))
	if cased == "" {
		return nil
	}
	norm := strings.ToLower(cased)

	best := map[string]string{}
	consider := func(canonical, surface string) {
		if canonical == "" || surface == "" {
			return
		}
		if prev, ok := best[canonical]; ok && utf8.RuneCountInString(surface) <= utf8.RuneCountInString(prev) {
			return
		}
		best[canonical] = surface
	}

	// Mirror Parse's strong/weak split: ambiguous English-word aliases only count
	// when a strong tech token corroborates them, so "must react to changes" does
	// not prefer "react" as a JD skill surface.
	type hit struct {
		canonical, surface string
		weak               bool
	}
	var hits []hit
	strong := false

	for surface, canonical := range sharedAcronyms {
		if loc := findBounded(cased, surface, wordmatch.UnicodeBoundary); loc != nil {
			hits = append(hits, hit{canonical, cased[loc[0]:loc[1]], false})
			strong = true
		}
	}
	for _, m := range phraseMatchers {
		for _, loc := range findAllPhrase(norm, m) {
			surface := cased[loc[0]:loc[1]]
			if nonCorroboratingPhrases[m.canonical] {
				hits = append(hits, hit{m.canonical, surface, false})
				continue
			}
			hits = append(hits, hit{m.canonical, surface, false})
			strong = true
		}
	}
	for _, loc := range wordTokenRE.FindAllStringIndex(norm, -1) {
		tok := norm[loc[0]:loc[1]]
		c, ok := wordAliases[tok]
		if !ok {
			continue
		}
		surface := cased[loc[0]:loc[1]]
		if ambiguousWords[tok] {
			hits = append(hits, hit{c, surface, true})
			continue
		}
		hits = append(hits, hit{c, surface, false})
		strong = true
	}
	for _, h := range hits {
		if h.weak && !strong {
			continue
		}
		consider(h.canonical, h.surface)
	}
	if len(best) == 0 {
		return nil
	}
	return best
}

// AliasesOf returns every curated lowercase alias that resolves to canonical,
// including the canonical slug itself when it is a recognised form. Order is
// longest-first so whole-phrase replaces run before shorter acronyms.
func AliasesOf(canonical string) []string {
	if canonical == "" {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	add := func(a string) {
		a = strings.TrimSpace(strings.ToLower(a))
		if a == "" {
			return
		}
		if _, ok := seen[a]; ok {
			return
		}
		seen[a] = struct{}{}
		out = append(out, a)
	}
	add(canonical)
	for alias, c := range wordAliases {
		if c == canonical {
			add(alias)
		}
	}
	for _, p := range phraseAliases {
		if p.canonical == canonical {
			add(p.alias)
		}
	}
	for surface, c := range sharedAcronyms {
		if c == canonical {
			add(surface)
		}
	}
	for surface, ca := range categoryScopedAcronyms {
		if ca.canonical == canonical {
			add(surface)
		}
	}
	for surface, c := range resumeAcronyms {
		if c == canonical {
			add(surface)
		}
	}
	// Longest first so "infrastructure as code" wins over "iac" when both are tried.
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if utf8.RuneCountInString(out[j]) > utf8.RuneCountInString(out[i]) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// IsProseSafeAlias reports whether alias may be rewritten inside summary/bullet
// prose. Multi-word phrases and strong acronyms are safe; ambiguous English-word
// aliases and one- or two-letter tokens are not (they collide with ordinary prose).
func IsProseSafeAlias(alias string) bool {
	a := strings.TrimSpace(strings.ToLower(alias))
	if a == "" {
		return false
	}
	if strings.ContainsAny(a, " \t_-/") {
		// Phrases and punctuated tokens (ci/cd, c++) — not bare English words.
		return true
	}
	if ambiguousWords[a] {
		return false
	}
	if utf8.RuneCountInString(a) <= 2 {
		return false
	}
	return true
}

// findBounded returns the [start,end) of the first whole-token occurrence of term
// in s, or nil.
func findBounded(s, term string, ok wordmatch.Boundary) []int {
	if term == "" {
		return nil
	}
	for from := 0; ; {
		i := strings.Index(s[from:], term)
		if i < 0 {
			return nil
		}
		i += from
		end := i + len(term)
		if ok(s, i, end) {
			return []int{i, end}
		}
		from = i + 1
	}
}

// findAllPhrase returns every boundary-checked match of m in norm.
func findAllPhrase(norm string, m phraseMatcher) [][]int {
	var out [][]int
	if m.re == nil {
		for from := 0; ; {
			i := strings.Index(norm[from:], m.token)
			if i < 0 {
				break
			}
			i += from
			end := i + len(m.token)
			if wordmatch.ASCIIBoundary(norm, i, end) {
				out = append(out, []int{i, end})
			}
			from = i + 1
		}
		return out
	}
	for _, loc := range m.re.FindAllStringIndex(norm, -1) {
		if wordmatch.ASCIIBoundary(norm, loc[0], loc[1]) {
			out = append(out, loc)
		}
	}
	return out
}
