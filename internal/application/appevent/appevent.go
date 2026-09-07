// Package appevent holds the application-event vocabulary shared by the paths that
// record into the ledger — internal/application/inbox (which owns the mail reconcile both the
// interactive paths and cmd/classify-mail call) and internal/application/jobtracking — and by the
// aggregates that read it.
//
// It is vocabulary and rules only, in the shape internal/application/userjob uses for stages: no
// database access, so every caller can validate before it writes and the rules are
// testable without a pool.
package appevent

import "fmt"

// The event kinds. This vocabulary is ours and narrow, and is meant to stay still: it
// says WHAT happened, while Signal says what was in it. Signal draws on the mailclassify
// status vocabulary, which has grown before and will again — folding the two together
// would make every classifier addition a change to an append-only table.
const (
	KindApplied       = "applied"
	KindEmployerReply = "employer_reply"
	KindFollowUpSent  = "follow_up_sent"
	KindStageSet      = "stage_set"
	// KindInterviewScheduled records that a meeting was arranged, dated by when we
	// observed the arrangement rather than by when the meeting is. The meeting's own
	// time lives in application_interviews: it moves and is called off, and occurred_at
	// here means "when this happened", which a future date would make untrue.
	KindInterviewScheduled = "interview_scheduled"
)

// Kinds is the canonical, ordered kind vocabulary.
var Kinds = []string{KindApplied, KindEmployerReply, KindFollowUpSent, KindStageSet, KindInterviewScheduled}

// The event sources, mirroring the three mail stores in internal/application/inbox plus the two ways
// a candidate records something themselves.
const (
	SourceMailGmail    = "mail_gmail"
	SourceMailHosted   = "mail_hosted"
	SourceMailExternal = "mail_external"
	SourceUser         = "user"
	SourceAssistant    = "assistant"
	// SourceCalendarGoogle is a fact read out of the candidate's own calendar. Trusted
	// for day math like the mail sources and for the same reason: the date was set by an
	// organiser and observed by us, not typed by the candidate when they got round to it.
	SourceCalendarGoogle = "calendar_google"
	// SourceSystem is a stage change the platform made on the candidate's behalf, not one
	// anybody typed or that mail/calendar evidence dated — e.g. internal/engage/nudge auto-expiring
	// an application whose listing closed. Not trusted for day math: it records when we
	// noticed, not an employer-side timing fact.
	SourceSystem = "system"
)

// Sources is the canonical, ordered source vocabulary.
var Sources = []string{SourceMailGmail, SourceMailHosted, SourceMailExternal, SourceUser, SourceAssistant, SourceCalendarGoogle, SourceSystem}

// MailSources is the mail three on their own — every source SourceForMail can return.
//
// It exists because a reader has to be able to ask "was this event read out of a message,
// or set by somebody". LastStageSetAt does: it means "when the stage was last set other
// than by mail", and since the mail path records its own auto-advances that is a predicate
// rather than a description. Spelling the three into a query would put the vocabulary in
// two places, which is the failure the pin test next door already guards against.
var MailSources = []string{SourceMailGmail, SourceMailHosted, SourceMailExternal}

// TrustedForDayMath reports whether events from this source may enter timing
// calculations.
//
// Only the mail and calendar sources carry a timestamp somebody other than the candidate
// set. A manually-recorded stage dates from when the candidate got around to updating
// their board, so a funnel built on it would measure diligence and report it as market
// behaviour. Unknown sources are untrusted: unknown provenance must never read as an
// observation.
func TrustedForDayMath(source string) bool {
	switch source {
	case SourceMailGmail, SourceMailHosted, SourceMailExternal, SourceCalendarGoogle:
		return true
	default:
		return false
	}
}

// SourceForMail maps an emails.source value (see inbox.Sources) to the event source.
//
// It errors rather than defaulting, because every default here is a trusted one: an
// unrecognised store silently mapped to a mail source would be admitted to timings on the
// strength of a typo.
func SourceForMail(mailSource string) (string, error) {
	switch mailSource {
	case "gmail":
		return SourceMailGmail, nil
	case "hosted":
		return SourceMailHosted, nil
	case "external":
		return SourceMailExternal, nil
	default:
		return "", fmt.Errorf("appevent: unknown mail source %q", mailSource)
	}
}
