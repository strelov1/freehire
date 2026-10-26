// Package classify derives a job's seniority and role category deterministically
// from its title. It is a curated dictionary, not a model: it resolves known
// English, Russian, German, Dutch, Danish and Japanese title terms and emits nothing for what it cannot resolve
// (it never guesses). Canonical values are drawn from the same controlled
// vocabularies the enrichment contract defines (vocab.SeniorityValues /
// vocab.CategoryValues), so the parser, the enrichment payload, and the search
// facet all speak one set of values — the same doctrine as internal/dict/location.
package classify

import (
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/dict/wordmatch"
)

// Classification is the seniority and role category parsed from a job title.
// Each field is "" when the title states nothing the dictionary resolves.
type Classification struct {
	Seniority string // "" or one of vocab.SeniorityValues
	Category  string // "" or one of vocab.CategoryValues
}

// Parse resolves a job title to its seniority and category. It never guesses;
// an unresolved field is "".
func Parse(title string) Classification {
	lower := strings.ToLower(title)
	return Classification{
		// The grade is read from the title with the grade-blind role names cut out,
		// so a grade word owned by a role name ("Member of Technical Staff") cannot
		// be mistaken for the grade itself. The category reads the untouched title:
		// no categoryTable alias hides inside those phrases.
		Seniority: matchSeniority(lower),
		Category:  matchCategory(lower),
	}
}

// matchCategory resolves the role category of an already-lowercased title. It is the
// ordered table walk plus one translation: categoryNone, the sentinel a blind alias
// carries, is served as "" (see the categoryNone doc comment).
//
// The Brazilian-Portuguese gender parentheticals are folded first, as IsNonTech folds
// them: "Operador(a) de Caixa" is the common written form, and a category reader that
// saw it differently from the non-tech reader would leave the posting recognised as
// non-technical but filed nowhere — which the ingest filter refuses.
func matchCategory(lower string) string {
	lower = ptGenderSuffix.Replace(lower)
	c := matchOrdered(lower, categoryTable)
	if c == "" {
		// Nothing in the main table: try the German compound heads (german.go),
		// then the Dutch ones (dutch.go), then the Danish ones (danish.go).
		if c := matchHeads(lower, germanCategoryTable); c != "" {
			return c
		}
		if c := matchHeads(lower, dutchCategoryTable); c != "" {
			return c
		}
		return matchHeads(lower, danishCategoryTable)
	}
	if c == categoryNone {
		return ""
	}
	return c
}

// matchSeniority resolves the grade of an already-lowercased title, first cutting
// out every gradeBlindPhrases occurrence. Each phrase is replaced by a space rather
// than removed, so the word boundaries of the surrounding terms survive and
// "senior member of technical staff" still offers a matchable "senior". Plain
// substring replacement suffices: every phrase is multi-word, so it cannot occur
// inside a single token the way a bare alias could. The early-careers desks
// (earlyCareersDeskPatterns) are cut the same way, by shape rather than by phrase.
func matchSeniority(lower string) string {
	for _, phrase := range gradeBlindPhrases {
		lower = strings.ReplaceAll(lower, phrase, " ")
	}
	for _, desk := range earlyCareersDeskPatterns {
		lower = desk.ReplaceAllString(lower, " ")
	}
	if s := matchOrdered(lower, seniorityTable); s != "" {
		return s
	}
	if s := matchOrdered(lower, japaneseSeniorityTable); s != "" {
		return s
	}
	// Nothing in the main table: try the Indian rungs and entry-hire words (india.go),
	// then the German grade words and heads (german.go), then the Dutch ones (dutch.go),
	// then the Danish ones (danish.go).
	if s := matchOrdered(lower, indianSeniorityTable); s != "" {
		return s
	}
	if s := matchHeads(foldGermanSeniority(lower), germanSeniorityTable); s != "" {
		return s
	}
	if s := matchHeads(foldDutchSeniority(lower), dutchSeniorityTable); s != "" {
		return s
	}
	return matchHeads(lower, danishSeniorityTable)
}

// Categories resolves every category the text mentions — each alias that occurs as a
// whole word — distinct and in precedence order (the primary category first). Unlike
// Parse, which keeps only the single strongest category, this surfaces the full set a
// résumé spans (a backend engineer who also does ML). Empty when nothing resolves; it
// never guesses.
//
// A blind alias contributes nothing here either: categoryNone is not a category, so it
// is dropped rather than surfaced as a value in a CV profile.
func Categories(text string) []string {
	out := matchAllOrdered(strings.ToLower(text), categoryTable)
	return slices.DeleteFunc(out, func(c string) bool { return c == categoryNone })
}

// matchAllOrdered returns the distinct canonical values of every alias (in priority
// order) that occurs as a whole word in title. Several aliases can share a canonical
// ("backend"/"back-end"), so results are deduplicated while preserving first-seen order.
//
// An occurrence lying wholly inside the span an EARLIER alias already matched does not
// count. That is the table's precedence, applied per occurrence rather than per title:
// "Front End Cashier" is the checkout lane, and its "front end" was read by the retail
// alias declared above the frontend one, so it cannot be read again as a developer.
// Without it every pin in the table would hold for Parse and leak through here — the
// infosec "Information Security Officer" would come back carrying the guard category
// of the "security officer" it contains. A shorter alias that also occurs OUTSIDE every
// claimed span still counts: "Security Officer, Information Security" names both.
func matchAllOrdered(title string, table []aliasEntry) []string {
	var out []string
	seen := map[string]bool{}
	var claimed [][2]int
	for _, e := range table {
		hit := false
		for _, span := range occurrences(title, e.alias) {
			if !within(span, claimed) {
				hit = true
			}
			claimed = append(claimed, span)
		}
		if hit && !seen[e.canonical] {
			seen[e.canonical] = true
			out = append(out, e.canonical)
		}
	}
	return out
}

// occurrences returns the [start,end) spans of every whole-word occurrence of term in s.
func occurrences(s, term string) [][2]int {
	var out [][2]int
	for from := 0; from < len(s); {
		i := strings.Index(s[from:], term)
		if i < 0 {
			break
		}
		start := from + i
		end := start + len(term)
		if titleBoundary(s, start, end) {
			out = append(out, [2]int{start, end})
		}
		from = start + 1
	}
	return out
}

// within reports whether span lies wholly inside one of the claimed spans.
func within(span [2]int, claimed [][2]int) bool {
	for _, c := range claimed {
		if c[0] <= span[0] && span[1] <= c[1] {
			return true
		}
	}
	return false
}

// CategoryAliases maps each category canonical to the title aliases that resolve
// to it (the inverse of the internal alias table); SeniorityAliases does the same
// for grades. Exposed so the web role picker can search roles by shorthand — the
// same curated terms used to tag titles, so "sre"/"sysadmin"/"sr" find their role
// rather than only its display label.
//
// The vocabulary is not English-only and has not been for some time: the table
// carries Hungarian, German, Spanish, Portuguese, French, Russian, Polish, Czech,
// Slovak, Slovenian, Croatian, Serbian, Ukrainian and Bulgarian forms where prod
// titles showed them in use. A caller that matches these terms against
// anything but a title should say so — matching them against a URL slug, say,
// means transliterating them first (see internal/ingest/sources' Profession
// prefilter), because a slug is ASCII and "szoftverfejlesztő" is not.
//
// The blind aliases are omitted: their sentinel is not a category, and it feeds the
// generated web contracts, where it would show up as a pickable value.
func CategoryAliases() map[string][]string {
	out := invertAliases(categoryTable)
	delete(out, categoryNone)
	return out
}

func SeniorityAliases() map[string][]string { return invertAliases(seniorityTable) }

func invertAliases(table []aliasEntry) map[string][]string {
	out := make(map[string][]string, len(table))
	for _, e := range table {
		out[e.canonical] = append(out[e.canonical], e.alias)
	}
	return out
}

// matchOrdered returns the canonical value of the first alias (in priority order)
// that occurs as a whole word in title, or "" if none match. Ordering encodes
// precedence: the most specific / highest-rank alias is checked first, so a title
// carrying several grade words ("Lead Senior") resolves the stronger one.
func matchOrdered(title string, table []aliasEntry) string {
	for _, e := range table {
		if wordmatch.Contains(title, e.alias, titleBoundary) {
			return e.canonical
		}
	}
	return ""
}
