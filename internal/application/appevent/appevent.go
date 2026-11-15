// Package appevent holds the application-event vocabulary shared by the paths that record
// into the ledger (internal/application/inbox, internal/application/jobtracking, and
// cmd/classify-mail through the mail reconcile) and by the aggregates that read it.
//
// Vocabulary and rules only, no database access, so callers validate before they write and
// the rules test without a pool.
package appevent

import "fmt"

// The event kinds. This vocabulary is narrow and meant to stay still: it says WHAT
// happened, while Signal says what was in it. Signal follows the growing mailclassify
// vocabulary; folding the two together would make every classifier addition a change to an
// append-only table.
const (
	KindApplied       = "applied"
	KindEmployerReply = "employer_reply"
	KindFollowUpSent  = "follow_up_sent"
	KindStageSet      = "stage_set"
	// KindInterviewScheduled records that a meeting was arranged, dated by when we observed
	// the arrangement. The meeting's own time lives in
	// application_interviews: it moves and is called off, and occurred_at means "when this
	// happened", which a future date would falsify.
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
	// SourceCalendarGoogle is a fact read out of the candidate's own calendar. Trusted for day
	// math like the mail sources: the date was set by an organiser and observed by us.
	SourceCalendarGoogle = "calendar_google"
	// SourceSystem is a stage change the platform made on the candidate's behalf (e.g.
	// internal/engage/nudge auto-expiring an application whose listing closed). Not trusted for
	// day math: it records when we noticed, not an employer-side timing fact.
	SourceSystem = "system"
	// SourceAutoApply is cmd/auto-apply's own successful, unattended submission. It is distinct
	// from SourceSystem so readers can filter on it.
	SourceAutoApply = "auto_apply"
	// SourceEmployer is a stage the employer set in their own pipeline for an application
	// sent on freehire (internal/ingest/directapply), mirrored onto the candidate's record.
	// The employer's own statement about the application, so it is the one source that may
	// move a candidate into a rejection or an offer. Trusted for day math like the mail
	// sources and for the same reason: it is stamped when the employer acted, on our clock,
	// not when the candidate got round to recording it.
	SourceEmployer = "employer"
)

// Sources is the canonical, ordered source vocabulary.
var Sources = []string{
	SourceMailGmail, SourceMailHosted, SourceMailExternal, SourceUser, SourceAssistant,
	SourceCalendarGoogle, SourceSystem, SourceAutoApply, SourceEmployer,
}

// MailSources is the three mail sources on their own: every source SourceForMail can
// return. LastStageSetAt needs the set to mean "set other than by mail"; spelling the three
// into a query would put the vocabulary in two places, which the pin test guards against.
var MailSources = []string{SourceMailGmail, SourceMailHosted, SourceMailExternal}

// TrustedForDayMath reports whether events from this source may enter timing calculations.
//
// Only the mail, calendar and employer sources carry a timestamp somebody other than the
// candidate set. A manually recorded stage dates from when the candidate updated their board,
// so a funnel built on it would measure diligence. Unknown sources are untrusted.
func TrustedForDayMath(source string) bool {
	switch source {
	case SourceMailGmail, SourceMailHosted, SourceMailExternal, SourceCalendarGoogle, SourceEmployer:
		return true
	default:
		return false
	}
}

// SourceForMail maps an emails.source value (see inbox.Sources) to the event source. It
// errors rather than defaulting, because every default here is a trusted one: an
// unrecognised store mapped to a mail source would be admitted to timings on a typo.
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
