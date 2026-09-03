package suggest

import (
	"strings"

	"github.com/strelov1/freehire/internal/dict/vocab"
)

// genericNouns name a position without naming a discipline. Each is a real and
// frequent posting title — bare "manager" occurs 44 times in a 2,000-title sample of
// the live catalogue and bare "director" 18 — and each is useless as a suggestion:
// somebody who does not know what to type is not helped by being offered "Manager".
//
// The list is short on purpose. It holds only nouns that are BOTH common enough to
// clear a frequency floor and empty enough to answer nothing; a word that names even
// a vague craft ("recruiter", "analyst") belongs in the dictionary, not here.
var genericNouns = map[string]bool{
	"manager":     true,
	"director":    true,
	"consultant":  true,
	"lead":        true,
	"head":        true,
	"specialist":  true,
	"coordinator": true,
	"officer":     true,
	"associate":   true,
	"executive":   true,
}

// gradeWords is the seniority vocabulary as it appears in a title. It is derived from
// vocab.SeniorityValues rather than retyped, so a grade added there is a grade here —
// the alternative is two lists that agree until one of them is edited.
var gradeWords = func() map[string]bool {
	m := make(map[string]bool, len(vocab.SeniorityValues))
	for _, v := range vocab.SeniorityValues {
		// `c_level` is a canonical value, not a word anybody writes in a title. Its
		// surface forms ("chief", "vp") are titles that DO name something, so it is
		// simply absent here rather than translated.
		m[strings.ReplaceAll(v, "_", " ")] = true
	}
	return m
}()

// Offerable reports whether a normalised title is worth suggesting.
//
// A frequency floor decides whether a title is COMMON; this decides whether it is
// USEFUL, and the two come apart at the very top of the distribution. Every word of
// the title being a grade or a generic noun means the title names no craft, and the
// role and category dictionaries already carry those axes properly.
//
// The line it holds: drop the bare noun, keep everything the noun qualifies.
// "Manager" says nothing, "Engineering Manager" says a great deal.
func Offerable(title string) bool {
	words := strings.Fields(title)
	if len(words) == 0 {
		return false
	}
	for _, w := range words {
		if !gradeWords[w] && !genericNouns[w] {
			return true
		}
	}
	return false
}
