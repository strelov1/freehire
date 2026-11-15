// Package answertopic keys an employer's screening question by WHAT IT ASKS rather than
// how it is worded, so an answer the candidate gives once serves every later posting that
// asks the same thing.
//
// Two passes, in order. A dictionary of known topics runs first because it collapses
// phrasings a fold cannot ("compensation expectations" and "desired salary" share no
// words). Everything else falls back to a fold of the question text, so an unanticipated
// question still gets a stable key and a missing dictionary entry costs a coarser key,
// not a lost answer. A curated list as the sole route would hide its own gaps.
//
// Pure and deterministic: no model, no I/O.
package answertopic

import (
	"strings"
	"unicode"
)

// dictionary maps a topic to the keyword sets that name it. ALL keywords in a set must
// appear for that set to fire; any one set firing is enough. Kept small: only for
// phrasings the fold cannot unify.
var dictionary = []struct {
	topic string
	sets  [][]string
}{
	{"salary_expectation", [][]string{
		{"desired", "salary"},
		{"desired", "compensation"},
		{"salary", "expect"},
		{"compensation", "expect"},
	}},
	{"notice_period", [][]string{
		{"notice", "period"},
		{"how much notice"},
	}},
	{"relocation", [][]string{
		{"relocat"},
	}},
	{"start_date", [][]string{
		{"start", "date"},
		{"when can you start"},
	}},
	// The most common form question. Employers drop their own name into it ("How did you
	// hear about Appian?"), so the fold alone gives each company its own topic. The sets
	// match the asking, not what is asked about.
	{"how_did_you_hear", [][]string{
		{"how did you hear"},
		{"how did you find out"},
		{"how did you learn about"},
		{"where did you hear"},
		{"how you heard"},
	}},
	// The candidate already answers this on file (screening_answers.age_18_or_older).
	{"age_18_or_older", [][]string{
		{"at least 18"},
		{"over the age of 18"},
		{"18 years of age or older"},
		{"18 or older"},
	}},
}

// politeWrappers are the openers employers put in front of the actual question. Dropped so
// "Please tell us your desired salary" and "Desired salary" key alike.
var politeWrappers = []string{
	"please tell us",
	"please share",
	"please provide",
	"could you tell us",
	"could you share",
	"we would like to know",
	"we d like to know",
	"tell us",
}

// Of returns the topic a question is keyed by, and whether it could be keyed at all.
//
// A question that folds to nothing (empty label, pure punctuation) is refused: a key
// derived from nothing cannot be recalled, so storing one would mark it answered forever.
//
// "Readable text" is Unicode's answer, not ASCII's: sources include Russian and Hungarian.
// An ASCII-only fold kept stray Latin fragments, so "Зарплата (USD)" folded to "usd" and
// merged every question mentioning that currency (TestOf_DifferentQuestionsKeepDifferentTopics).
func Of(question string) (string, bool) {
	folded := fold(question)
	if folded == "" {
		return "", false
	}
	for _, entry := range dictionary {
		for _, set := range entry.sets {
			if containsAll(folded, set) {
				return entry.topic, true
			}
		}
	}
	return folded, true
}

// fold normalises a question to its comparable form: lowercase, letters and digits only
// (every other rune becomes a space), single-spaced, with a leading politeness wrapper
// dropped.
func fold(question string) string {
	lower := strings.ToLower(question)
	var b strings.Builder
	b.Grow(len(lower))
	for _, r := range lower {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(r)
		default:
			// Every non-alphanumeric rune, not a fixed punctuation list: a question can
			// carry a currency symbol, an em dash or a non-Latin quote, and naming them
			// one at a time is the curated-list trap this package exists to avoid.
			b.WriteRune(' ')
		}
	}
	folded := strings.Join(strings.Fields(b.String()), " ")
	for _, wrapper := range politeWrappers {
		if strings.HasPrefix(folded, wrapper+" ") {
			folded = strings.TrimSpace(strings.TrimPrefix(folded, wrapper))
			break
		}
	}
	return folded
}

// containsAll reports whether every keyword appears in the folded question.
//
// Substring, not whole-word, so NOT internal/dict/wordmatch: these are STEMS ("relocat"
// must match "relocation", "expect" must match "expectations"), which wordmatch's
// boundaries refuse.
func containsAll(folded string, keywords []string) bool {
	for _, kw := range keywords {
		if !strings.Contains(folded, kw) {
			return false
		}
	}
	return true
}
