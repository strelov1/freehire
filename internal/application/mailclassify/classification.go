// Package mailclassify holds the controlled status vocabulary an inbox email is
// classified into and the pure rules that map a classification onto the
// application-stage pipeline. The LLM adapter is in classifier.go; this file is the
// contract and the sanitizer that never lets an out-of-vocabulary value persist.
package mailclassify

import "github.com/strelov1/freehire/internal/application/userjob"

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

// SignalValues is the ordered, canonical vocabulary of classification labels. The request
// schema and validSignals are both built from it, so they cannot diverge.
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
// confidence in [0,1], and (only when deterministic matching was ambiguous or
// empty) the disambiguation pick, 0 meaning "none".
type Classification struct {
	Signal       StatusSignal `json:"signal"`
	Confidence   float64      `json:"confidence"`
	MatchedJobID int64        `json:"matched_job_id"`
	// MatchConfidence is the confidence in MatchedJobID, kept apart from Confidence so a
	// suggested link does not inherit the status confidence. The LLM answers one number
	// for both and copies it; Jev answers each.
	MatchConfidence float64 `json:"-"`
	// Model names the classifier that answered when it is not the default LLM, persisted as
	// emails.classification_model. Never decoded from model JSON: an email cannot choose it.
	Model string `json:"-"`
}

// Sanitize coerces the classification into the controlled vocabulary before it
// is persisted or served: an unknown signal becomes `other`, and both confidences are
// clamped to [0,1]. The caller validates the matched id against the real candidates.
func (c Classification) Sanitize() Classification {
	if !validSignals[c.Signal] {
		c.Signal = SignalOther
	}
	c.Confidence = clampUnit(c.Confidence)
	c.MatchConfidence = clampUnit(c.MatchConfidence)
	return c
}

func clampUnit(v float64) float64 {
	switch {
	case v < 0:
		return 0
	case v > 1:
		return 1
	}
	return v
}

// StageImplication is what a status signal says about the application: the stage the message
// implies, and whether that implication may be acted on without asking.
//
// They are separate because a rejection implies `rejected` yet must never move the
// application by itself.
type StageImplication struct {
	Stage    string
	Advances bool
}

// signalStage maps a status signal to the stage it implies.
//
// Non-progress signals are absent on purpose: `info_request` and `incomplete_application` are
// to-dos, and `other` means the classifier could not tell. None implies a stage.
var signalStage = map[StatusSignal]StageImplication{
	SignalAcknowledgement:     {Stage: "applied", Advances: true},
	SignalScreening:           {Stage: "screening", Advances: true},
	SignalAssessment:          {Stage: "screening", Advances: true},
	SignalInterviewInvitation: {Stage: "interview", Advances: true},
	SignalOffer:               {Stage: "offer", Advances: true},
	SignalRejection:           {Stage: "rejected", Advances: false},
}

// StageFor reports the stage a signal implies and whether it may be applied automatically.
// An empty stage means the signal implies none. Exported for readers that show what an
// email implies or offer a suggestion when the two disagree.
func StageFor(sig StatusSignal) (stage string, advances bool) {
	imp, ok := signalStage[sig]
	if !ok {
		return "", false
	}
	return imp.Stage, imp.Advances
}

// AdvanceStage returns the stage a signal should move `current` to and whether an automatic
// change should occur.
//
// This is the seam: which stage a SIGNAL implies is mail's question; whether an application
// may MOVE between stages is a tracking rule owned by internal/application/userjob with the
// stage vocabulary.
func AdvanceStage(current string, sig StatusSignal) (string, bool) {
	target, advances := StageFor(sig)
	if !advances {
		return "", false
	}
	if !userjob.Forward(current, target) {
		return "", false
	}
	return target, true
}
