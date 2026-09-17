package userjob

// ReplyRateSide is one side of the reply-rate comparison: how many observable
// applications there were (the applicant has a connected mailbox, so a reply would
// have been seen) and how many of them received a non-retracted employer_reply.
type ReplyRateSide struct {
	Applications int64 `json:"applications"`
	Answered     int64 `json:"answered"`
}

// ReplyRateBenchmark compares the caller's own reply rate against the global one,
// both counted under the same observable/answered definitions.
type ReplyRateBenchmark struct {
	You    ReplyRateSide `json:"you"`
	Global ReplyRateSide `json:"global"`
}

// ObservableSampleGate is the minimum observable-application count a response-rate
// figure needs before it is served — shared with
// internal/api/handler.responseSampleGate rather than restated, because both gate the
// identical quantity (observable applications before a response/reply rate is
// trustworthy): the per-company figure and this personal-vs-global one. Below it,
// absence is the honest answer, not a zero or an estimate.
const ObservableSampleGate = 10

// GateReplyRateBenchmark returns the comparison when both sides clear
// ObservableSampleGate, or nil when either does not — including when the caller has no
// connected mailbox, which surfaces here as zero observable applications rather than a
// separate check.
func GateReplyRateBenchmark(you, global ReplyRateSide) *ReplyRateBenchmark {
	if you.Applications < ObservableSampleGate || global.Applications < ObservableSampleGate {
		return nil
	}
	return &ReplyRateBenchmark{You: you, Global: global}
}

// ExcludeCallerFromGlobal removes the caller's own observable/answered counts from a
// platform-wide total, so the "global" side of the comparison means everyone ELSE —
// without this, a caller with a non-trivial share of the platform's observable
// applications would be partly comparing themselves to themselves. Each field clamps
// at zero independently: globalTotal comes from a periodic rollup while you is read
// live, so a caller's very recent application may not have reached the rollup yet, and
// a negative count is nonsense to serve rather than a signal worth surfacing.
func ExcludeCallerFromGlobal(you, globalTotal ReplyRateSide) ReplyRateSide {
	return ReplyRateSide{
		Applications: nonNegative(globalTotal.Applications - you.Applications),
		Answered:     nonNegative(globalTotal.Answered - you.Answered),
	}
}

func nonNegative(n int64) int64 {
	if n < 0 {
		return 0
	}
	return n
}
