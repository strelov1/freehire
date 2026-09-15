package atsapply

import (
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/platform/config"
)

// Nine live cloud runs, measured 2026-09-15: 41, 73, 83, 91, 129, 158, 229, 282 and 347
// seconds. The agent reads a page, decides, and acts, so it is minutes rather than seconds —
// and the budget has to cover the slow end, not the middle.
const slowestObservedCloudRun = 347 * time.Second

func TestCloudRunBudget_CoversTheSlowestObservedRun(t *testing.T) {
	if browserUseWaitTimeout <= slowestObservedCloudRun {
		t.Errorf("browserUseWaitTimeout = %s, want more than the slowest run actually measured (%s)",
			browserUseWaitTimeout, slowestObservedCloudRun)
	}
}

// The outer per-attempt deadline must be the LAST thing to fire, not the first.
//
// It was 120s against the cloud path's own 180s, so the outer one always won — and losing
// that way is expensive: Wait returns a transport error, the attempt is reported unconfirmed
// (it might already have submitted), and unconfirmed dead-letters immediately. A live Lever
// entry died exactly there on 2026-09-13, 115 seconds in, while the agent went on to finish
// at 229 seconds and report that it had deliberately NOT submitted. Letting the inner
// timeout fire first turns that into the ordinary, recoverable outcome it always was.
func TestOuterAttemptDeadlineOutlastsTheCloudRunBudget(t *testing.T) {
	t.Setenv("AUTO_APPLY_CALL_TIMEOUT_SECONDS", "")

	outer := config.LoadAutoApply().CallTimeout

	if outer <= browserUseWaitTimeout {
		t.Errorf("AUTO_APPLY_CALL_TIMEOUT_SECONDS default = %s, want it to outlast the cloud run budget %s; "+
			"whichever fires first decides, and only the inner one can read the agent's report",
			outer, browserUseWaitTimeout)
	}
}
