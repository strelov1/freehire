package matchanalysis

// attemptsForStage is how many times a stage's LLM call is tried.
//
// Stages 1 and 2 keep the retry `stageAttempts` names: nothing is served without them. The
// audit gets one shot: its subject, the Stage 2 verdict, is already streamed and is what gets
// served if the audit never lands, and a retry of a consistently slow provider only repeats the
// failure (the retry exists for a hung call the next attempt answers quickly).
//
// An unrecognised stage keeps the ordinary retry.
func attemptsForStage(stage int) int {
	if stage == 3 {
		return 1
	}

	return stageAttempts
}
