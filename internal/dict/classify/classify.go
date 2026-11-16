// Package classify derives a job's seniority and role category deterministically from its title. It is a curated dictionary, not a model: it resolves known English, Russian, German, Dutch, Danish, Spanish, Portuguese, French, Polish, Croatian and Serbian, Slovenian, Japanese and Chinese title terms and emits nothing for what it cannot resolve. Canonical values come from the vocabularies the enrichment contract defines (vocab.SeniorityValues / vocab.CategoryValues), so the parser, the enrichment payload and the search facet share one set of values.
package classify

import (
	"regexp"
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
		// The grade is read with the grade-blind role names cut out, so a grade word owned by a role name ("Member of Technical Staff") is not mistaken for the grade. The category reads the untouched title.
		Seniority: matchSeniority(lower),
		Category:  matchCategory(lower),
	}
}

// matchCategory resolves the role category of an already-lowercased title: the ordered table walk plus one translation, categoryNone (the sentinel a blind alias carries) is served as "".
//
// The Brazilian-Portuguese gender parentheticals are folded first, as IsNonTech folds them: "Operador(a) de Caixa" must not be recognised as non-technical yet filed nowhere.
func matchCategory(lower string) string {
	lower = unchefFrench(ptGenderSuffix.Replace(lower))
	c := matchOrdered(cutBackEndShifts(lower), categoryTable)
	if c == "" {
		// Nothing in the main table: try the German (german.go), Dutch, Danish, Spanish, Portuguese (unless the Spanish table vetoed the title), French, Polish and Croatian/Serbian (bcs.go) heads in that order.
		if c := matchHeads(lower, germanCategoryTable); c != "" {
			return c
		}
		if c := matchHeads(lower, dutchCategoryTable); c != "" {
			return c
		}
		if c := matchHeads(lower, danishCategoryTable); c != "" {
			return c
		}
		c = matchSpanishCategory(lower)
		if c == "" {
			c = matchPortugueseCategory(lower)
		}
		if c == "" {
			c = matchFrenchCategory(lower)
		}
		if c == "" {
			c = matchPolishCategory(lower)
		}
		if c == "" {
			c = matchBCSCategory(lower)
		}
	}
	if c == categoryNone {
		return ""
	}
	return c
}

// backEndShiftPatterns are the "back end" that names a SHIFT, not server-side software: a plant or hospital on a compressed week splits it into a front end and a back end, and titles carry the half beside the day or night ("Registered Nurse - Back End Days", "Machine Operator - Back End Relief").
//
// The shift is cut before the category match so the rest of the title decides, unlike a categoryNone pin, which stops the walk. Each span is matched whole on word boundaries, so "Backend Developer - Night Shift" keeps its reading.
var backEndShiftPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\bback[\s-]?end[\s,()-]*(?:days?|nights?|shift|relief)\b`),
	regexp.MustCompile(`\b(?:days?|nights?|cww)[\s,()-]*back[\s-]?end\b`),
}

// cutBackEndShifts replaces every backEndShiftPatterns span with a space, keeping the surrounding word boundaries. Parse and Categories both apply it.
func cutBackEndShifts(lower string) string {
	for _, shift := range backEndShiftPatterns {
		lower = shift.ReplaceAllString(lower, " ")
	}
	return lower
}

// matchSeniority resolves the grade of an already-lowercased title after cutting every gradeBlindPhrases occurrence, replaced by a space so word boundaries survive ("senior member of technical staff" still offers "senior"). Plain substring replacement suffices: every phrase is multi-word. The early-careers desks (earlyCareersDeskPatterns) are cut the same way, by shape.
func matchSeniority(lower string) string {
	for _, phrase := range gradeBlindPhrases {
		lower = strings.ReplaceAll(lower, phrase, " ")
	}
	for _, desk := range earlyCareersDeskPatterns {
		lower = desk.ReplaceAllString(lower, " ")
	}
	if ssrRendering.MatchString(lower) && (!openingSSR.MatchString(lower) || ssrTechniques.MatchString(lower)) {
		lower = ssrWord.ReplaceAllString(lower, " ")
	}
	lower = semiSenior.ReplaceAllString(lower, "ssr")
	if s := matchOrdered(lower, seniorityTable); s != "" {
		return s
	}
	if s := matchOrdered(lower, japaneseSeniorityTable); s != "" {
		return s
	}
	if s := matchOrdered(lower, chineseSeniorityTable); s != "" {
		return s
	}
	// Nothing in the main table: try the Indian rungs and entry-hire words (india.go), then the German, Dutch, Danish, Spanish, Portuguese, French, Polish and Croatian/Serbian grade words in that order.
	if s := matchOrdered(lower, indianSeniorityTable); s != "" {
		return s
	}
	if s := matchHeads(foldGermanSeniority(lower), germanSeniorityTable); s != "" {
		return s
	}
	if s := matchHeads(foldDutchSeniority(lower), dutchSeniorityTable); s != "" {
		return s
	}
	if s := matchHeads(lower, danishSeniorityTable); s != "" {
		return s
	}
	if s := matchOrdered(foldSpanishSeniority(lower), spanishSeniorityTable); s != "" {
		return s
	}
	if s := matchPortugueseSeniority(lower); s != "" {
		return s
	}
	if s := matchFrenchSeniority(lower); s != "" {
		return s
	}
	if s := matchPolishSeniority(lower); s != "" {
		return s
	}
	return matchBCSSeniority(lower)
}

// semiSenior is Latin America's spelled-out middle grade — "Semi Senior",
// "Semi-Senior", "Semisenior", "Semi Sr" — which contains "senior" and would otherwise
// read as it. matchSeniority folds it to "ssr", the abbreviation the same postings use.
var semiSenior = regexp.MustCompile(`\bsemi[\s-]?(?:senior|sr)\b`)

// ssrRendering is a frontend title whose "SSR" is server-side rendering, not the grade:
// "Frontend Engineer (SSR, Next.js)". Such a title's bare "ssr" (ssrWord) is cut first. A
// framework name alone does not qualify: "Desarrollador React Ssr" is a LatAm grade.
var (
	ssrRendering = regexp.MustCompile(`\b(?:next\.?js|nuxt|ssg|csr|server[\s-]side|rendering)\b`)
	ssrWord      = regexp.MustCompile(`\bssr\b`)
	// openingSSR is a title that opens with the grade ("Ssr Fullstack Developer (Node.js +
	// Next.js)"). An opening SSR beside another rendering technique ("SSR / SSG Developer")
	// is still the technique: ssrTechniques.
	openingSSR    = regexp.MustCompile(`^[^\p{L}\p{N}]*ssr\b`)
	ssrTechniques = regexp.MustCompile(`\b(?:ssg|csr|isr|server[\s-]side|rendering)\b`)
)

// Categories resolves every category the text mentions (each alias that occurs as a whole word), distinct and in precedence order. Unlike Parse, which keeps the single strongest, it surfaces the full set a résumé spans (a backend engineer who also does ML). Empty when nothing resolves.
//
// A blind alias contributes nothing: categoryNone is not a category.
func Categories(text string) []string {
	out := matchAllOrdered(cutBackEndShifts(strings.ToLower(text)), categoryTable)
	return slices.DeleteFunc(out, func(c string) bool { return c == categoryNone })
}

// matchAllOrdered returns the distinct canonical values of every alias, in priority order, that occurs as a whole word in title. Aliases sharing a canonical ("backend"/"back-end") are deduplicated, first seen wins.
//
// An occurrence wholly inside the span an EARLIER alias already matched does not count: the table's precedence applied per occurrence. "Front End Cashier" was read by the retail alias declared above the frontend one, and "Information Security Officer" must not pick up the guard category of the "security officer" it contains. A shorter alias that also occurs OUTSIDE every claimed span still counts: "Security Officer, Information Security" names both.
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

// CategoryAliases maps each category canonical to the title aliases that resolve to it (the inverse of the alias table); SeniorityAliases does the same for grades. Exposed so the web role picker can search roles by shorthand ("sre"/"sysadmin"/"sr").
//
// The vocabulary is multilingual (Hungarian, German, Spanish, Portuguese, French, Russian, Polish, Czech, Slovak, Slovenian, Croatian, Serbian, Ukrainian, Bulgarian), so a caller matching these terms against anything but a title should say so: a URL slug is ASCII and needs them transliterated first (see internal/ingest/sources' Profession prefilter).
//
// The blind aliases are omitted: their sentinel is not a category and would show up as a pickable value in the generated web contracts.
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
