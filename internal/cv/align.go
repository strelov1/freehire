package cv

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/strelov1/freehire/internal/skilltag"
)

// Align rewrites doc so skill wording matches preferred surfaces (canonical →
// JD spelling from skilltag.PreferredFromText). Skills chips and experience
// stacks accept any alias; summary and bullets only unambiguous aliases. Same-
// canonical duplicate chips are collapsed. Pure and deterministic; no LLM.
// The input is not mutated — a rewritten copy is returned. preferred may be nil.
func Align(doc Document, preferred map[string]string) Document {
	if len(preferred) == 0 {
		return doc
	}
	out := doc
	out.Skills = alignSkillGroups(doc.Skills, preferred)
	out.Summary = replaceProse(doc.Summary, preferred)
	if len(doc.Experience) > 0 {
		out.Experience = make([]ExperienceItem, len(doc.Experience))
		copy(out.Experience, doc.Experience)
		for i := range out.Experience {
			out.Experience[i].Stack = alignChipList(doc.Experience[i].Stack, preferred)
			out.Experience[i].Summary = replaceProse(doc.Experience[i].Summary, preferred)
			out.Experience[i].Bullets = alignProseList(doc.Experience[i].Bullets, preferred)
		}
	}
	if len(doc.Projects) > 0 {
		out.Projects = make([]Project, len(doc.Projects))
		copy(out.Projects, doc.Projects)
		for i := range out.Projects {
			out.Projects[i].Bullets = alignProseList(doc.Projects[i].Bullets, preferred)
		}
	}
	return out
}

// AlignChanged reports whether Align would change doc for the given preferred map.
func AlignChanged(doc Document, preferred map[string]string) bool {
	aligned := Align(doc, preferred)
	return !documentsEqualForAlign(doc, aligned)
}

func alignSkillGroups(groups []SkillGroup, preferred map[string]string) []SkillGroup {
	if len(groups) == 0 {
		return groups
	}
	out := make([]SkillGroup, len(groups))
	for i, g := range groups {
		out[i] = SkillGroup{Group: g.Group, Items: collapseIdentical(alignChipList(g.Items, preferred))}
	}
	return out
}

func alignChipList(items []string, preferred map[string]string) []string {
	if len(items) == 0 {
		return items
	}
	out := make([]string, len(items))
	copy(out, items)
	for i, item := range items {
		resolved := skilltag.Canonicalize([]string{item}, skilltag.WithResumeAcronyms())
		if len(resolved) != 1 {
			continue
		}
		want, ok := preferred[resolved[0]]
		if !ok || want == "" {
			continue
		}
		if strings.EqualFold(item, want) {
			// Already the right wording; keep the JD casing.
			out[i] = want
			continue
		}
		out[i] = want
	}
	return out
}

func collapseIdentical(items []string) []string {
	if len(items) < 2 {
		return items
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		key := strings.ToLower(strings.TrimSpace(item))
		if key == "" {
			out = append(out, item)
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func alignProseList(items []string, preferred map[string]string) []string {
	if len(items) == 0 {
		return items
	}
	out := make([]string, len(items))
	for i, s := range items {
		out[i] = replaceProse(s, preferred)
	}
	return out
}

// replaceProse rewrites unambiguous aliases of preferred canonicals in text.
func replaceProse(text string, preferred map[string]string) string {
	if text == "" || len(preferred) == 0 {
		return text
	}
	out := text
	for canonical, want := range preferred {
		if want == "" {
			continue
		}
		for _, alias := range skilltag.AliasesOf(canonical) {
			if !skilltag.IsProseSafeAlias(alias) {
				continue
			}
			if strings.EqualFold(alias, want) {
				// Still rewrite casing to the JD form when the alias matches want ignoring case.
				out = replaceWholeFold(out, alias, want)
				continue
			}
			out = replaceWholeFold(out, alias, want)
		}
	}
	return out
}

// replaceWholeFold replaces whole-token occurrences of from (case-insensitive) with to.
// ASCII alphanumeric boundaries plus a leading '.'/'-' guard (same idea as skilltag).
func replaceWholeFold(text, from, to string) string {
	if from == "" || text == "" {
		return text
	}
	fromLower := strings.ToLower(from)
	var b strings.Builder
	b.Grow(len(text))
	lower := strings.ToLower(text)
	for i := 0; i < len(text); {
		j := strings.Index(lower[i:], fromLower)
		if j < 0 {
			b.WriteString(text[i:])
			break
		}
		j += i
		end := j + len(fromLower)
		// fromLower is ASCII for curated aliases; match length equals the source slice
		// when the original run is the same byte length (true for ASCII casing).
		if end > len(text) {
			b.WriteString(text[i:])
			break
		}
		// Align end to the actual UTF-8 slice of equal lowercase length in text[j:].
		src := text[j:end]
		if strings.ToLower(src) != fromLower {
			// Multi-byte casing mismatch — advance one byte and continue.
			b.WriteByte(text[i])
			i++
			continue
		}
		if !proseBoundary(text, j, end) {
			b.WriteString(text[i : j+1])
			i = j + 1
			continue
		}
		b.WriteString(text[i:j])
		b.WriteString(to)
		i = end
	}
	return b.String()
}

// proseBoundary mirrors wordmatch.ASCIIBoundary but also treats non-ASCII letters
// as word runes so Cyrillic neighbours do not get false hits.
func proseBoundary(s string, start, end int) bool {
	if start > 0 {
		r, _ := utf8.DecodeLastRuneInString(s[:start])
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '-' {
			return false
		}
	}
	if end < len(s) {
		r, _ := utf8.DecodeRuneInString(s[end:])
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func documentsEqualForAlign(a, b Document) bool {
	if a.Summary != b.Summary {
		return false
	}
	if len(a.Skills) != len(b.Skills) {
		return false
	}
	for i := range a.Skills {
		if a.Skills[i].Group != b.Skills[i].Group || !stringSlicesEqual(a.Skills[i].Items, b.Skills[i].Items) {
			return false
		}
	}
	if len(a.Experience) != len(b.Experience) {
		return false
	}
	for i := range a.Experience {
		ae, be := a.Experience[i], b.Experience[i]
		if ae.Summary != be.Summary || !stringSlicesEqual(ae.Bullets, be.Bullets) || !stringSlicesEqual(ae.Stack, be.Stack) {
			return false
		}
	}
	if len(a.Projects) != len(b.Projects) {
		return false
	}
	for i := range a.Projects {
		if !stringSlicesEqual(a.Projects[i].Bullets, b.Projects[i].Bullets) {
			return false
		}
	}
	return true
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
