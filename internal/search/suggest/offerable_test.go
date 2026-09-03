package suggest

import "testing"

// A frequency floor decides whether a title is COMMON. It cannot decide whether a
// title is USEFUL, and the two come apart at the top of the distribution: measured
// over a 2,000-title sample of the live catalogue, bare "manager" occurs 44 times and
// bare "director" 18 — far above any sane floor, and worthless as a suggestion,
// because neither names a craft. Someone who does not know what to type is not helped
// by being offered "Manager".
func TestOfferable(t *testing.T) {
	cases := []struct {
		name  string
		title string
		want  bool
	}{
		{"names a craft", "product owner", true},
		{"names a craft with a grade", "senior software engineer", true},
		{"a grade qualifying a craft", "lead data engineer", true},

		{"bare grade", "senior", false},
		{"bare grade, another", "principal", false},
		{"bare grade, intern", "intern", false},

		// The generic management nouns. Each is a real, frequent title and names no
		// discipline at all; the role and category dictionaries carry that axis.
		{"bare manager", "manager", false},
		{"bare director", "director", false},
		{"bare consultant", "consultant", false},

		// Qualified, the same word names a job. This is the line the rule has to hold:
		// drop the bare noun, keep everything it qualifies.
		{"qualified manager", "engineering manager", true},
		{"qualified director", "sales & marketing director", true},
		{"qualified consultant", "sap consultant", true},

		// A grade plus a bare generic is still no craft. "Senior Manager" occurs 17
		// times in the same sample and says nothing more than "Manager" does.
		{"grade plus bare generic", "senior manager", false},
		{"grade plus bare generic, principal", "principal consultant", false},

		// NOT a grade — "assistant manager" is a real and very common retail job, 47
		// occurrences in the same sample. The rule drops words that name no job, not
		// every phrase built around a generic noun.
		{"a word that only looks like a grade", "assistant manager", true},

		{"empty", "", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Offerable(c.title); got != c.want {
				t.Errorf("Offerable(%q) = %v, want %v", c.title, got, c.want)
			}
		})
	}
}
