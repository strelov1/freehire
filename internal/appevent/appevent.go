// Package appevent holds the application-event vocabulary shared by the paths that
// record into the ledger — internal/inbox (which owns the mail reconcile both the
// interactive paths and cmd/classify-mail call) and internal/jobtracking — and by the
// aggregates that read it.
//
// It is vocabulary and rules only, in the shape internal/userjob uses for stages: no
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
)

// Kinds is the canonical, ordered kind vocabulary.
var Kinds = []string{KindApplied, KindEmployerReply, KindFollowUpSent, KindStageSet}

// ValidKind reports whether k is a known event kind.
func ValidKind(k string) bool {
	for _, kind := range Kinds {
		if kind == k {
			return true
		}
	}
	return false
}

// The event sources, mirroring the three mail stores in internal/inbox plus the two ways
// a candidate records something themselves.
const (
	SourceMailGmail    = "mail_gmail"
	SourceMailHosted   = "mail_hosted"
	SourceMailExternal = "mail_external"
	SourceUser         = "user"
	SourceAssistant    = "assistant"
)

// Sources is the canonical, ordered source vocabulary.
var Sources = []string{SourceMailGmail, SourceMailHosted, SourceMailExternal, SourceUser, SourceAssistant}

// ValidSource reports whether s is a known event source.
func ValidSource(s string) bool {
	for _, src := range Sources {
		if src == s {
			return true
		}
	}
	return false
}

// TrustedForDayMath reports whether events from this source may enter timing
// calculations.
//
// Only the mail sources carry a timestamp the employer set. A manually-recorded stage
// dates from when the candidate got around to updating their board, so a funnel built on
// it would measure diligence and report it as market behaviour. Unknown sources are
// untrusted: unknown provenance must never read as an observation.
func TrustedForDayMath(source string) bool {
	switch source {
	case SourceMailGmail, SourceMailHosted, SourceMailExternal:
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
