package dictgap

import "testing"

func TestSkillGapCandidatesIncludesUnresolvedPhrase(t *testing.T) {
	got := SkillGapCandidates(map[string]int{"Frobnicator": 5})

	if len(got) != 1 {
		t.Fatalf("got %d candidates, want 1: %+v", len(got), got)
	}
	if got[0].Phrase != "Frobnicator" || got[0].Count != 5 {
		t.Fatalf("got %+v, want {Phrase: Frobnicator, Count: 5}", got[0])
	}
}

func TestSkillGapCandidatesExcludesResolvedPhrase(t *testing.T) {
	got := SkillGapCandidates(map[string]int{"Python": 100})

	if len(got) != 0 {
		t.Fatalf("got %d candidates, want 0 (Python resolves in skilltag): %+v", len(got), got)
	}
}

func TestSkillGapCandidatesCollapsesCaseAndPunctuationVariants(t *testing.T) {
	got := SkillGapCandidates(map[string]int{
		"Frobnicator":  5,
		"FROBNICATOR":  3,
		"frobnicator.": 2,
	})

	if len(got) != 1 {
		t.Fatalf("got %d candidates, want 1 merged candidate: %+v", len(got), got)
	}
	if got[0].Count != 10 {
		t.Fatalf("got count %d, want 10 (5+3+2)", got[0].Count)
	}
	if got[0].Phrase != "Frobnicator" {
		t.Fatalf("got display phrase %q, want %q (the most frequent original spelling)", got[0].Phrase, "Frobnicator")
	}
}

func TestSkillGapCandidatesSortsDescendingByCount(t *testing.T) {
	got := SkillGapCandidates(map[string]int{
		"Frobnicator": 3,
		"Widgetize":   9,
	})

	if len(got) != 2 {
		t.Fatalf("got %d candidates, want 2: %+v", len(got), got)
	}
	if got[0].Phrase != "Widgetize" || got[1].Phrase != "Frobnicator" {
		t.Fatalf("got %+v, want Widgetize (9) before Frobnicator (3)", got)
	}
}

func TestSkillGapCandidatesTreatsIdentityBearingSymbolsAsPartOfThePhrase(t *testing.T) {
	// C++ resolves to the "cpp" canonical (internal/dict/skilltag/dictionaries.go);
	// bare "C" does not. Stripping the trailing "++" would fold both into the same
	// normalized key "c" and, since C++'s count is higher, pick "C++" as the
	// bucket's display form — which resolves, so the WHOLE bucket (including C's
	// real, unresolved occurrences) would be dropped from the report.
	got := SkillGapCandidates(map[string]int{
		"C++": 10,
		"C":   7,
	})

	if len(got) != 1 {
		t.Fatalf("got %d candidates, want 1 (only bare C is unresolved): %+v", len(got), got)
	}
	if got[0].Phrase != "C" || got[0].Count != 7 {
		t.Fatalf("got %+v, want {Phrase: C, Count: 7} — C++ must not fold into C's bucket", got[0])
	}
}

func TestSkillGapCandidatesKeepsBareLetterSeparateFromItsSharpVariant(t *testing.T) {
	// F# resolves to "fsharp" (internal/dict/skilltag/dictionaries.go); bare "F"
	// has no entry at all. The trailing "#" must not be stripped, or both fold into
	// a bucket keyed on the bare letter and F's real occurrences are lost the same
	// way the C/C++ case above demonstrates.
	got := SkillGapCandidates(map[string]int{
		"F#": 4,
		"F":  6,
	})

	if len(got) != 1 {
		t.Fatalf("got %d candidates, want 1 (F# resolves, bare F does not): %+v", len(got), got)
	}
	if got[0].Phrase != "F" || got[0].Count != 6 {
		t.Fatalf("got %+v, want {Phrase: F, Count: 6}", got[0])
	}
}

func TestSkillGapCandidatesSkipsEmptyPhrase(t *testing.T) {
	got := SkillGapCandidates(map[string]int{"": 7, "   ": 4})

	if len(got) != 0 {
		t.Fatalf("got %d candidates, want 0 for blank phrases: %+v", len(got), got)
	}
}
