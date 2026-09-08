package matchanalysis

// attemptsForStage is how many times a stage's LLM call is tried.
//
// Stages 1 and 2 keep the retry `stageAttempts` names: nothing is served without them, so
// one more attempt is the difference between an analysis and no analysis.
//
// The adversarial audit gets one shot. Its subject — the Stage 2 verdict — has already been
// streamed to the reader and is what gets served if the audit never lands, so a second
// attempt refines something nobody is waiting for. Production says it does not even do
// that: measured over 2026-09-08, three of three audit retries burned the identical 90
// seconds and failed the same way, and the analyses that DID complete their audit did so on
// the first attempt in a chain that finished in 28 seconds.
//
// That is the case the retry was not written for. It exists for a HUNG call that the next
// attempt answers in seconds (see stageAttempts); a provider that is consistently slow is a
// different animal, and paying it twice costs ninety seconds of a reader's time and one
// billed call to receive the same refusal.
//
// An unrecognised stage keeps the ordinary retry, so a fourth stage added later behaves
// like the two that matter rather than silently inheriting the audit's single shot.
func attemptsForStage(stage int) int {
	if stage == 3 {
		return 1
	}

	return stageAttempts
}
