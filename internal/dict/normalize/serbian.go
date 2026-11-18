package normalize

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// serbianLatin is the official Serbian Cyrillic-to-Latin mapping (Gaj's alphabet), keyed
// by the lower-case letter, plus the Macedonian ѓ, ќ and ѕ. It is one-to-one, so
// transliteration loses nothing.
var serbianLatin = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'ђ': "đ", 'е': "e", 'ж': "ž",
	'з': "z", 'и': "i", 'ј': "j", 'к': "k", 'л': "l", 'љ': "lj", 'м': "m", 'н': "n",
	'њ': "nj", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'ћ': "ć", 'у': "u",
	'ф': "f", 'х': "h", 'ц': "c", 'ч': "č", 'џ': "dž", 'ш': "š",
	'ѓ': "đ", 'ќ': "ć", 'ѕ': "dz",
}

// IsSerbianCyrillic reports whether r is a letter of the Serbian (or Macedonian) Cyrillic
// alphabet, in either case.
func IsSerbianCyrillic(r rune) bool {
	_, ok := serbianLatin[unicode.ToLower(r)]
	return ok
}

// LatinizeSerbian transliterates every Serbian Cyrillic letter in s into Latin and leaves
// everything else (Latin text, digits, markup) as it is. Case is kept: a capital whose Latin
// form is two letters (Љ, Њ, Џ) is written "Lj" before a lower-case letter and "LJ" inside a
// word in capitals ("ЉУБАВ" -> "LJUBAV", "Љубав" -> "Ljubav").
//
// It does not check that the text IS Serbian: a Russian word spelled with shared letters is
// transliterated too. A caller with text of unknown language decides that first
// (internal/dict/classify reads a title as Serbian only when it shows it).
func LatinizeSerbian(s string) string {
	if !strings.ContainsFunc(s, IsSerbianCyrillic) {
		return s
	}
	rs := []rune(s)
	var b strings.Builder
	b.Grow(len(s))
	for i, r := range rs {
		lower := unicode.ToLower(r)
		latin, ok := serbianLatin[lower]
		if !ok {
			b.WriteRune(r)
			continue
		}
		if r == lower {
			b.WriteString(latin)
			continue
		}
		if utf8.RuneCountInString(latin) > 1 && !capitalsAround(rs, i) {
			// A title-case digraph: only its first letter is a capital.
			first, size := utf8.DecodeRuneInString(latin)
			b.WriteRune(unicode.ToUpper(first))
			b.WriteString(latin[size:])
			continue
		}
		b.WriteString(strings.ToUpper(latin))
	}
	return b.String()
}

// capitalsAround reports whether the capital at rs[i] stands in a word written in
// capitals: the letter after it, or failing a letter there the one before it, is a capital.
func capitalsAround(rs []rune, i int) bool {
	if i+1 < len(rs) && unicode.IsLetter(rs[i+1]) {
		return unicode.IsUpper(rs[i+1])
	}
	return i > 0 && unicode.IsLetter(rs[i-1]) && unicode.IsUpper(rs[i-1])
}
