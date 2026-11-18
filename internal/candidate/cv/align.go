package cv

import (
	"cmp"
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/dict/skilltag"
	"github.com/strelov1/freehire/internal/dict/wordmatch"
)

// Align rewrites doc so skill wording matches the spellings the vacancy used (canonical →
// spelling, from skilltag.PreferredFromText) and reports whether anything changed. Chips and
// experience stacks match through the full alias tables; summary and bullet prose only
// through spellings that cannot be ordinary English. Same-skill duplicates the rewrite
// creates are collapsed. Pure, deterministic, idempotent, no LLM; the input is not mutated.
// preferred may be nil.
func Align(doc Document, preferred map[string]string) (Document, bool) {
	if len(preferred) == 0 {
		return doc, false
	}
	rules := proseRules(preferred)
	out := doc
	out.Skills = alignSkillGroups(doc.Skills, preferred)
	out.Summary = replaceProse(doc.Summary, rules)
	if len(doc.Experience) > 0 {
		out.Experience = make([]ExperienceItem, len(doc.Experience))
		copy(out.Experience, doc.Experience)
		for i := range out.Experience {
			out.Experience[i].Stack = alignChips(doc.Experience[i].Stack, preferred)
			out.Experience[i].Summary = replaceProse(doc.Experience[i].Summary, rules)
			out.Experience[i].Bullets = alignProseList(doc.Experience[i].Bullets, rules)
		}
	}
	if len(doc.Projects) > 0 {
		out.Projects = make([]Project, len(doc.Projects))
		copy(out.Projects, doc.Projects)
		for i := range out.Projects {
			out.Projects[i].Bullets = alignProseList(doc.Projects[i].Bullets, rules)
		}
	}
	return out, !documentsEqualForAlign(doc, out)
}

func alignSkillGroups(groups []SkillGroup, preferred map[string]string) []SkillGroup {
	if len(groups) == 0 {
		return groups
	}
	out := make([]SkillGroup, len(groups))
	for i, g := range groups {
		out[i] = SkillGroup{Group: g.Group, Items: alignChips(g.Items, preferred)}
	}
	return out
}

// alignChips rewrites each chip that resolves to a canonical the vacancy spelled its own way,
// then drops the duplicates that creates. A chip already in the vacancy's spelling keeps the
// candidate's casing.
func alignChips(items []string, preferred map[string]string) []string {
	if len(items) == 0 {
		return items
	}
	out := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if resolved := skilltag.Canonicalize([]string{item}, skilltag.WithResumeAcronyms()); len(resolved) == 1 {
			if want := preferred[resolved[0]]; want != "" && !strings.EqualFold(item, want) {
				item = want
			}
		}
		if key := strings.ToLower(strings.TrimSpace(item)); key != "" {
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
		}
		out = append(out, item)
	}
	return out
}

func alignProseList(items []string, rules []proseRule) []string {
	if len(items) == 0 {
		return items
	}
	out := make([]string, len(items))
	for i, s := range items {
		out[i] = replaceProse(s, rules)
	}
	return out
}

// proseRule rewrites one spelling of a skill into the spelling the vacancy used. from is
// ASCII-lowercase, so it can be compared against asciiLower(text) at the same offsets.
type proseRule struct{ from, to string }

// proseRules flattens preferred into the replacements a single pass over prose applies. The
// order is derived only from the rules, so map iteration cannot reach the output; longest
// first keeps "node" from matching inside "node.js".
func proseRules(preferred map[string]string) []proseRule {
	var rules []proseRule
	for canonical, want := range preferred {
		if want == "" {
			continue
		}
		for _, spelling := range skilltag.ProseSurfaces(canonical) {
			for _, from := range separatorVariants(strings.ToLower(spelling)) {
				// Skip prose already in the vacancy's spelling (rewriting would only impose its casing),
				// checked per separator spelling so "infrastructure-as-code" still rewrites to a spaced form.
				if strings.EqualFold(from, want) {
					continue
				}
				rules = append(rules, proseRule{from: from, to: want})
			}
		}
	}
	slices.SortFunc(rules, func(a, b proseRule) int {
		return cmp.Or(
			cmp.Compare(len(b.from), len(a.from)),
			strings.Compare(a.from, b.from),
			strings.Compare(a.to, b.to),
		)
	})
	return rules
}

// separatorVariants spells a multi-word term the three ways a CV writes it; prose rewriting
// compares bytes, so it needs them up front.
func separatorVariants(spelling string) []string {
	if !strings.Contains(spelling, " ") {
		return []string{spelling}
	}
	return []string{
		spelling,
		strings.ReplaceAll(spelling, " ", "-"),
		strings.ReplaceAll(spelling, " ", "_"),
	}
}

// replaceProse applies rules in one left-to-right pass. The scan continues after the matched
// SOURCE span, so no replacement feeds another rule; that keeps a second alignment a no-op
// (sequential replace turned "REST API" into "REST APIs APIs").
func replaceProse(text string, rules []proseRule) string {
	if text == "" || len(rules) == 0 {
		return text
	}
	lower := asciiLower(text)
	var b strings.Builder
	b.Grow(len(text))
	for i := 0; i < len(text); {
		if r, ok := ruleAt(text, lower, i, rules); ok {
			b.WriteString(r.to)
			i += len(r.from)
			continue
		}
		b.WriteByte(text[i])
		i++
	}
	return b.String()
}

// ruleAt returns the first rule whose spelling starts at i as a whole word. rules are
// longest-first, so "node.js" is preferred over the "node" inside it.
func ruleAt(text, lower string, i int, rules []proseRule) (proseRule, bool) {
	for _, r := range rules {
		end := i + len(r.from)
		if end <= len(text) && lower[i:end] == r.from && wordmatch.TechTermBoundary(text, i, end) {
			return r, true
		}
	}
	return proseRule{}, false
}

// asciiLower lowercases only ASCII letters so the result indexes byte-for-byte into its
// source. strings.ToLower is not length-preserving (U+023A grows a byte), and mixing its
// offsets with the original once disabled the rest of a bullet or ran off its end.
func asciiLower(s string) string {
	b := []byte(s)
	for i := 0; i < len(b); i++ {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}

func documentsEqualForAlign(a, b Document) bool {
	if a.Summary != b.Summary {
		return false
	}
	if len(a.Skills) != len(b.Skills) {
		return false
	}
	for i := range a.Skills {
		if a.Skills[i].Group != b.Skills[i].Group || !slices.Equal(a.Skills[i].Items, b.Skills[i].Items) {
			return false
		}
	}
	if len(a.Experience) != len(b.Experience) {
		return false
	}
	for i := range a.Experience {
		ae, be := a.Experience[i], b.Experience[i]
		if ae.Summary != be.Summary || !slices.Equal(ae.Bullets, be.Bullets) || !slices.Equal(ae.Stack, be.Stack) {
			return false
		}
	}
	if len(a.Projects) != len(b.Projects) {
		return false
	}
	for i := range a.Projects {
		if !slices.Equal(a.Projects[i].Bullets, b.Projects[i].Bullets) {
			return false
		}
	}
	return true
}
