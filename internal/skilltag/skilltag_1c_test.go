package skilltag

import (
	"slices"
	"testing"
)

// Test1C covers the two routes to the "1c" canonical: the Cyrillic "1С" the RU market uses
// resolves as a strong phrase (tags even alone), while the Latin "1c" word alias is gated so it
// tags only alongside another tech token — a bare "1c" in prose (a figure label) never leaks.
func Test1C(t *testing.T) {
	cases := []struct {
		name string
		text string
		want bool
	}{
		{"cyrillic standalone is strong", "Требуется программист 1С для доработки конфигураций.", true},
		{"cyrillic hyphenated", "Ищем 1С-разработчика в продуктовую команду.", true},
		{"latin corroborated by another tech token", "1C:Enterprise developer, experience with SQL required.", true},
		{"latin alone in prose is gated out", "Please review figure 1c in the attached report.", false},
	}
	for _, c := range cases {
		if got := slices.Contains(Parse(c.text), "1c"); got != c.want {
			t.Errorf("%s: Parse(%q) contains %q = %v, want %v", c.name, c.text, "1c", got, c.want)
		}
	}
}
