package skilltag

import "testing"

// TestDictionaryInvariants guards the two properties the engine relies on:
// every canonical is a stable slug (lowercase, no spaces), and the vocabulary is
// at least the launch floor so an accidental truncation is caught.
func TestDictionaryInvariants(t *testing.T) {
	for alias, c := range wordAliases {
		assertSlug(t, "wordAliases["+alias+"]", c)
	}
	for _, p := range phraseAliases {
		assertSlug(t, "phraseAliases "+p.alias, p.canonical)
	}
	if got := len(wordAliases) + len(phraseAliases); got < 200 {
		t.Errorf("vocabulary size = %d, want >= 200 (launch floor)", got)
	}
	// Ambiguous English words must never be bare word aliases (they resolve only
	// via an unambiguous alias or a phrase).
	for _, w := range []string{"go", "c", "r"} {
		if _, ok := wordAliases[w]; ok {
			t.Errorf("ambiguous word %q must not be a wordAliases key", w)
		}
	}

	// Acronym canonicals must be valid slugs AND already reachable via an existing
	// alias — an acronym is another route to a known canonical, never a new facet value.
	existing := map[string]bool{}
	for _, c := range wordAliases {
		existing[c] = true
	}
	for _, p := range phraseAliases {
		existing[p.canonical] = true
	}
	for tier, acr := range map[string]map[string]string{"sharedAcronyms": sharedAcronyms, "resumeAcronyms": resumeAcronyms} {
		for surface, c := range acr {
			assertSlug(t, tier+"["+surface+"]", c)
			if !existing[c] {
				t.Errorf("%s[%q] → %q is not an existing canonical (would create a new facet value)", tier, surface, c)
			}
		}
	}
}

func assertSlug(t *testing.T, what, s string) {
	t.Helper()
	if s == "" || s != trimLower(s) {
		t.Errorf("%s: canonical %q is not a lowercase no-space slug", what, s)
	}
}
