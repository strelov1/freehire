// Package answertopic keys an employer's screening question by WHAT IT ASKS rather than
// how it is worded, so an answer the candidate gives once serves every later posting that
// asks the same thing.
//
// Two passes, in order, and the order is the point. A dictionary of known topics runs
// first because it collapses phrasings a fold cannot ("compensation expectations" and
// "desired salary" share no words). Everything else falls back to a fold of the question
// text — so a question nobody anticipated still gets a stable key, and a missing
// dictionary entry costs a coarser key rather than a lost answer.
//
// That fallback is why this is not another curated list. A hand-maintained list that is
// the SOLE route is a list whose gaps are invisible: internal/api/atsapply's own
// labelAnswerKeyFor had no salary rule for months, and nothing reported the questions it
// was failing to match.
//
// Pure and deterministic — no model, no I/O. A model proposing a topic for an awkwardly
// folded question is deliberately out of scope.
package answertopic

import "strings"

// dictionary maps a topic to the keyword sets that name it. ALL keywords in a set must
// appear for that set to fire; any one set firing is enough.
//
// Kept small on purpose: it exists for phrasings the fold genuinely cannot unify, not as
// the place every question is expected to be listed.
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
	"we'd like to know",
	"tell us",
}

// Of returns the topic a question is keyed by, and whether it could be keyed at all.
//
// A question that folds to nothing — an empty label, or one that is pure punctuation, both
// of which real forms produce — is refused. A key derived from nothing cannot be recalled,
// so storing one would silently mark a question answered forever.
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
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
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
func containsAll(folded string, keywords []string) bool {
	for _, kw := range keywords {
		if !strings.Contains(folded, kw) {
			return false
		}
	}
	return true
}
