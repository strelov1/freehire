package skilltag

import (
	"maps"
	"slices"
	"strings"
)

// trimLower is the canonical-slug normal form used by the invariant test: lowercase
// with no surrounding or internal spaces. A canonical equal to its trimLower form is
// a valid slug.
func trimLower(s string) string {
	return strings.ToLower(strings.TrimSpace(strings.ReplaceAll(s, " ", "")))
}

// wordAliases maps a bare alphanumeric token to its canonical skill slug. Matched
// on whole word tokens (the word pass), so an entry never matches inside a larger
// word. Ambiguous English words (go, c, r) are deliberately ABSENT here — they are
// emitted only via an unambiguous alias (golang) or a phrase ("c developer"), so
// "please go home" or "plan c" never tags. Seed list; expand toward ~200 from
// MIND-tech-ontology and github/linguist languages.yml — pure data, no engine change.
var wordAliases = func() map[string]string {
	out := make(map[string]string, 810)
	for _, part := range []map[string]string{wordAliases1, wordAliases2, wordAliases3} {
		maps.Copy(out, part)
	}
	return out
}()

type phraseAlias struct {
	alias     string
	canonical string
}

// phraseAliases is every phrase the engine matches — the engineering vocabulary plus
// the non-engineering professional one. The two are declared separately for the reader
// only; Parse treats them identically.
var phraseAliases = slices.Concat(engineeringPhraseAliases, professionalPhraseAliases, occupationalSafetyPhraseAliases, industrialMaintenancePhraseAliases)

// engineeringPhraseAliases covers terms the word pass cannot see because they contain
// non-alphanumeric characters or spaces. Includes the ONLY routes by which an
// ambiguous canonical (c) may be emitted.
var engineeringPhraseAliases = slices.Concat(engineeringPhraseAliases1, engineeringPhraseAliases2, engineeringPhraseAliases3, engineeringPhraseAliases4)
