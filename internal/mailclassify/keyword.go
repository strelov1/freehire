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

// keywordRules is precision-first: only strong, unambiguous phrases. Rejection is
// checked before acknowledgement so a "thank you for applying … unfortunately …"
// email resolves to rejection, and ambiguous openers ("thank you for your
// interest" alone) match nothing and defer to the LLM.
var keywordRules = []keywordRule{
	{SignalOffer, []string{"pleased to offer you", "we are pleased to offer", "job offer from", "offer of employment", "extend an offer"}},
	{SignalAssessment, []string{"coding challenge", "take-home", "take home assignment", "technical assessment", "hackerrank", "codility", "online assessment", "coding test"}},
	{SignalInterviewInvitation, []string{"invite you to interview", "invite you to an interview", "invitation to interview", "interview invitation", "like to invite you", "schedule a call", "schedule an interview", "set up a call", "set up an interview", "you're invited", "you’re invited", "invited to a call", "book a time"}},
	// "unfortunately" alone is intentionally excluded: too weak to skip the LLM on
	// (it appears in some acknowledgements) — those emails defer to the LLM instead.
	{SignalRejection, []string{"we regret", "regret to inform", "not to proceed", "not moving forward", "not be moving forward", "won't be moving forward", "won’t be moving forward", "decided not to move", "decided not to proceed", "other candidates", "not be progressing", "will not be progressing", "not selected", "move forward with other", "not the right fit", "not a fit for", "decided to move forward with"}},
	// Ordered before acknowledgement: an "…thank you for starting your application,
	// please complete it…" email is an incomplete-application to-do, not an ack.
	{SignalIncompleteApplication, []string{"application is incomplete", "incomplete application", "complete your application", "finish your application", "action required to complete", "to complete your application", "your application is not complete", "did not complete your application"}},
	{SignalAcknowledgement, []string{"thank you for applying", "thank you for your application", "thanks for applying", "we have received your application", "we've received your", "we’ve received your", "received your application", "received your resume", "application submitted", "your application has been received"}},
}

// KeywordStatus deterministically classifies an email's status from its subject
// and body, returning (signal, true) only on a strong, unambiguous phrase and
// ("", false) otherwise — deferring the ambiguous tail (soft/multilingual
// rejections, bare interest openers) to the LLM. Precision over recall by design.
func KeywordStatus(subject, body string) (StatusSignal, bool) {
	text := strings.ToLower(stripTags.ReplaceAllString(subject+" \n "+body, " "))
	for _, r := range keywordRules {
		for _, p := range r.phrases {
			if strings.Contains(text, p) {
				return r.signal, true
			}
		}
	}
	return "", false
}
