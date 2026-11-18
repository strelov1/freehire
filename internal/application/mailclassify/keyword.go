package mailclassify

import (
	"regexp"
	"strings"
)

// KeywordConfidence is the confidence a keyword hit reports, high enough to clear
// the stage-advance threshold.
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

// rejectionPhrases is a named list because KeywordStatus reads it twice: in list
// order, and to settle a contradiction.
//
// "unfortunately" alone is excluded: it appears in some acknowledgements, so those defer to the LLM.
//
// The last two entries are rejections that QUOTE offer vocabulary ("unable to extend
// an offer", "decided to extend an offer to another candidate"); they are listed so the
// contradiction rule can see them.
var rejectionPhrases = []string{
	"we regret", "regret to inform", "not to proceed", "not moving forward", "not be moving forward",
	"won't be moving forward", "won’t be moving forward", "decided not to move", "decided not to proceed",
	"other candidates", "not be progressing", "will not be progressing", "not selected",
	"move forward with other", "not the right fit", "not a fit for", "decided to move forward with",
	"unable to extend an offer", "another candidate",
}

// decisiveRejectionPhrases is the subset that may VETO a positive signal. A weak
// member of the list above is fine as a late match, but as a veto it is wrong: the same
// words appear in good news ("decided to move forward with you", "speaking with other
// candidates this week", "after another candidate withdrew"). SuggestStage reads the
// implied stage whether or not it advances, so a wrong veto would offer a move to
// `rejected` on the day someone was hired.
//
// What remains says the sender is closing the application in words with no benign
// reading, plus the two that quote offer vocabulary while refusing.
var decisiveRejectionPhrases = []string{
	"we regret", "regret to inform", "not to proceed", "not moving forward", "not be moving forward",
	"won't be moving forward", "won’t be moving forward", "decided not to move", "decided not to proceed",
	"not be progressing", "will not be progressing", "not selected", "move forward with other",
	"unable to extend an offer", "offer to another candidate",
}

// keywordRules is precision-first: only strong, unambiguous phrases. Rejection is
// checked before acknowledgement so a "thank you for applying … unfortunately …"
// email resolves to rejection, and ambiguous openers match nothing and defer to the LLM.
//
// Order does not decide a rejection against a POSITIVE signal; KeywordStatus does,
// since no order reads a rejection worded in offer vocabulary correctly.
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
// A text that also holds a decisive rejection phrase never comes back as a signal that
// ADVANCES the application: a rejection is written in the vocabulary of what it declines
// ("we cannot extend an offer", "thank you for completing the coding challenge —
// unfortunately we are not moving forward"), and the fast path's 0.95 confidence clears
// the 0.8 stage-advance bar.
//
// The rejection is returned rather than deferring to the model because it is the safe
// direction (StageFor gives it Advances:false). Which signals advance is read from
// signalStage, not restated here.
//
// This reads the FULL body while the model path is capped at maxBodyRunes: a rejection
// often sits at the END of a long quoted thread. The resulting risk, a quoted old
// rejection outranking a fresh invitation, is bounded by decisiveRejectionPhrases.
func KeywordStatus(subject, body string) (StatusSignal, bool) {
	text := strings.ToLower(stripTags.ReplaceAllString(subject+" \n "+body, " "))
	for _, r := range keywordRules {
		if !containsAny(text, r.phrases) {
			continue
		}
		if _, advances := StageFor(r.signal); advances && containsAny(text, decisiveRejectionPhrases) {
			return SignalRejection, true
		}
		return r.signal, true
	}
	return "", false
}

// containsAny reports whether text holds any of phrases. text must already be lowercased
// and tag-stripped; the phrases are lowercase.
func containsAny(text string, phrases []string) bool {
	for _, p := range phrases {
		if strings.Contains(text, p) {
			return true
		}
	}
	return false
}
