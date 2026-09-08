package matchanalysis

import "testing"

// Stages 1 and 2 keep their retry: nothing is served without them, so one more attempt is
// the difference between an analysis and no analysis.
func TestTheStagesNothingIsServedWithoutKeepTheirRetry(t *testing.T) {
	for _, stage := range []int{1, 2} {
		if got := attemptsForStage(stage); got != stageAttempts {
			t.Errorf("stage %d gets %d attempts, want %d", stage, got, stageAttempts)
		}
	}
}

// Stage 3 does not. Its verdict is already streamed and cached by the time it runs, so a
// second attempt buys a refinement nobody is waiting for — and production says it does not
// even buy that: three of three retries burned the identical 90 seconds and failed the same
// way. The retry exists for a HUNG call that the next attempt answers in seconds; a
// consistently slow provider is not that, and paying it twice is ninety seconds and one
// billed call for the same refusal.
func TestTheAuditGetsOneAttemptBecauseItsRetryOnlyEverRepeatedTheFailure(t *testing.T) {
	if got := attemptsForStage(3); got != 1 {
		t.Errorf("stage 3 gets %d attempts, want 1", got)
	}
}

// A stage nobody has an opinion about keeps the ordinary retry, so a fourth stage added
// later behaves like stages 1 and 2 rather than silently inheriting the audit's single shot.
func TestAnUnknownStageKeepsTheOrdinaryRetry(t *testing.T) {
	if got := attemptsForStage(9); got != stageAttempts {
		t.Errorf("stage 9 gets %d attempts, want %d", got, stageAttempts)
	}
}
