package suggest

import (
	"strings"

	"github.com/strelov1/freehire/internal/dict/vocab"
)

// genericNouns name a position without a discipline. Each is a frequent bare title but
// useless as a suggestion. Keep the list short: a word naming even a vague craft
// ("recruiter", "analyst") belongs in the dictionary instead.
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

// gradeWords is the seniority vocabulary as it appears in a title, derived from
// vocab.SeniorityValues so the two stay in step.
var gradeWords = func() map[string]bool {
	m := make(map[string]bool, len(vocab.SeniorityValues))
	for _, v := range vocab.SeniorityValues {
		// `c_level` is not a word written in titles; its surface forms ("chief", "vp")
		// name something, so it is omitted.
		m[strings.ReplaceAll(v, "_", " ")] = true
	}
	return m
}()

// Offerable reports whether a normalised title is worth suggesting: false when every
// word is a grade or generic noun, since such a title names no craft. "Manager" is
// dropped, "Engineering Manager" kept.
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
