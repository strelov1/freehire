package matchanalysis

import (
	"testing"
	"time"
)

// The audit's budget is exactly what its retry used to cost. Before #2647 a slow Stage 3
// spent two attempts of the ordinary budget to fail; it now spends one attempt of twice
// that. The worst case per analysis is unchanged and the stage can actually finish.
func TestTheAuditsBudgetIsWhatItsRetryUsedToCost(t *testing.T) {
	base := 90 * time.Second
	if got, want := timeoutForStage(3, base), 2*base; got != want {
		t.Errorf("stage 3 budget = %v, want %v — the two attempts it replaces", got, want)
	}
}

// The stages a reader is blocked on keep the caller's budget. Nothing is served until they
// answer, so a longer wait there is a longer blank screen, not a better verdict.
func TestTheStagesAReaderWaitsOnKeepTheCallersBudget(t *testing.T) {
	base := 90 * time.Second
	for _, stage := range []int{1, 2} {
		if got := timeoutForStage(stage, base); got != base {
			t.Errorf("stage %d budget = %v, want the caller's %v", stage, got, base)
		}
	}
}

// An unrecognised stage keeps the caller's budget, so a fourth stage added later waits as
// long as the two that matter rather than silently inheriting the audit's doubled one.
func TestAnUnknownStageKeepsTheCallersBudget(t *testing.T) {
	base := 90 * time.Second
	if got := timeoutForStage(9, base); got != base {
		t.Errorf("stage 9 budget = %v, want the caller's %v", got, base)
	}
}

// A caller that set no budget gets none imposed here: the client's own default stands, and
// doubling zero would be inventing one.
func TestNoBudgetStaysNoBudget(t *testing.T) {
	if got := timeoutForStage(3, 0); got != 0 {
		t.Errorf("stage 3 budget = %v, want 0 when the caller named none", got)
	}
}
