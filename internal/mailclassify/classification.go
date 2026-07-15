// Package mailclassify holds the controlled status vocabulary an inbox email is
// classified into and the pure rules that map a classification onto the
// application-stage pipeline. The LLM adapter lives beside it in classifier.go;
// this file is the contract + sanitizer, the "never persist an out-of-vocabulary
// value" guard (and the prompt-injection guard for untrusted email bodies).
package mailclassify

// StatusSignal is the controlled vocabulary an email is classified into.
type StatusSignal string

const (
	SignalAcknowledgement     StatusSignal = "acknowledgement"
	SignalScreening           StatusSignal = "screening"
	SignalInterviewInvitation StatusSignal = "interview_invitation"
	SignalAssessment          StatusSignal = "assessment"
	SignalOffer               StatusSignal = "offer"
	SignalRejection           StatusSignal = "rejection"
	SignalInfoRequest         StatusSignal = "info_request"
	// SignalIncompleteApplication is an actionable to-do: the application was
	// started but not finished. Intentionally absent from signalStage — an
	// unfinished application has not progressed.
	SignalIncompleteApplication StatusSignal = "incomplete_application"
	SignalOther                 StatusSignal = "other"
)

var validSignals = map[StatusSignal]bool{
	SignalAcknowledgement: true, SignalScreening: true,
	SignalInterviewInvitation: true, SignalAssessment: true,
	SignalOffer: true, SignalRejection: true,
	SignalInfoRequest: true, SignalIncompleteApplication: true, SignalOther: true,
}

// Classification is the LLM output for one email: the status signal, a
// confidence in [0,1], and (only used when deterministic matching was
// ambiguous/none) the disambiguation pick — 0 meaning "none".
type Classification struct {
	Signal       StatusSignal `json:"signal"`
	Confidence   float64      `json:"confidence"`
	MatchedJobID int64        `json:"matched_job_id"`
}

// Sanitize coerces the classification into the controlled vocabulary before it
// is persisted or served: an unknown signal becomes `other`, and the confidence
// is clamped to [0,1]. The matched id is validated against the real candidate
// set by the caller, not here.
func (c Classification) Sanitize() Classification {
	if !validSignals[c.Signal] {
		c.Signal = SignalOther
	}
	switch {
	case c.Confidence < 0:
		c.Confidence = 0
	case c.Confidence > 1:
		c.Confidence = 1
	}
	return c
}

// stageOrder is the forward pipeline rank of the active application stages a
// classified email can advance to. Terminal outcomes (rejected/withdrawn) and
// accepted are intentionally absent — they are never applied automatically.
var stageOrder = map[string]int{
	"applied": 1, "screening": 2, "responded": 3, "interview": 4, "offer": 5,
}

// terminalStages are the settled outcomes an automatic advance must never move a
// job OUT of. They rank below `applied` in stageOrder (rank 0), so without this
// guard any forward signal would "advance" a rejected/accepted/withdrawn job back
// into the active pipeline — resurrecting a dead application.
var terminalStages = map[string]bool{
	"rejected": true, "accepted": true, "withdrawn": true,
}

// signalStage maps a status signal to the application stage it implies and
// whether that stage may be applied automatically. Negative/terminal outcomes
// (rejection) and non-progress signals (info_request, other) are never auto.
var signalStage = map[StatusSignal]string{
	SignalAcknowledgement:     "applied",
	SignalScreening:           "screening",
	SignalAssessment:          "screening",
	SignalInterviewInvitation: "interview",
	SignalOffer:               "offer",
}

// AdvanceStage returns the stage a signal should move `current` to and whether
// an automatic change should occur. It advances only strictly forward in the
// pipeline; a backward, terminal, or non-progress signal returns ("", false).
// An empty `current` ranks below every stage, so a first classified email can
// seed the stage.
func AdvanceStage(current string, sig StatusSignal) (string, bool) {
	if terminalStages[current] {
		return "", false // a settled application is never resurrected automatically
	}
	target, ok := signalStage[sig]
	if !ok {
		return "", false
	}
	if stageOrder[target] > stageOrder[current] {
		return target, true
	}
	return "", false
}
