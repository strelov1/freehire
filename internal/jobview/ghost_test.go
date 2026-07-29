package jobview

import (
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/ghost"
)

var ghostNow = time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)

// stamped is a posting with a fresh absence stamp, so one structural criterion
// already fires and each test only has to add the axis it is about.
func stamped() GhostInput {
	return GhostInput{
		Now:          ghostNow,
		RealityClass: "likely-evergreen",
		ATSAbsentAt:  ghostNow,
		HasATSAbsent: true,
	}
}

func TestClassifyGhost_ClosedJobCarriesNoSignal(t *testing.T) {
	in := stamped()
	in.Closed = true
	in.Evidence = ghost.Evidence{Contributors: 5, Reports: 5}

	got := ClassifyGhost(in)
	if got != nil {
		t.Errorf("ghost = %+v, want nil — a closed posting warns nobody about anything", got)
	}
}

func TestClassifyGhost_NoCriteriaCarriesNoSignal(t *testing.T) {
	if got := ClassifyGhost(GhostInput{Now: ghostNow, RealityClass: "fresh"}); got != nil {
		t.Errorf("ghost = %+v, want nil at level none", got)
	}
}

// The anonymity gate is structural: below two contributors the counts are ABSENT
// from the payload, not zeroed or rounded, so there is nothing a later caller
// could forget to redact.
func TestClassifyGhost_WithholdsCountsBelowTheGate(t *testing.T) {
	in := stamped()
	in.Evidence = ghost.Evidence{Contributors: 1, SilentApplications: 1}
	got := ClassifyGhost(in)
	if got == nil {
		t.Fatal("ghost = nil, want a signal")
	}
	if got.Contributors != nil {
		t.Errorf("contributors = %v, want the field absent with a single witness", *got.Contributors)
	}
}

func TestClassifyGhost_ServesCountsAboveTheGate(t *testing.T) {
	in := stamped()
	in.Evidence = ghost.Evidence{Contributors: 3, SilentApplications: 2, Reports: 1}
	got := ClassifyGhost(in)
	if got == nil {
		t.Fatal("ghost = nil, want a signal")
	}
	if got.Contributors == nil || *got.Contributors != 3 {
		t.Errorf("contributors = %v, want 3", got.Contributors)
	}
}

func TestClassifyGhost_CarriesTheCriteriaAndTheScaleDenominator(t *testing.T) {
	got := ClassifyGhost(stamped())
	if got == nil {
		t.Fatal("ghost = nil, want a signal")
	}
	if got.Level != ghost.LevelPossible {
		t.Errorf("level = %q, want %q", got.Level, ghost.LevelPossible)
	}
	if len(got.Criteria) != 2 {
		t.Errorf("criteria = %v, want both structural criteria", got.Criteria)
	}
	if got.CriteriaTotal != ghost.CriteriaTotal {
		t.Errorf("criteria_total = %d, want %d — the scale needs its denominator", got.CriteriaTotal, ghost.CriteriaTotal)
	}
}

// The stamp's age must reach the reader: the checklist says "checked N days ago",
// and a criterion with no provenance is the accusation this design refuses to make.
func TestClassifyGhost_CarriesTheCheckedAtStamp(t *testing.T) {
	got := ClassifyGhost(stamped())
	if got == nil || got.ATSCheckedAt == nil {
		t.Fatalf("ghost = %+v, want the absence stamp served", got)
	}
	if want := ghostNow.Format(time.RFC3339); *got.ATSCheckedAt != want {
		t.Errorf("ats_checked_at = %q, want %q", *got.ATSCheckedAt, want)
	}
}

func TestClassifyGhost_OmitsTheStampWhenTheCriterionDidNotFire(t *testing.T) {
	got := ClassifyGhost(GhostInput{
		Now:          ghostNow,
		RealityClass: "likely-evergreen",
		Evidence:     ghost.Evidence{Contributors: 2, Reports: 2},
	})
	if got == nil {
		t.Fatal("ghost = nil, want a signal")
	}
	if got.ATSCheckedAt != nil {
		t.Errorf("ats_checked_at = %v, want absent — the cross-check never ran on this posting", *got.ATSCheckedAt)
	}
}
