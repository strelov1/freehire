package jobreality

import "testing"

func TestHasEvergreenMarker_DetectsKnownPhrases(t *testing.T) {
	cases := map[string]string{
		"always hiring":        "We are always hiring talented engineers.",
		"talent community":     "Join our talent community for future roles.",
		"talent pool":          "Add yourself to our talent pool.",
		"future opportunities": "Submit your CV for future opportunities.",
		"ru always looking":    "Мы всегда в поиске сильных разработчиков.",
		"ru reserve":           "Резюме попадёт в кадровый резерв компании.",
	}
	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			if !HasEvergreenMarker(text) {
				t.Errorf("expected evergreen marker in %q", text)
			}
		})
	}
}

// The dictionary never guesses: an ordinary description with no evergreen phrasing
// (even one mentioning a generic "pipeline") emits no marker.
func TestHasEvergreenMarker_EmitsNothingForUnmatched(t *testing.T) {
	cases := map[string]string{
		"plain role":       "We are hiring a senior Go engineer to build our data pipeline.",
		"specific opening": "This role owns the checkout service. Apply by Friday.",
		"empty":            "",
	}
	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			if HasEvergreenMarker(text) {
				t.Errorf("did not expect evergreen marker in %q", text)
			}
		})
	}
}
