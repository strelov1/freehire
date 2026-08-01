// Package mailclassify holds the controlled status vocabulary an inbox email is
// classified into and the pure rules that map a classification onto the
// application-stage pipeline. The LLM adapter lives beside it in classifier.go;
// this file is the contract + sanitizer, the "never persist an out-of-vocabulary
// value" guard (and the prompt-injection guard for untrusted email bodies).
package mailclassify

import "github.com/strelov1/freehire/internal/userjob"

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

// SignalValues is the ordered, canonical vocabulary of classification labels. It is
// the one definition: the request schema constrains the model to it, and validSignals
// — the check that still runs on receipt — is built from it, so a label added here
// cannot reach one and miss the other.
var SignalValues = []string{
	string(SignalAcknowledgement),
	string(SignalScreening),
	string(SignalInterviewInvitation),
	string(SignalAssessment),
	string(SignalOffer),
	string(SignalRejection),
	string(SignalInfoRequest),
	string(SignalIncompleteApplication),
	string(SignalOther),
}

var validSignals = func() map[StatusSignal]bool {
	valid := make(map[StatusSignal]bool, len(SignalValues))
	for _, s := range SignalValues {
		valid[StatusSignal(s)] = true
	}

	return valid
}()

// IsValidSignal reports whether s is a known classification label — used to
// validate a caller-supplied inbox label filter before it reaches the query.
func IsValidSignal(s string) bool {
	return validSignals[StatusSignal(s)]
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

// AdvanceStage returns the stage a signal should move `current` to and whether an automatic
// change should occur.
//
// The two halves belong to different packages and this is the seam. Which stage a SIGNAL implies
// is mail's own question — it is about what an employer's email means. Whether an application may
// MOVE from one stage to another is a tracking rule, and internal/userjob owns it along with the
// vocabulary itself, so a stage added there cannot leave this package computing an order that no
// longer matches.
func AdvanceStage(current string, sig StatusSignal) (string, bool) {
	target, ok := signalStage[sig]
	if !ok {
		return "", false
	}
	if !userjob.Forward(current, target) {
		return "", false
	}
	return target, true
}
