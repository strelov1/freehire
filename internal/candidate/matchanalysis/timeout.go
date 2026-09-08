package matchanalysis

import "time"

// timeoutForStage is how long one attempt of a stage may take, given the budget the caller
// built the client with (matchAnalysisLLMTimeout today).
//
// Stages 1 and 2 keep it. A reader is blocked on both — nothing is served until they answer
// — so a longer wait there buys a longer blank screen, not a better verdict.
//
// The adversarial audit gets twice that, and the number is not a guess: it is exactly what
// the audit's retry used to cost. Before #2647 a slow Stage 3 spent two attempts of the
// ordinary budget and failed; it now spends one attempt of twice the budget. The worst case
// per analysis is unchanged, and the stage can actually finish inside it.
//
// It can afford the wait because its subject is already served. The Stage 2 verdict has been
// streamed and is exactly what a reader gets when the audit does not land, so a slow audit
// delays a refinement rather than an answer.
//
// The measurement behind it: over roughly three hours on 2026-09-08 the audit failed nine
// times and landed none, every one of them at exactly 90 seconds, while Stage 2 — the same
// weight of judgement over the same material — answered in 45 to 83. A stage that needs more
// than its neighbour's worst case was never going to fit in its neighbour's budget.
//
// A caller that named no budget gets none imposed: the client's own default stands, and
// doubling zero would be inventing one.
func timeoutForStage(stage int, base time.Duration) time.Duration {
	if stage == 3 && base > 0 {
		return 2 * base
	}

	return base
}
