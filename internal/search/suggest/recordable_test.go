package suggest

import (
	"strings"
	"testing"
)

// The demand table is public input, written on every search. Two things follow, and
// one rule answers both: what is worth recording is a search PHRASE, and a phrase is
// short.
//
// Privacy: `Title` normalises text, it does not judge it — a visitor who pastes an
// email address or a whole job description into the box would otherwise have it stored
// indefinitely, in a table whose only purpose is ranking vocabulary.
//
// Growth: the table's key is the phrase, so every distinct string is a row, and the
// per-caller rate limit bounds one caller rather than the aggregate. Unbounded length
// times unbounded distinct values is the whole problem.
func TestRecordable(t *testing.T) {
	cases := []struct {
		name string
		q    string
		want bool
	}{
		{"a phrase people search", "senior software engineer", true},
		{"one word", "backend", true},
		{"a phrase with an employer", "product owner google", true},

		{"empty", "", false},
		{"whitespace", "   ", false},

		// Nobody types this into a search box. Something pasted it.
		{"an email address", "john.smith@example.com", false},
		{"too many words", strings.Repeat("word ", 12), false},
		{"a pasted paragraph", strings.Repeat("a", 200), false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Recordable(Title(c.q)); got != c.want {
				t.Errorf("Recordable(Title(%q)) = %v, want %v", c.q, got, c.want)
			}
		})
	}
}

// The bound is on the NORMALISED phrase, because that is what gets stored. A long
// title with a qualifier is cut down to its name first and must survive.
func TestRecordable_JudgesWhatIsActuallyStored(t *testing.T) {
	long := "Senior Software Engineer, Infrastructure, Platform, Spanner, Mountain View"
	if !Recordable(Title(long)) {
		t.Errorf("Title(%q) = %q, which should be recordable", long, Title(long))
	}
}
