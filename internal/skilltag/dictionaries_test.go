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

// The dictionary deliberately covers the NON-engineering roles an IT company hires
// for — recruiting, HR, finance, legal, operations, customer success. That is right
// for a skills facet, which describes any posting. It is wrong as evidence that a
// board is a technical employer: cmd/prune's board report treated "has any skill" as
// "has posted something technical", so a recruiting coordinator tagged
// {stakeholder-management, candidate-experience} vouched for the whole board.
//
// HasEngineering is the narrower question that report should have been asking.
// Tech-industry craft that is not software engineering — developer relations,
// technical writing, business analysis, pre-sales — counts as engineering here on
// purpose: it is posted by technical employers, and the conservative error is to keep
// a board, never to retire a live one.
func TestHasEngineering(t *testing.T) {
	cases := []struct {
		name   string
		skills []string
		want   bool
	}{
		{"nothing tagged", nil, false},
		{"pure recruiting", []string{"talent-sourcing", "candidate-experience"}, false},
		{"pure hr", []string{"employee-relations", "performance-management"}, false},
		{"pure finance", []string{"accounts-payable", "financial-reporting"}, false},
		{"pure legal", []string{"contract-negotiation", "regulatory-compliance"}, false},
		{"pure operations", []string{"stakeholder-management", "process-improvement"}, false},
		{"pure customer success", []string{"customer-onboarding", "churn-prevention"}, false},
		// One engineering canonical is enough — the board has posted something technical.
		{"recruiter who also names a stack", []string{"talent-sourcing", "python"}, true},
		{"plain engineering", []string{"kubernetes"}, true},
		// Tech-industry craft counts, deliberately.
		{"developer relations", []string{"developer-advocacy"}, true},
		{"technical writing", []string{"docs-as-code"}, true},
		{"business analysis", []string{"user-stories"}, true},
		{"pre-sales", []string{"solution-design"}, true},
		// An unknown canonical is not evidence of absence: keep the board.
		{"unknown canonical", []string{"something-new"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := HasEngineering(c.skills); got != c.want {
				t.Errorf("HasEngineering(%v) = %v, want %v", c.skills, got, c.want)
			}
		})
	}
}

// Every non-engineering canonical must be one the dictionary actually emits — a
// marker on a canonical no alias resolves to would be dead config, and would hide a
// typo that silently reclassifies nothing.
func TestNonEngineeringCanonicalsAreReal(t *testing.T) {
	emitted := map[string]bool{}
	for _, c := range wordAliases {
		emitted[c] = true
	}
	for _, p := range phraseAliases {
		emitted[p.canonical] = true
	}
	for c := range nonEngineeringCanonicals {
		if !emitted[c] {
			t.Errorf("nonEngineeringCanonicals[%q] is not a canonical any alias resolves to", c)
		}
	}
}
