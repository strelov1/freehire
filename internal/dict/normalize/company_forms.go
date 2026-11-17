package normalize

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// CloseSpacedForms closes up the Polish "o.o." written with a space inside it, "Sp. z o. o.". Split there, it is two words of the letter
// "o", and a lone "o" cannot be a form — it would strip the end of any name with an initial.
// Closing the gap before the walk turns it into the "o.o." [formQualifiers] already reads.
// It is anchored to the end of the name (past a closing bracket, "Acme (Sp. z o. o.)") and
// needs the "z" before it, so only that tail moves.
//
// The Slovenian "d. o. o." is closed up the same way, into the "d.o.o." [legalSuffixes] reads:
// "Adria Mobil, d. o. o.", one of them with a "k.d." after it ("Bauhaus, d. o. o., k.d."), which the anchor steps over. It needs all
// three letters, so a name ending in initials ("D. O.") is not touched, and only at the tail:
// mid-name the slug renders "d-o-o" either way.
//
// So is the Slovenian and Croatian "d. d.", into the "d.d." legalSuffixes reads:
// "Telekom Slovenije, d. d.", "Banka Intesa Sanpaolo d. d.".
// Two Latin d's are matched, never the Cyrillic "Д", so the Russian initials "ИП КИМ А. Д."
// keep their letters.
//
// It also drops the registered SEAT that a Slovenian or Croatian name puts after its form:
// "KRKA, d.d., Novo mesto" or "TERME KRKA, d.o.o., Novo mesto". The register name carries the
// seat as part of the name, so a source that copies it verbatim ends the name with a town. The
// form is then no longer the last word, and the walk never reaches it. That kept
// "Krka, tovarna zdravil, d. d., Novo mesto" and "KRKA, d.d., Novo mesto" apart from the Krka
// the employer's own board files under. The cut needs the dotted d.d. or
// d.o.o. (spaced or not) and a comma after it, and it takes only a LAST segment made of letters,
// so "Acme d. d. Ljubljana" stays as it is. With no comma there, nothing says the town is a seat
// and not part of the name.
//
// The cut also needs at least two name words before the form, because a short name freed of
// its seat lands on whoever else holds that word ("SCAN, d.o.o., Preddvor" on the US health plan SCAN,
// "Bass d.o.o., Celje" on the German BASS GmbH). Such names are safe only through the seat that
// tells them apart; the price is that one employer's short spellings stay apart until someone
// merges them.
//
// Every walk over a name's tail calls it before splitting the name into words, for the same
// reason each asks [IsLegalFormBefore]: a walk that skipped it would count "o." "o." as two
// name words where the company key drops them.
func CloseSpacedForms(name string) string {
	name = dropSeat(name)
	name = spacedPolishTail.ReplaceAllString(name, "${1}o.o.")
	name = spacedBalkanTail.ReplaceAllString(name, "${1}.o.o.${2}")
	return spacedJointStockTail.ReplaceAllString(name, "${1}.d.${2}")
}

// dropSeat cuts the registered seat after a Balkan form, when at least two name words stand
// before the form. See [CloseSpacedForms].
func dropSeat(name string) string {
	loc := balkanSeatTail.FindStringSubmatchIndex(name)
	if loc == nil {
		return name
	}
	// isWordBreak keeps '.' and '/' inside a word, so a lone "/" arrives here as a word of its
	// own, and "SCAN / d.o.o., Preddvor" must still count as one name word.
	words := slices.DeleteFunc(strings.FieldsFunc(dropApostrophes(name[:loc[0]]), isWordBreak), func(w string) bool {
		return !strings.ContainsFunc(w, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) })
	})
	next := ""
	for len(words) > 0 && IsLegalFormBefore(words[len(words)-1], next) {
		next = words[len(words)-1]
		words = words[:len(words)-1]
	}
	if len(words) < 2 {
		return name
	}
	return name[:loc[3]]
}

var (
	balkanSeatTail       = regexp.MustCompile(`(?i)((?:^|[^\p{L}\p{N}])d\.[\s\p{Zs}]*(?:o\.[\s\p{Zs}]*o\.?|d\.?))[\s\p{Zs}]*,[\s\p{Zs}]*\p{L}[\p{L}\s\p{Zs}.\-]*$`)
	spacedPolishTail     = regexp.MustCompile(`(?i)([\s\p{Zs}.]z[\s\p{Zs}]+)o\.[\s\p{Zs}]+o\.?[\s\p{Zs})\]]*$`)
	spacedBalkanTail     = regexp.MustCompile(`(?i)((?:^|[^\p{L}\p{N}])d)\.[\s\p{Zs}]+o\.[\s\p{Zs}]+o\.?((?:[\s\p{Zs},]+k\.d\.?)?[\s\p{Zs})\]]*)$`)
	spacedJointStockTail = regexp.MustCompile(`(?i)((?:^|[^\p{L}\p{N}])d)\.[\s\p{Zs}]+d\.?([\s\p{Zs})\]]*)$`)
)

// dropApostrophes removes the possessive/elision marks a name carries so they cannot become
// word breaks in the key. "Kohl's" is one word and belongs at `kohls`, which is also what a
// person types; [Slug] alone renders it `kohl-s`.
//
// The canonical slug is derived from a name rather than chosen among stored slugs, so the
// `kohl-s` artefact could no longer lose to the clean `kohls`. Both the ASCII and the
// typographic mark are covered.
func dropApostrophes(name string) string {
	return strings.NewReplacer("'", "", "\u2019", "", "\u02bc", "").Replace(name)
}

// isWordBreak reports whether a rune separates two words of a company name. Everything [Slug]
// would drop separates, EXCEPT "." and "/" — those two live inside the very forms this is
// looking for ("B.V.", "A/S"), so splitting on them would hide the form instead of finding it.
//
// Whitespace alone is not enough: "Sun Technologies,Inc." is how aggregators routinely write
// the name. Splitting here rather than on Slug's output is also what keeps "Foo.com" one word.
func isWordBreak(r rune) bool {
	return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '.' && r != '/'
}

// spelledForms are the corporate forms recognised only AS WRITTEN — the whole lowercased word
// past any punctuation around it, dots inside kept — because reducing them to their letters,
// as [legalSuffixes] does, would read ordinary words as forms. Two reasons put a form here:
//
//   - Its letters are not all ASCII. [letters] keeps ASCII only, so "OÜ" reads "o". Folding
//     accents inside letters instead was measured and refused: it transliterates
//     Cyrillic too, and "ООО "САС"" and "ООО "АГ"" both became `ooo` — two employers on one
//     key — while initials like "С.А." started reading as forms.
//   - Its bare letters are a word, or another place's abbreviation, while its dotted spelling
//     is only the form. "a.d." is a Serbian joint-stock company (Banca Intesa a.d.,
//     Erste Bank a.d.), while "ad" — and "úřad", whose accents letters drops — is an advert or
//     an office; "s.p." is a Slovenian sole trader, while a bare "SP" is often São Paulo;
//     "z.o.o." is a Slovenian cooperative or Polish "Sp. z.o.o.", while "Zoo" is a zoo. "k.d." (the Lidl companies' "d.o.o.
//     k.d.") is here and not in legalSuffixes because letters reads the Danish "Kød" as "kd".
//     Its undotted "KD" is deliberately NOT listed: it also ends the fashion brand "NA-KD",
//     which it would key at `na`. "d.n.o."
//     because "dno" is a Slavic word. "e.s.p." is the Colombian public-utility company (empresa
//     de servicios públicos, "Conhydra S.A E.S.P"), while a bare "ESP" ends names for Spain or
//     as an acronym ("Hotmart (ESP)", "Millennium IT ESP"). The dotted "S.A.D." is deliberately
//     NOT listed: it also ends a French home-care service
//     ("Castel S.A.D", Service d'Aide à Domicile), where it is part of the name.
//
// Matching the spelling is also what lets a GLUED compound in: "Sp.zo.o." and "Sp.z.o.o." are
// one word to the tail walk (a dot is not a word break), and as letters "spzoo", which ends no
// list. Spelled out they are unambiguous, so each is listed whole.
var spelledForms = map[string]struct{}{
	"oü":  {},
	"a.d": {}, "s.p": {}, "k.d": {}, "d.n.o": {},
	"z.o.o": {}, "zo.o": {}, "sp.z.o.o": {}, "sp.zo.o": {},
	"e.s.p": {},
}

// notLetter trims what may surround a [spelledForms] word — "OÜ.", "(OÜ)", "a.d." — the way
// [letters] ignores it for every other form.
func notLetter(r rune) bool { return !unicode.IsLetter(r) }

// isGluedForms reports whether a word is several forms joined by dots with no space, "Pvt.Ltd."
// or "Pte.Ltd" — one word to the tail walk, since a dot is not a word break (it lives inside
// "B.V."), and as letters "pvtltd", which no entry names. It is a form when EVERY dotted part
// is one, read right to left as the walk reads words, so "Pvt.Ltd" and "P.Ltd" qualify while
// "Foo.com" does not. The punctuated single forms never reach this: "B.V." is already "bv".
func isGluedForms(word string) bool {
	parts := strings.FieldsFunc(word, func(r rune) bool { return r == '.' })
	if len(parts) < 2 {
		return false
	}
	next := ""
	for i := len(parts) - 1; i >= 0; i-- {
		if !IsLegalFormBefore(parts[i], next) {
			return false
		}
		next = parts[i]
	}
	return true
}
