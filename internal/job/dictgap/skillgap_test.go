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

func TestSkillGapCandidatesSkipsEmptyPhrase(t *testing.T) {
	got := SkillGapCandidates(map[string]int{"": 7, "   ": 4})

	if len(got) != 0 {
		t.Fatalf("got %d candidates, want 0 for blank phrases: %+v", len(got), got)
	}
}
