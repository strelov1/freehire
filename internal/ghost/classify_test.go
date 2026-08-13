package ghost

import (
	"slices"
	"testing"
	"time"
)

var now = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// base is a posting with no evidence of any kind: an ordinary job nobody has
// complained about. Tests mutate one axis off it, as internal/jobreality's own
// tests do, so what a case is actually asserting is visible in its diff.
func base() Input {
	return Input{
		Now:          now,
		RealityClass: "stale",
	}
}

// evergreen turns on the evergreen_posting criterion.
func evergreen(in Input) Input {
	in.RealityClass = "likely-evergreen"
	return in
}

// absent turns on the ats_absent criterion with a stamp taken today.
func absent(in Input) Input {
	in.ATSAbsentAt = now
	in.HasATSAbsent = true
	return in
}

func TestClassify_NoEvidenceIsNone(t *testing.T) {
	got := Classify(base())
	if got.Level != LevelNone {
		t.Errorf("level = %q, want %q", got.Level, LevelNone)
	}
	if len(got.Criteria) != 0 {
		t.Errorf("criteria = %v, want none", got.Criteria)
	}
}

func TestClassify_OneCriterionIsNotEnough(t *testing.T) {
	got := Classify(evergreen(base()))
	if got.Level != LevelNone {
		t.Errorf("level = %q, want %q — a lone criterion must not mark a posting", got.Level, LevelNone)
	}
	if !slices.Contains(got.Criteria, CriterionEvergreenPosting) {
		t.Errorf("criteria = %v, want it to record %q even at level none", got.Criteria, CriterionEvergreenPosting)
	}
}

func TestClassify_TwoStructuralCriteriaReachPossible(t *testing.T) {
	got := Classify(absent(evergreen(base())))
	if got.Level != LevelPossible {
		t.Errorf("level = %q, want %q", got.Level, LevelPossible)
	}
	want := []string{CriterionEvergreenPosting, CriterionATSAbsent}
	if !slices.Equal(got.Criteria, want) {
		t.Errorf("criteria = %v, want %v", got.Criteria, want)
	}
}

// The doctrine guard. Structural signals describe the shape of a posting and can
// never witness what happened to an applicant, so no amount of them may produce
// the stronger claim. No scenario-shaped test would catch this rule being
// inverted, which is why it is asserted directly.
func TestClassify_StructuralEvidenceNeverReachesLikely(t *testing.T) {
	in := absent(evergreen(base()))
	for contributors := 0; contributors < 2; contributors++ {
		in.Contributors = contributors
		if got := Classify(in); got.Level != LevelPossible {
			t.Errorf("contributors=%d: level = %q, want %q", contributors, got.Level, LevelPossible)
		}
	}
}

func TestClassify_OutcomeEvidenceFromTwoPeopleReachesLikely(t *testing.T) {
	in := evergreen(base())
	in.SilentApplications = 2
	in.Contributors = 2

	got := Classify(in)
	if got.Level != LevelLikely {
		t.Errorf("level = %q, want %q", got.Level, LevelLikely)
	}
	if !slices.Contains(got.Criteria, CriterionSilentApplications) {
		t.Errorf("criteria = %v, want it to record %q", got.Criteria, CriterionSilentApplications)
	}
}

// Two people, but the only criterion that fires is the one they contributed to.
// The stronger claim needs corroboration from a second criterion, not only a
// second person.
func TestClassify_OutcomeEvidenceAloneIsNotLikely(t *testing.T) {
	in := base()
	in.Reports = 2
	in.Contributors = 2

	got := Classify(in)
	if got.Level != LevelPossible {
		t.Errorf("level = %q, want %q", got.Level, LevelPossible)
	}
}

// Both outcome channels firing is two criteria, so two people who arrived by
// different routes do reach the stronger claim.
func TestClassify_BothOutcomeChannelsCorroborateEachOther(t *testing.T) {
	in := base()
	in.SilentApplications = 1
	in.Reports = 1
	in.Contributors = 2

	if got := Classify(in); got.Level != LevelLikely {
		t.Errorf("level = %q, want %q", got.Level, LevelLikely)
	}
}

// A single person on both channels fires two criteria but is still one witness:
// the count that gates the stronger claim is people, not criteria.
func TestClassify_OnePersonOnBothChannelsStaysPossible(t *testing.T) {
	in := base()
	in.SilentApplications = 1
	in.Reports = 1
	in.Contributors = 1

	if got := Classify(in); got.Level != LevelPossible {
		t.Errorf("level = %q, want %q", got.Level, LevelPossible)
	}
}

// The cross-check worker re-stamps every run, so a stamp that has aged out means
// the worker stopped — and a stopped worker must fall silent rather than keep
// accusing the catalogue from a frozen snapshot. The expiry lives inside
// Classify so that no caller can forget to apply it.
func TestClassify_ExpiredAbsenceStampDoesNotFire(t *testing.T) {
	in := evergreen(base())
	in.ATSAbsentAt = now.AddDate(0, 0, -15)
	in.HasATSAbsent = true

	got := Classify(in)
	if slices.Contains(got.Criteria, CriterionATSAbsent) {
		t.Errorf("criteria = %v, want %q absent — the stamp is 15 days old", got.Criteria, CriterionATSAbsent)
	}
	if got.Level != LevelNone {
		t.Errorf("level = %q, want %q — only the evergreen criterion survives", got.Level, LevelNone)
	}
}

// The threshold is the last tolerated day, not the first offending one — the
// same convention internal/userjob's silence ladder uses.
func TestClassify_AbsenceStampAtTheThresholdStillFires(t *testing.T) {
	in := evergreen(base())
	in.ATSAbsentAt = now.AddDate(0, 0, -14)
	in.HasATSAbsent = true

	if got := Classify(in); !slices.Contains(got.Criteria, CriterionATSAbsent) {
		t.Errorf("criteria = %v, want %q to fire at exactly 14 days", got.Criteria, CriterionATSAbsent)
	}
}

func TestClassify_CriteriaTotalCountsEveryCriterion(t *testing.T) {
	if CriteriaTotal != 4 {
		t.Errorf("CriteriaTotal = %d, want 4 — the scale's denominator", CriteriaTotal)
	}
}

// CriterionCodes is what cmd/gen-contracts emits and what the SPA builds its
// checklist rows from, but Classify appends its own criteria in its own order —
// two lists free to drift. Firing everything at once pins them together: the
// vocabulary the interface renders is exactly the one the classifier reports, in
// the order it reports it.
func TestClassify_ReportsTheWholeVocabularyInOrder(t *testing.T) {
	in := absent(evergreen(base()))
	in.SilentApplications = 1
	in.Reports = 1

	got := Classify(in).Criteria
	if !slices.Equal(got, CriterionCodes[:]) {
		t.Errorf("criteria = %v, want %v — every criterion, in the fixed order", got, CriterionCodes)
	}
}

func TestClassify_Deterministic(t *testing.T) {
	in := absent(evergreen(base()))
	in.SilentApplications = 3
	in.Contributors = 2

	first, second := Classify(in), Classify(in)
	if first.Level != second.Level || !slices.Equal(first.Criteria, second.Criteria) {
		t.Errorf("not deterministic: %+v != %+v", first, second)
	}
}

// The rollout's feature flag IS this threshold, so it is asserted as a property
// rather than left as a consequence of other tests.
//
// On the day the migration and image land, ats_absent_at is unpopulated everywhere
// and no report exists, so the only criterion that can fire is evergreen_posting.
// Convergence needs two. The catalogue is therefore silent until cmd/ghost-crosscheck
// is deliberately run with --apply — a flag that cannot be left on by accident,
// because it is the logic rather than a switch beside it.
func TestClassify_SilentUntilASecondCriterionExists(t *testing.T) {
	for _, realityClass := range []string{"fresh", "stale", "likely-evergreen"} {
		in := base()
		in.RealityClass = realityClass
		// The world as it is at deploy: no cross-check has run, nobody has
		// reported, nobody's tracked application has gone silent.
		in.HasATSAbsent = false
		in.Contributors = 0
		in.SilentApplications = 0
		in.Reports = 0

		if got := Classify(in); got.Level != LevelNone {
			t.Errorf("reality %q: level = %q, want %q — the catalogue must be silent at deploy",
				realityClass, got.Level, LevelNone)
		}
	}
}
