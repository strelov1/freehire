package handler

import (
	"testing"

	"github.com/strelov1/freehire/internal/candidate/hardconstraint"
)

// A plain env read, not memoized (see autoApplyEligibilityEnforce's doc comment) — this
// is what makes it possible to test both branches with t.Setenv in one test binary at
// all, which a sync.OnceValue-memoized version (an earlier draft of this flag) could not.
func TestAutoApplyEligibilityEnforce_ReadsTheEnvVarEachCall(t *testing.T) {
	t.Setenv("AUTO_APPLY_ELIGIBILITY_ENFORCE", "")
	if autoApplyEligibilityEnforce() {
		t.Error("want shadow mode (false) when the env var is unset")
	}
	t.Setenv("AUTO_APPLY_ELIGIBILITY_ENFORCE", "1")
	if !autoApplyEligibilityEnforce() {
		t.Error("want enforced (true) once the env var is set to 1")
	}
	t.Setenv("AUTO_APPLY_ELIGIBILITY_ENFORCE", "")
	if autoApplyEligibilityEnforce() {
		t.Error("want shadow mode (false) again once the env var is cleared")
	}
}

func TestFirstEligibilityBlocker_ReturnsAnUnmetWorkAuthBlocker(t *testing.T) {
	blockers := []hardconstraint.Blocker{
		{Category: hardconstraint.CategoryExperience, Met: false},
		{Category: hardconstraint.CategoryWorkAuth, Met: false, Reason: "no visa sponsorship"},
	}
	b, ok := firstEligibilityBlocker(blockers)
	if !ok {
		t.Fatal("want a blocker, got none")
	}
	if b.Category != hardconstraint.CategoryWorkAuth || b.Reason != "no visa sponsorship" {
		t.Errorf("blocker = %+v, want the work-authorization one", b)
	}
}

func TestFirstEligibilityBlocker_ReturnsAnUnmetLocationBlocker(t *testing.T) {
	blockers := []hardconstraint.Blocker{
		{Category: hardconstraint.CategoryLocationWorkMode, Met: false, Reason: "on-site in US"},
	}
	b, ok := firstEligibilityBlocker(blockers)
	if !ok || b.Category != hardconstraint.CategoryLocationWorkMode {
		t.Fatalf("blocker = %+v, ok = %v, want the location-and-work-mode one", b, ok)
	}
}

func TestFirstEligibilityBlocker_IgnoresIrrelevantCategories(t *testing.T) {
	// Experience/education/certification/language bear on fit, not on whether the
	// submission itself would misrepresent the candidate — never a gate reason.
	blockers := []hardconstraint.Blocker{
		{Category: hardconstraint.CategoryExperience, Met: false},
		{Category: hardconstraint.CategoryEducation, Met: false},
		{Category: hardconstraint.CategoryCertification, Met: false},
		{Category: hardconstraint.CategoryLanguage, Met: false},
	}
	if _, ok := firstEligibilityBlocker(blockers); ok {
		t.Error("want no gate blocker among non-eligibility categories")
	}
}

func TestFirstEligibilityBlocker_IgnoresMetBlockers(t *testing.T) {
	blockers := []hardconstraint.Blocker{
		{Category: hardconstraint.CategoryWorkAuth, Met: true},
		{Category: hardconstraint.CategoryLocationWorkMode, Met: true},
	}
	if _, ok := firstEligibilityBlocker(blockers); ok {
		t.Error("want no gate blocker when every eligibility category is met")
	}
}

func TestFirstEligibilityBlocker_NoBlockersAtAllProceeds(t *testing.T) {
	// Missing evidence on either side means hardconstraint.Evaluate emits no blocker at
	// all for that category — never emit a false blocker — so an empty slice must not be
	// treated as a mismatch.
	if _, ok := firstEligibilityBlocker(nil); ok {
		t.Error("want no gate blocker for an empty evaluation")
	}
}
