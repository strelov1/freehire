package handler

import (
	"testing"

	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/matchanalysis"
)

// The plan is the list the report uses (missing_have + missing_gap), not every requirement the
// analysis matched, so the bar's total equals the report's length.
func TestRunPlanCountsOnlyWhatTheReportCovers(t *testing.T) {
	a := &matchanalysis.Analysis{RequirementMatch: []matchanalysis.Requirement{
		{Text: "met one", Status: matchanalysis.StatusCovered},
		{Text: "have one", Status: matchanalysis.StatusMissingHave},
		{Text: "  ", Status: matchanalysis.StatusMissingGap},
		{Text: "gap one", Status: matchanalysis.StatusMissingGap},
	}}
	plan := runPlan(a)
	if len(plan) != 2 || plan[0].Requirement != "have one" || plan[1].Requirement != "gap one" {
		t.Fatalf("plan = %+v, want have one and gap one", plan)
	}
	for _, e := range plan {
		if e.Status != cv.AutopilotNotReached {
			t.Errorf("%q status %q, want not_reached", e.Requirement, e.Status)
		}
	}
}
