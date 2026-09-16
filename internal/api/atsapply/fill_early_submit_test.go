package atsapply

import (
	"errors"
	"slices"
	"testing"
)

// runFillLoop is the pure loop-control decision `fillAndSubmit` delegates to: fill each
// field in order, and — only right after a text/textarea field, the one kind whose
// interaction can trigger the form's own submit binding — ask presentAfter whether the
// submit control is still on the page. These tests drive that decision with plain
// closures, no chromedp and no real browser, per this package's existing pattern of
// testing a browser-observed signal as a parameter (classifyFillFailure,
// pollEndedByDeadline) rather than testing the live DOM check itself.

// The submit control disappearing right after a text field must stop the loop before it
// reaches any later field — continuing would fill fields that may no longer exist on a
// page whose form may already be gone.
func TestRunFillLoop_SubmitControlGoneAfterTextFieldStopsRemainingFields(t *testing.T) {
	kinds := []string{"text", "select", "text"}
	var filled []int

	outcome, err := runFillLoop(kinds, func(i int) error {
		filled = append(filled, i)
		return nil
	}, func(i int) bool {
		return i != 0 // gone right after field 0 fills
	})

	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !outcome.stoppedEarly {
		t.Error("stoppedEarly = false, want true")
	}
	if outcome.filledCount != 1 {
		t.Errorf("filledCount = %d, want 1", outcome.filledCount)
	}
	if want := []int{0}; !slices.Equal(filled, want) {
		t.Errorf("filled = %v, want %v (fields after the disappearance must never fill)", filled, want)
	}
}

// The ordinary case: the submit control stays present after every text/textarea field, so
// the loop fills every field in order exactly as it does today.
func TestRunFillLoop_SubmitControlStillPresentContinuesFilling(t *testing.T) {
	kinds := []string{"text", "textarea", "select"}
	var filled []int

	outcome, err := runFillLoop(kinds, func(i int) error {
		filled = append(filled, i)
		return nil
	}, func(i int) bool {
		return true // never disappears
	})

	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if outcome.stoppedEarly {
		t.Error("stoppedEarly = true, want false")
	}
	if outcome.filledCount != len(kinds) {
		t.Errorf("filledCount = %d, want %d", outcome.filledCount, len(kinds))
	}
	if want := []int{0, 1, 2}; !slices.Equal(filled, want) {
		t.Errorf("filled = %v, want %v", filled, want)
	}
}

// A sequence that never triggers an early submit must reach the end of the plan — the
// caller (fillAndSubmit) relies on stoppedEarly=false, filledCount==len(kinds) as the
// single signal that it is safe to proceed to its own, one-time submit click.
func TestRunFillLoop_NoEarlyStopReachesTheEndOfThePlan(t *testing.T) {
	kinds := []string{"text", "text", "text"}

	outcome, err := runFillLoop(kinds, func(i int) error { return nil }, func(i int) bool { return true })

	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if outcome.stoppedEarly {
		t.Error("stoppedEarly = true, want false — nothing should have stopped this loop")
	}
	if outcome.filledCount != len(kinds) {
		t.Errorf("filledCount = %d, want %d (every field must have filled)", outcome.filledCount, len(kinds))
	}
}

// select/checkbox_group/file interactions cannot trigger a form's own submit binding
// (no key event), so the presence check must never run after one of those — checking would
// be wasted work at best and, if the check's own fail-closed default ever mismatched a live
// page, a wrongly-stopped fill for a field kind that was never at risk.
func TestRunFillLoop_NonTextKindsAreNeverFollowedByThePresenceCheck(t *testing.T) {
	kinds := []string{"select", "checkbox_group", "file"}
	var checked []int

	outcome, err := runFillLoop(kinds, func(i int) error { return nil }, func(i int) bool {
		checked = append(checked, i)
		return true
	})

	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if outcome.stoppedEarly {
		t.Error("stoppedEarly = true, want false")
	}
	if outcome.filledCount != len(kinds) {
		t.Errorf("filledCount = %d, want %d", outcome.filledCount, len(kinds))
	}
	if len(checked) != 0 {
		t.Errorf("presentAfter was called for field(s) %v, want it never called for these kinds", checked)
	}
}

// The submit control can disappear right after the LAST field fills, not only a middle
// one — the loop then has filled every field (filledCount == len(kinds)) AND stopped early
// (stoppedEarly == true) at once. The caller must key off stoppedEarly, not filledCount,
// to decide whether it is safe to click submit: this is exactly the combination that would
// wrongly reach a second submit click if a future change compared filledCount instead.
func TestRunFillLoop_SubmitControlGoneAfterTheLastFieldStillCountsAsStoppedEarly(t *testing.T) {
	kinds := []string{"text", "text"}

	outcome, err := runFillLoop(kinds, func(i int) error { return nil }, func(i int) bool {
		return i != len(kinds)-1 // gone only right after the last field fills
	})

	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !outcome.stoppedEarly {
		t.Error("stoppedEarly = false, want true — the submit control disappeared after the last field too")
	}
	if outcome.filledCount != len(kinds) {
		t.Errorf("filledCount = %d, want %d (every field did fill, even though the loop stopped early)", outcome.filledCount, len(kinds))
	}
}

// A field that fails to fill must abort the loop immediately, exactly as fillAndSubmit
// does today — the presence check is only ever reached after a field fills successfully.
func TestRunFillLoop_AFillErrorStopsTheLoopWithoutCheckingPresence(t *testing.T) {
	kinds := []string{"text", "text"}
	checkedPresence := false
	wantErr := errors.New("fill test sentinel")

	outcome, err := runFillLoop(kinds, func(i int) error {
		if i == 0 {
			return wantErr
		}
		return nil
	}, func(i int) bool {
		checkedPresence = true
		return true
	})

	if err != wantErr {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if outcome.filledCount != 0 {
		t.Errorf("filledCount = %d, want 0 (field 0 never finished filling)", outcome.filledCount)
	}
	if checkedPresence {
		t.Error("presentAfter was called after a fill error, want it never called")
	}
}
