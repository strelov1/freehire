package mailclassify

import (
	"regexp"
	"strings"
)

// KeywordConfidence is the confidence a deterministic keyword hit reports. It is
// high enough to clear the stage-advance threshold, since a strong phrase is a
// firmer signal than a probabilistic LLM read.
const KeywordConfidence = 0.95

// stripTags drops HTML tags so keyword matching works on the readable text of an
// HTML-only email as well as a plain-text one.
var stripTags = regexp.MustCompile(`<[^>]+>`)

// keywordRule is an ordered (signal, phrases) rule; the first rule with any
// matching phrase wins, so stronger/negative signals are listed before the
// acknowledgement templates they would otherwise be confused with.
type keywordRule struct {
	signal  StatusSignal
	phrases []string
}

// rejectionPhrases is named separately from the rule that carries it because
// KeywordStatus reads it twice: once in list order, and once more to settle a
// contradiction — see the rule there.
//
// "unfortunately" alone is intentionally excluded: too weak to skip the LLM on
// (it appears in some acknowledgements) — those emails defer to the LLM instead.
//
// The last two entries are the standard rejections that QUOTE offer vocabulary
// ("we are unable to extend an offer at this time", "we have decided to extend an
// offer to another candidate"). Both were measured against KeywordStatus resolving
// to `offer`, because the phrase "extend an offer" is really in them; they are
// listed here so the contradiction rule below has something to see.
var rejectionPhrases = []string{
	"we regret", "regret to inform", "not to proceed", "not moving forward", "not be moving forward",
	"won't be moving forward", "won’t be moving forward", "decided not to move", "decided not to proceed",
	"other candidates", "not be progressing", "will not be progressing", "not selected",
	"move forward with other", "not the right fit", "not a fit for", "decided to move forward with",
	"unable to extend an offer", "another candidate",
}

// keywordRules is precision-first: only strong, unambiguous phrases. Rejection is
// checked before acknowledgement so a "thank you for applying … unfortunately …"
// email resolves to rejection, and ambiguous openers ("thank you for your
// interest" alone) match nothing and defer to the LLM.
//
// Order alone no longer decides a rejection against a POSITIVE signal — KeywordStatus
// settles that explicitly, because there is no order of this list that reads a
// rejection worded in offer vocabulary correctly.
var keywordRules = []keywordRule{
	{SignalOffer, []string{"pleased to offer you", "we are pleased to offer", "job offer from", "offer of employment", "extend an offer"}},
	{SignalAssessment, []string{"coding challenge", "take-home", "take home assignment", "technical assessment", "hackerrank", "codility", "online assessment", "coding test"}},
	{SignalInterviewInvitation, []string{"invite you to interview", "invite you to an interview", "invitation to interview", "interview invitation", "like to invite you", "schedule a call", "schedule an interview", "set up a call", "set up an interview", "you're invited", "you’re invited", "invited to a call", "book a time"}},
	{SignalRejection, rejectionPhrases},
	// Ordered before acknowledgement: an "…thank you for starting your application,
	// please complete it…" email is an incomplete-application to-do, not an ack.
	{SignalIncompleteApplication, []string{"application is incomplete", "incomplete application", "complete your application", "finish your application", "action required to complete", "to complete your application", "your application is not complete", "did not complete your application"}},
	{SignalAcknowledgement, []string{"thank you for applying", "thank you for your application", "thanks for applying", "we have received your application", "we've received your", "we’ve received your", "received your application", "received your resume", "application submitted", "your application has been received"}},
}

// KeywordStatus deterministically classifies an email's status from its subject
// and body, returning (signal, true) only on a strong, unambiguous phrase and
// ("", false) otherwise — deferring the ambiguous tail (soft/multilingual
// rejections, bare interest openers) to the LLM. Precision over recall by design.
//
// Contradictory evidence is settled, not raced down the list: a text that also holds a
// rejection phrase can never come back as a signal that ADVANCES the application. A
// rejection is written in the vocabulary of the thing it declines ("we regret to inform
// you we cannot extend an offer", "thank you for completing the coding challenge —
// unfortunately we are not moving forward"), so first-match-wins over an ordered list
// answered `offer` and `assessment` for those, and Classify's keyword fast path reports
// 0.95 confidence, over the 0.8 the stage advance needs — a rejection moved the
// application to `offer`.
//
// The rejection is returned rather than deferring to the model: it is the safe direction
// (StageFor gives it Advances:false, so nothing moves on it), it is the right answer for
// every phrasing measured here, and it keeps the deterministic path deterministic. Which
// signals "advance" is not restated here — it is read from signalStage, so a signal added
// there is covered without a second list to keep in step.
//
// This reads the FULL body while the model path is bounded to maxBodyRunes, and that
// asymmetry is deliberate now that it matters: a rejection line sits at the END of a long
// quoted thread more often than at the top, and truncating to match the prompt would blind
// this check to exactly the evidence it exists to find. The error it can produce instead —
// a quoted old rejection outranking a fresh invitation — costs a label and never a stage.
func KeywordStatus(subject, body string) (StatusSignal, bool) {
	text := strings.ToLower(stripTags.ReplaceAllString(subject+" \n "+body, " "))
	for _, r := range keywordRules {
		if !containsAny(text, r.phrases) {
			continue
		}
		if _, advances := StageFor(r.signal); advances && containsAny(text, rejectionPhrases) {
			return SignalRejection, true
		}
		return r.signal, true
	}
	return "", false
}

// containsAny reports whether text holds any of phrases. text is already lowercased and
// tag-stripped by the caller; the phrases are lowercase by construction.
func containsAny(text string, phrases []string) bool {
	for _, p := range phrases {
		if strings.Contains(text, p) {
			return true
		}
	}
	return false
}
