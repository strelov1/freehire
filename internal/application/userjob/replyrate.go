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

// ReplyRateSampleGate is the minimum observable-application count either side of the
// comparison must clear before it is served — the same sample floor
// company-hiring-signal already applies to a single named company's response rate.
// Below it, absence is the honest answer, not a zero or an estimate.
const ReplyRateSampleGate = 10

// GateReplyRateBenchmark returns the comparison when both sides clear
// ReplyRateSampleGate, or nil when either does not — including when the caller has no
// connected mailbox, which surfaces here as zero observable applications rather than a
// separate check.
func GateReplyRateBenchmark(you, global ReplyRateSide) *ReplyRateBenchmark {
	if you.Applications < ReplyRateSampleGate || global.Applications < ReplyRateSampleGate {
		return nil
	}
	return &ReplyRateBenchmark{You: you, Global: global}
}
