package matchanalysis

import "time"

// timeoutForStage is how long one attempt of a stage may take, given the budget the caller
// built the client with (matchAnalysisLLMTimeout).
//
// Stages 1 and 2 keep it: a reader is blocked on both, so a longer wait buys a longer blank
// screen. The audit gets twice that, which is what its two-attempt retry used to cost, so the
// worst case per analysis is unchanged and the stage can finish inside it. It can afford the
// wait because the Stage 2 verdict is already served.
//
// A caller that named no budget gets none imposed: doubling zero would invent one.
func timeoutForStage(stage int, base time.Duration) time.Duration {
	if stage == 3 && base > 0 {
		return 2 * base
	}

	return base
}
