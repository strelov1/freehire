package classify

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/strelov1/freehire/internal/dict/normalize"
)

// Croatian, Serbian, Bosnian and Montenegrin (BCS) titles share one table. Two things set
// them apart.
//
// First, the words INFLECT: a title names its role in the nominative ("Voditelj trgovine")
// but the field after it is in the genitive, locative or instrumental ("Rad u skladištu").
// The main table lists whole surface forms, so an alias here may end a word in "*", the
// stem plus one of the case and gender endings in bcsEndings ("prodaj*" is prodaja,
// prodaje, prodaji, prodaju, prodajom). The endings are a closed list, so a stem never
// reaches into an unrelated longer word.
//
// Second, Serbian is written in two scripts. A Cyrillic title ("Програмер") is
// transliterated to Latin (normalize.LatinizeSerbian) before this table reads it, but only
// when it shows it is Serbian (looksSerbianCyrillic); Russian, Ukrainian and Bulgarian
// titles have their own Cyrillic aliases in the main table.
//
// Aliases match a FOLDED title: transliterated, gender forms cut ("(m/ž)", "Voditelj/ica",
// "MLAĐI/A"), diacritics dropped, đ written "dj" (boards print both "Vozač" and "Vozac").
// Before this table, the main table is asked once more of the transliterated, gender-cut
// title, so "Програмер" finds "programer".
//
// The table is a FALLBACK: it runs only after every other table resolved nothing, so no
// existing answer changes. Every entry must also be safe on the English, Slovenian,
// Polish, Czech and Slovak titles that reach the same fallback; risky ones are argued
// beside their entry.
// bcsEndings are the case and gender endings a "*" stem may take. They cover the
// nominative, genitive, dative, accusative, instrumental and locative of the noun and
// adjective declensions in both numbers, and the feminine agent suffixes (-ica, -ka,
// -kinja) with their own endings.
var bcsEndings = map[string]bool{
	"": true, "a": true, "e": true, "i": true, "o": true, "u": true,
	"om": true, "em": true, "ama": true, "ima": true, "ih": true, "im": true,
	"og": true, "oj": true, "ome": true, "omu": true, "ega": true, "emu": true, "ej": true,
	"ov": true, "ova": true, "ovi": true, "ove": true, "ovima": true,
	"ev": true, "eva": true, "evi": true, "eve": true, "evima": true,
	"ica": true, "ice": true, "ici": true, "icu": true, "icom": true, "icama": true,
	"ka": true, "ke": true, "ki": true, "ku": true, "kom": true, "kama": true,
	"kinja": true, "kinje": true, "kinji": true, "kinju": true,
}

// bcsToken is one word of a BCS alias: a literal word, or a stem that takes an ending.
// A diacritic token is matched against the title's words with their diacritics kept.
type bcsToken struct {
	text      string
	stem      bool
	diacritic bool
}

type bcsEntry struct {
	tokens    []bcsToken
	canonical string
}

func compileBCS(table []aliasEntry) []bcsEntry {
	out := make([]bcsEntry, 0, len(table))
	for _, e := range table {
		var toks []bcsToken
		alias, diacritic := strings.CutPrefix(e.alias, "=")
		for _, w := range strings.Fields(alias) {
			if s, ok := strings.CutSuffix(w, "*"); ok {
				toks = append(toks, bcsToken{text: s, stem: true, diacritic: diacritic})
			} else {
				toks = append(toks, bcsToken{text: w, diacritic: diacritic})
			}
		}
		out = append(out, bcsEntry{tokens: toks, canonical: e.canonical})
	}
	return out
}

// matches reports whether word is the token: the literal word, or the stem followed
// by one of bcsEndings.
func (t bcsToken) matches(word string) bool {
	if !t.stem {
		return word == t.text
	}
	rest, ok := strings.CutPrefix(word, t.text)
	return ok && bcsEndings[rest]
}

// bcsTitle is a folded title's words twice over: with their diacritics, and in ASCII.
type bcsTitle struct {
	diacritic, ascii []string
}

// matchBCSOrdered returns the canonical value of the first entry (in table order)
// whose tokens occur as consecutive words of the folded title, or "".
func matchBCSOrdered(title bcsTitle, table []bcsEntry) string {
	for _, e := range table {
		n := len(e.tokens)
		for i := 0; i+n <= len(title.ascii); i++ {
			ok := true
			for j, t := range e.tokens {
				words := title.ascii
				if t.diacritic {
					words = title.diacritic
				}
				if !t.matches(words[i+j]) {
					ok = false
					break
				}
			}
			if ok {
				return e.canonical
			}
		}
	}
	return ""
}

// latinizeSerbian transliterates a lowercased title written in Serbian Cyrillic into
// the Latin alphabet, and returns any other title unchanged.
//
// A title is read as Serbian only when it says so (looksSerbianCyrillic): Russian,
// Ukrainian and Bulgarian titles reach this fallback too, and words like "Шеф-повар" or
// "Кухар" would transliterate into BCS words and change rows the main table's Cyrillic
// aliases left alone on purpose.
func latinizeSerbian(lower string) string {
	if !looksSerbianCyrillic(lower) {
		return lower
	}
	return normalize.LatinizeSerbian(lower)
}

// looksSerbianCyrillic reports whether a lowercased title is written in Serbian (or
// Macedonian) Cyrillic: every Cyrillic letter in it is one the Serbian alphabet has, and
// it carries a letter only Serbian and Macedonian use (ђ, ј, љ, њ, ћ, џ) or one of the
// Serbian role words no Russian, Ukrainian or Bulgarian title spells (serbianCyrillicCue).
func looksSerbianCyrillic(lower string) bool {
	cyrillic, serbianOnly := false, false
	for _, r := range lower {
		if !unicode.Is(unicode.Cyrillic, r) {
			continue
		}
		if !normalize.IsSerbianCyrillic(r) {
			return false
		}
		cyrillic = true
		switch r {
		case 'ђ', 'ј', 'љ', 'њ', 'ћ', 'џ':
			serbianOnly = true
		}
	}
	return cyrillic && (serbianOnly || serbianCyrillicCue.MatchString(lower))
}

// serbianCyrillicCue is a Serbian role word spelled without a Serbian-only letter, which
// no other Cyrillic language writes this way (Russian: "программист", "водитель",
// "продавец"). Each is a word stem; the boundary before it is spelled out, as RE2's \b
// is ASCII-only.
var serbianCyrillicCue = regexp.MustCompile(`(?:^|[^\p{L}])(?:програмер|возач|продавац|продавачиц|конобар|кувар|сарадни[кц]|саветни[кц]|медицинск[аи]|рачуновод|магацин|оператер|техничар|приправни[кц]|чистачиц|електричар|стручни|референт за|менаџ)`)

// bcsGenderNotice is the gender notice BCS titles carry: "(m/ž)", "(M/Ž)", "(ž/m)",
// "(m/ž/x)", "m/ž", "M/Ž/D". The bare form is cut only as a whole token.
var bcsGenderNotice = regexp.MustCompile(`\(\s*[mžz]\s*/\s*[mžz](?:\s*/\s*[xdo])?\s*\)|(?:^|\s|-)[mž]\s*/\s*[žm](?:\s*/\s*[xd])?(?:$|\s|[,;)])`)

// bcsGenderSlash is the inline feminine form written after a slash: "Voditelj/ica",
// "Inženjer/ka", "MLAĐI/A", "Operater/ka", "ISTRAŽIVAČ/ICA". The ending is cut, so the
// masculine word before it matches.
var bcsGenderSlash = regexp.MustCompile(`(\p{L})/(?:ica|ice|ka|ke|kinja|a|e)([\s,;:()]|$)`)

// foldBCSGender transliterates a lowercased title and cuts its gender forms, keeping
// the diacritics: the main table is asked again of it.
func foldBCSGender(lower string) string {
	lower = latinizeSerbian(lower)
	lower = bcsGenderNotice.ReplaceAllString(lower, " ")
	lower = bcsGenderSlash.ReplaceAllString(lower, "$1$2")
	return strings.Join(strings.Fields(lower), " ")
}

// bcsASCII drops the diacritics the BCS alphabets use and writes đ as "dj".
var bcsASCII = strings.NewReplacer("č", "c", "ć", "c", "š", "s", "ž", "z", "đ", "dj")

// bcsWords splits a title already folded by foldBCSGender into its words, with their
// diacritics and in ASCII (đ written "dj"; a word keeps its position in both).
func bcsWords(folded string) bcsTitle {
	var t bcsTitle
	for _, w := range strings.FieldsFunc(folded, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		t.diacritic = append(t.diacritic, w)
		t.ascii = append(t.ascii, bcsASCII.Replace(w))
	}
	return t
}

var (
	bcsCategory  = compileBCS(bcsCategoryTable)
	bcsSeniority = compileBCS(bcsSeniorityTable)
)

// matchBCSCategory resolves the category of a lowercased title the other tables left
// unresolved: the main table once more on the transliterated, gender-cut title, then
// the BCS table on the folded one, then the Slovenian table (slovenian.go). A blind alias
// answers categoryNone.
func matchBCSCategory(lower string) string {
	g := foldBCSGender(lower)
	if g != lower {
		if c := matchOrdered(g, categoryTable); c != "" {
			return c
		}
	}
	words := bcsWords(g)
	if c := matchBCSOrdered(words, bcsCategory); c != "" {
		return c
	}
	return matchBCSOrdered(words, slovenianCategory)
}
