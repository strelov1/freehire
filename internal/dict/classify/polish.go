package classify

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// polishOnlyWords are words no other language reaching the fallback spells this way,
// so a Polish title written without its diacritics ("Specjalista ds. IT", "Kierownik
// projektu") still counts as Polish. Only the Polish spelling counts: "operator",
// "administrator", "architekt", "technik", "referent", "inspektor", "lider" and
// "ekspert" are English, German, Czech, Spanish or Danish words as well, so their
// feminine ("operatorka", "architektka") stands in for them.
var polishOnlyWords = regexp.MustCompile(`(?:^|[^\p{L}])(?:ds|specjalist\p{L}*|kierowni\p{L}*|pracowni\p{L}*|programist\p{L}*|inzynier\p{L}*|ksiegow\p{L}*|magazynier\p{L}*|kierowc\p{L}*|przedstawiciel\p{L}*|sprzedawc\p{L}*|handlowiec|handlowc\p{L}*|asystent\p{L}*|doradc\p{L}*|doradczyni|konsultantk\p{L}*|opiekun\p{L}*|pielegniar\p{L}*|koordynatork\p{L}*|starsz[ayae]\p{L}*|mlodsz[ayae]\p{L}*|stazyst\p{L}*|praktykant\p{L}*|analityk\p{L}*|analityczk\p{L}*|architektk\p{L}*|kosztorysant|tokarz|lekarz\p{L}*|lekark\p{L}*|higienistk\p{L}*|kurierzy|kurierk\p{L}*|przewoznik\p{L}*|zastepc\p{L}*|zastepczyni|liderk\p{L}*|menedzer\p{L}*|menadzer\p{L}*|inspektork\p{L}*|referentk\p{L}*|wsparcie|stanowisk\p{L}*|techniczk\p{L}*|instruktork\p{L}*|operatork\p{L}*|administratork\p{L}*|dyrektor\p{L}*|prezes\p{L}*|zespol\p{L}*|dzial\p{L}*|obslugi|sprzedazy|zatrudnimy|radca|radczyni)(?:$|[^\p{L}])`)

// polishLetters are the letters only Polish among the fallback's languages writes.
var polishLetters = regexp.MustCompile(`[ąćęłńśźż]`)

// polishGenderNotice is the gender notice of a Polish posting — "(K/M)", "(m/k)",
// "(k/m/n)", "K/M". It needs the "k" (kobieta): "(m/f/d)" is German and English.
var polishGenderNotice = regexp.MustCompile(`\(\s*(?:k\s*/\s*m|m\s*/\s*k)(?:\s*[/|,]\s*[nxd])?\s*\)|(?:^|\s)(?:k\s*/\s*m|m\s*/\s*k)(?:\s*/\s*[nx])?(?:$|\s)`)

// polishEndings are the case endings only Polish spells this way: the adjective's
// "-owy/-owa/-owych" (Czech and Slovak write "-ový/-ová", with a "v"), the genitive's
// "-nego" and "-cji", and the abstract noun's "-ości" ("Architekt Systemowy",
// "Analityk Hurtowni Danych", "Specjalista ds. jakości" without its diacritics).
var polishEndings = regexp.MustCompile(`(?:\p{L}{3}(?:owy|owa|owe|owej|owych|owego|owym|nego|cji|osci)|(?:^|[^\p{L}])danych)(?:$|[^\p{L}])`)

// looksPolish reports whether a lowercased title shows it is written in Polish: a
// letter only Polish writes, its gender notice, a word only Polish spells this way, or
// a Polish case ending.
func looksPolish(lower string) bool {
	if polishLetters.MatchString(lower) || polishGenderNotice.MatchString(lower) {
		return true
	}
	if polishFeminineSuffix.MatchString(lower) {
		return true
	}
	folded := foldPolish(lower)
	return pairsPolishFeminine(folded) || polishOnlyWords.MatchString(folded) || polishEndings.MatchString(folded)
}

// polishFeminineSuffix is the feminine ending appended to a masculine head:
// "Administrator/-ka", "Konsultant(ka)", "Specjalista/tka". The Spanish "/a" is not one.
var polishFeminineSuffix = regexp.MustCompile(`\p{L}\s*(?:/\s*-?|\(\s*-?)\s*(?:ka|tka|czka)(?:$|[^\p{L}])`)

// pairsPolishFeminine reports whether the title spells a role in both genders the
// Polish way — "Ratownik / Ratowniczka", "Optometrysta / Optometrystka", "Radczyni
// Prawna/Radca Prawny": two words around a slash sharing a stem, one of them ending in
// the feminine "-ka" or "-yni". It reads the folded title, so a stem is plain ASCII.
func pairsPolishFeminine(folded string) bool {
	parts := strings.Split(folded, "/")
	for i := 0; i+1 < len(parts); i++ {
		a, b := lastWord(parts[i]), firstWord(parts[i+1])
		if len(a) < 4 || len(b) < 4 {
			continue
		}
		for _, p := range [][2]string{{a, b}, {b, a}} {
			fem, masc := p[0], p[1]
			feminine := strings.HasSuffix(fem, "ka") || strings.HasSuffix(fem, "yni")
			if feminine && strings.HasPrefix(fem, masc[:len(masc)-2]) {
				return true
			}
		}
	}
	return false
}

func lastWord(s string) string {
	f := strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) })
	if len(f) == 0 {
		return ""
	}
	return f[len(f)-1]
}

func firstWord(s string) string {
	f := strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) })
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

// polishGenderSuffix is the feminine ending a Polish title appends to the masculine:
// "Inżynier(-ka)", "Sieciowy(-a)", "Specjalista/-ka", "Konsultant/ka", "Specjalist(k)a",
// "Młodszy/a". It is cut, so the masculine the main table spells is what remains.
var polishGenderSuffix = regexp.MustCompile(`(\p{L})(?:\s*\(\s*-?\s*(?:ka|a|tka|czka|y|i)\s*\)|/\s*-?\s*(?:ka|a|tka|czka)(?:$|[^\p{L}])|\(k\)a)`)

// foldPolishGender cuts the gender forms out of a lowercased title, keeping its
// diacritics: the main table is asked again of it.
func foldPolishGender(lower string) string {
	lower = polishGenderNotice.ReplaceAllString(lower, " ")
	lower = polishGenderSuffix.ReplaceAllStringFunc(lower, func(m string) string {
		sub := polishGenderSuffix.FindStringSubmatch(m)
		if strings.HasSuffix(m, "(k)a") {
			return sub[1] + "a"
		}
		tail := ""
		if r := []rune(m); !unicode.IsLetter(r[len(r)-1]) && r[len(r)-1] != ')' {
			tail = string(r[len(r)-1])
		}
		return sub[1] + tail
	})
	return strings.Join(strings.Fields(lower), " ")
}

// foldPolish prepares a lowercased title for the Polish tables: the gender forms cut,
// "mł." and "st." spelled out as the grades they abbreviate, and the diacritics
// dropped — "ł" by hand, since Unicode does not decompose it.
func foldPolish(lower string) string {
	lower = foldPolishGender(lower)
	lower = polishJuniorAbbrev.ReplaceAllString(lower, "${1}mlodszy ")
	lower = strings.ReplaceAll(lower, "ł", "l")
	var b strings.Builder
	for _, r := range norm.NFD.String(lower) {
		if !unicode.Is(unicode.Mn, r) {
			b.WriteRune(r)
		}
	}
	return polishSeniorAbbrev.ReplaceAllString(b.String(), "${1}starszy $2")
}

// polishJuniorAbbrev is "Mł." — the abbreviated "Młodszy". Only the spelling with
// "ł" and the period counts: a bare "ml" is machine learning.
var polishJuniorAbbrev = regexp.MustCompile(`(^|[^\p{L}])mł\.\s*`)

// polishSeniorAbbrev is "St." — the abbreviated "Starszy" — before a Polish head
// ("St. specjalistka", "St.specjalista"). "St." alone is a saint's or a street's.
var polishSeniorAbbrev = regexp.MustCompile(`(^|[^\p{L}])st\.\s*(specjalist|inzynier|ksiegow|analityk|programist|referent|asystent|konsultant|technik|administrator|koordynator|ekspert|inspektor|kierownik)`)

// polishWords splits a folded title into its words. A hyphen joins ("wodno-kanalizacyjnych",
// "back-end"), and the table spells hyphenated words whole.
func polishWords(folded string) []string {
	return strings.FieldsFunc(folded, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-'
	})
}

// matchPolish returns the canonical of the first alias (in table order) that occurs
// in the words, or "".
func matchPolish(words []string, table []aliasEntry) string {
	for _, e := range table {
		toks := strings.Fields(e.alias)
		for i := 0; i+len(toks) <= len(words); i++ {
			if polishPhraseAt(words[i:], toks) {
				return e.canonical
			}
		}
	}
	return ""
}

// polishPhraseAt reports whether the words begin with the alias tokens, a "*" token
// matching any word that starts with it.
func polishPhraseAt(words, toks []string) bool {
	for j, t := range toks {
		if stem, ok := strings.CutSuffix(t, "*"); ok {
			if !strings.HasPrefix(words[j], stem) {
				return false
			}
		} else if words[j] != t {
			return false
		}
	}
	return true
}

// matchPolishCategory resolves the category of a lowercased title the other tables
// left unresolved: the main table once more on the gender-cut title, then the Polish
// table on the folded one. A blind alias answers categoryNone.
func matchPolishCategory(lower string) string {
	if !looksPolish(lower) {
		return ""
	}
	if g := foldPolishGender(lower); g != lower {
		if c := matchOrdered(g, categoryTable); c != "" {
			return c
		}
	}
	return matchPolish(polishWords(polishPerks.ReplaceAllString(foldPolish(lower), " ")), polishCategoryTable)
}

// polishPerks are the benefits a title advertises beside the role — "darmowy transport"
// is the free shuttle to a warehouse, not a transport job.
var polishPerks = regexp.MustCompile(`\b(?:darmowy|bezplatny|zapewniony|zapewniamy)\s+(?:transport|dojazd)\b`)

// openingPolishLider is a title that OPENS with "Lider"/"Liderka": "Lider Obszaru",
// "Lider/Liderka Zmiany". Anywhere else the word may praise the employer ("lider
// rynku"), so only the opening word is read, as the Spanish "Líder".
var openingPolishLider = regexp.MustCompile(`^[^\p{L}\p{N}]*lider(?:ka)?\b`)
