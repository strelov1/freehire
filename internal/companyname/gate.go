// Package companyname resolves a real company display name for boards whose
// ingested company name is still a squished slug (e.g. "lbresearch", "gs1ca",
// "afcb"). The name is sourced deterministically from each ATS's own careers-page
// title or API; a slug is never prettified into a name. The acceptance gate here
// is conservative on purpose: a wrong-but-plausible name reads worse than the
// monogram fallback, so a candidate is applied only when it demonstrably shares
// text with the slug.
package companyname

import (
	"html"
	"regexp"
	"strings"
	"unicode"
)

var (
	// A slug carried into the company field: one token, no whitespace, no
	// uppercase, at least one letter. Hyphens and digits are allowed
	// (chetwood-bank, gs1ca).
	nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)
	words    = regexp.MustCompile(`[A-Za-z0-9]+`)
	jobsAt   = regexp.MustCompile(`(?i)Jobs at (.+?)\s*\|`)
	// A careers-page lead-in that precedes the real name: "Jobs at X",
	// "Careers at X", "Employment Opportunities at X", "Open roles at X".
	leadInAt = regexp.MustCompile(`(?i)^(?:jobs|careers|employment opportunities|open (?:roles|positions|jobs)) at (.+)$`)

	// Titles that look resolvable but are placeholders or artifacts, not a
	// company. The confidence gate catches most of these already; this is a
	// belt-and-braces reject for cases that could otherwise share text.
	junkMarkers = []string{"test platform", "meta recruitment", "just a moment", "not found"}
)

// SlugLike reports whether name is still a squished slug rather than a real
// display name — a single lowercase token with at least one letter.
func SlugLike(name string) bool {
	if name == "" || strings.ContainsFunc(name, unicode.IsSpace) {
		return false
	}
	hasLetter := false
	for _, r := range name {
		if unicode.IsUpper(r) {
			return false
		}
		if unicode.IsLetter(r) {
			hasLetter = true
		}
	}
	return hasLetter
}

// ExtractTitleName pulls a company name out of a careers-page <title>. It
// handles the shapes ATS careers pages use — a "<lead-in> at {Name}" prefix
// (Jobs/Careers/Employment Opportunities at …) and a trailing "{Name} Careers" —
// then cleans a stray " | …" section and collapsed whitespace off the result.
// Returns "" when no shape matches.
func ExtractTitleName(title string) string {
	title = strings.TrimSpace(html.UnescapeString(title))
	switch {
	case jobsAt.MatchString(title):
		return clean(jobsAt.FindStringSubmatch(title)[1])
	case leadInAt.MatchString(title):
		return clean(leadInAt.FindStringSubmatch(title)[1])
	default:
		if rest, ok := cutSuffixFold(title, "Careers"); ok {
			return clean(rest)
		}
		return ""
	}
}

// clean drops a trailing " | …" fragment (careers titles append a section name
// after the company) and collapses runs of whitespace to single spaces.
func clean(s string) string {
	if i := strings.Index(s, " | "); i >= 0 {
		s = s[:i]
	}
	return strings.Join(strings.Fields(s), " ")
}

// Accept decodes and validates a candidate name against the slug. It returns the
// cleaned name and true only when the candidate is non-junk and confidently
// related to the slug (shares a substring or a multi-letter acronym).
func Accept(slug, candidate string) (string, bool) {
	candidate = strings.TrimSpace(html.UnescapeString(candidate))
	// An empty or still-slug-like candidate is no improvement over what's stored,
	// so reject it: applying it would be a no-op write that also keeps the company
	// slug-like, so every subsequent run would re-fetch and re-write it.
	if candidate == "" || SlugLike(candidate) {
		return "", false
	}
	low := strings.ToLower(candidate)
	for _, m := range junkMarkers {
		if strings.Contains(low, m) {
			return "", false
		}
	}
	if !confident(slug, candidate) {
		return "", false
	}
	return candidate, true
}

// confident reports whether candidate demonstrably refers to the same entity as
// slug: the squished forms contain one another, or the candidate's word-initial
// acronym (2+ letters) lines up with the slug. A single-letter acronym is too
// weak — it would match any same-initial name.
func confident(slug, candidate string) bool {
	s := squish(slug)
	r := squish(candidate)
	if s == "" || r == "" {
		return false
	}
	if strings.Contains(r, s) || strings.Contains(s, r) {
		return true
	}
	acr := acronym(candidate)
	if len(acr) >= 2 && (strings.HasPrefix(s, acr) || strings.HasPrefix(acr, s) || acr == s) {
		return true
	}
	return false
}

func squish(s string) string { return nonAlnum.ReplaceAllString(strings.ToLower(s), "") }

func acronym(s string) string {
	var b strings.Builder
	for _, w := range words.FindAllString(s, -1) {
		b.WriteByte(w[0])
	}
	return strings.ToLower(b.String())
}

// cutSuffixFold is strings.CutSuffix but case-insensitive on the suffix.
func cutSuffixFold(s, suffix string) (string, bool) {
	if len(s) >= len(suffix) && strings.EqualFold(s[len(s)-len(suffix):], suffix) {
		return s[:len(s)-len(suffix)], true
	}
	return s, false
}
