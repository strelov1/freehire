package skilltag

import (
	"slices"
	"testing"
)

func TestAliasesGathersEveryTier(t *testing.T) {
	tests := []struct {
		name      string
		canonical string
		want      []string // a subset that must be present, not the whole answer
	}{
		// The word tier: the canonical's own spelling and its short form.
		{"word aliases", "kubernetes", []string{"k8s", "kubernetes"}},
		// The phrase tier, for spellings the word pass cannot see.
		{"phrase aliases", "cpp", []string{"c++", "c/c++"}},
		// The acronym tiers. "rag" is reachable through a phrase and through two
		// separate acronym tables; all three spellings are things a reader may meet.
		{"acronym tiers", "rag", []string{"RAG", "retrieval augmented generation"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Aliases(tc.canonical)
			for _, want := range tc.want {
				if !slices.Contains(got, want) {
					t.Errorf("Aliases(%q) = %v, missing %q", tc.canonical, got, want)
				}
			}
		})
	}
}

// Every tier, exhaustively. Pinning a handful of canonicals cannot notice a dropped
// tier: no canonical today is reachable ONLY through an acronym table, so deleting the
// sharedAcronyms loop would leave every example still passing through its word or
// phrase alias. Walking the tables themselves is what makes a sixth tier added to
// Canonicals() and forgotten here a failure.
func TestAliasesCoversEveryAliasTable(t *testing.T) {
	present := func(t *testing.T, tier, alias, canonical string) {
		t.Helper()
		if !slices.Contains(Aliases(canonical), alias) {
			t.Errorf("Aliases(%q) is missing %q from %s", canonical, alias, tier)
		}
	}
	for alias, canonical := range wordAliases {
		present(t, "wordAliases", alias, canonical)
	}
	for _, p := range phraseAliases {
		present(t, "phraseAliases", p.alias, p.canonical)
	}
	for alias, canonical := range sharedAcronyms {
		present(t, "sharedAcronyms", alias, canonical)
	}
	for alias, canonical := range resumeAcronyms {
		present(t, "resumeAcronyms", alias, canonical)
	}
	for alias, scoped := range categoryScopedAcronyms {
		present(t, "categoryScopedAcronyms", alias, scoped.canonical)
	}
}

// The glossary renders this list, so it must be stable between builds — Go's map
// iteration is not — and must not spell one thing twice: "RAG" is in two acronym
// tables at once.
func TestAliasesAreSortedAndDeduplicated(t *testing.T) {
	for _, canonical := range []string{"rag", "kubernetes", "safe-agile"} {
		got := Aliases(canonical)
		if !slices.IsSorted(got) {
			t.Errorf("Aliases(%q) = %v, not sorted", canonical, got)
		}
		if len(slices.Compact(slices.Clone(got))) != len(got) {
			t.Errorf("Aliases(%q) = %v, has a duplicate", canonical, got)
		}
	}
}

func TestAliasesOfANonCanonicalIsEmpty(t *testing.T) {
	if got := Aliases("some-new-thing"); len(got) != 0 {
		t.Errorf("Aliases(\"some-new-thing\") = %v, want empty", got)
	}
}

// Every canonical is reachable by construction — Canonicals() is drawn from the alias
// tables — so a canonical with no alias would mean a tier this function forgot.
func TestEveryCanonicalHasAtLeastOneAlias(t *testing.T) {
	for _, canonical := range Canonicals() {
		if len(Aliases(canonical)) == 0 {
			t.Errorf("Aliases(%q) is empty", canonical)
		}
	}
}
