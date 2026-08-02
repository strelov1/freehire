package userjob

import "testing"

func TestCountByStageKeepsTheWholeVocabulary(t *testing.T) {
	got := CountByStage([]StageCount{
		{Stage: "applied", Count: 70},
		{Stage: "screening", Count: 5},
		{Stage: "interview", Count: 20},
	})

	if got.Applications != 95 {
		t.Errorf("Applications = %d, want 95", got.Applications)
	}
	// Every stage is present, including the ones nobody is at: a caller reading a count of
	// nothing must not have to tell it apart from a key the server left out.
	for _, stage := range Stages {
		if _, ok := got.Stages[stage]; !ok {
			t.Errorf("Stages is missing %q", stage)
		}
	}
	if len(got.Stages) != len(Stages) {
		t.Errorf("Stages has %d keys, want %d", len(got.Stages), len(Stages))
	}
	if got.Stages["offer"] != 0 {
		t.Errorf("Stages[offer] = %d, want 0", got.Stages["offer"])
	}
}

func TestCountByStageSumsToApplications(t *testing.T) {
	got := CountByStage([]StageCount{
		{Stage: "applied", Count: 3},
		{Stage: "interview", Count: 4},
		{Stage: "rejected", Count: 5},
	})
	var sum int64
	for _, n := range got.Stages {
		sum += n
	}
	if sum != got.Applications {
		t.Errorf("stage counts sum = %d, want Applications = %d", sum, got.Applications)
	}
}

// An applied row with no explicit stage arrives as an empty Stage. It is an application
// waiting on a first reply, which is what `applied` means.
func TestCountByStageCountsAnUnsetStageAsApplied(t *testing.T) {
	got := CountByStage([]StageCount{{Stage: "", Count: 6}})
	if got.Stages["applied"] != 6 {
		t.Errorf("Stages[applied] = %d, want 6", got.Stages["applied"])
	}
	if got.Applications != 6 {
		t.Errorf("Applications = %d, want 6", got.Applications)
	}
}

// `stage` is a free text column with no CHECK, so a value outside the vocabulary is
// possible — from older data, a hand-written UPDATE, or a rolled-back deploy. It is still an
// application, and the counts must still sum, so it lands where an unset stage lands rather
// than vanishing from the total or inventing a key outside the vocabulary.
func TestCountByStageFoldsAnUnknownStageRatherThanLosingIt(t *testing.T) {
	got := CountByStage([]StageCount{{Stage: "bogus", Count: 4}})
	if got.Applications != 4 {
		t.Errorf("Applications = %d, want 4", got.Applications)
	}
	var sum int64
	for _, n := range got.Stages {
		sum += n
	}
	if sum != 4 {
		t.Errorf("stage counts sum = %d, want 4", sum)
	}
	if len(got.Stages) != len(Stages) {
		t.Errorf("Stages has %d keys, want %d — an unknown stage must not add one",
			len(got.Stages), len(Stages))
	}
}

func TestCountByStageEmptyInputStillCarriesEveryStage(t *testing.T) {
	got := CountByStage(nil)
	if got.Applications != 0 {
		t.Errorf("Applications = %d, want 0", got.Applications)
	}
	if len(got.Stages) != len(Stages) {
		t.Errorf("Stages has %d keys, want %d", len(got.Stages), len(Stages))
	}
}
