// Package calmatch decides which application a calendar meeting belongs to.
//
// It is the calendar's counterpart to internal/mailmatch and holds the same asymmetry
// for the same reason: exactly one signal is trusted to link, and everything weaker is
// offered to the candidate as a suggestion. A wrong link here is worse than a missing
// one — a meeting attached to the wrong employer sends someone to prepare for the wrong
// interview, and the mail stack already paid for learning that lesson once.
//
// Pure: no database access and no I/O, so the rules are testable on their own and the
// worker can be read without them.
package calmatch

import "strings"

// Tier records which signal resolved a meeting, or which one failed to.
type Tier int

const (
	// TierNone means nothing recognised the meeting. It is not stored at all.
	TierNone Tier = iota
	// TierUID means the meeting carries the identifier of an invitation already linked
	// to an application. Nothing is inferred, so this is the only tier that links.
	TierUID
	// TierName means an employer's name in the title matched exactly one application —
	// a suggestion, not a link.
	TierName
	// TierAmbiguous means the name matched more than one application. Two applications
	// to one employer are two rounds, and the title does not say which.
	TierAmbiguous
)

// Tiers is the canonical vocabulary, so a test can walk it and a new tier cannot be
// added without a verdict on whether it may link.
var Tiers = []Tier{TierNone, TierUID, TierName, TierAmbiguous}

// Links reports whether this tier may attach a meeting without asking the candidate.
//
// A method rather than a convention because the difference is the whole design: a caller
// that treated a name match as a link would attach a meeting on the strength of a word in
// a title. Unknown tiers do not link — an unrecognised signal must never read as proof.
func (t Tier) Links() bool { return t == TierUID }

// Event is the projection of a calendar entry the matcher reads. It carries no attendee
// list, no description and no organiser name: those are the parts of a calendar that
// belong to the candidate's private life, and nothing here needs them.
type Event struct {
	// UID is the meeting's own identifier, shared with the invitation that announced it.
	UID string
	// Title is the event's summary, read only for the suggestion tier.
	Title string
	// Organizer is the address that scheduled it. Deliberately read by nothing: it is
	// evidence about who sent the invitation, not about who is interviewing — an ATS
	// schedules from its own domain as readily as a recruiter schedules from theirs.
	// Kept on the struct so a reader can see the decision rather than guess at it.
	Organizer string
}

// Candidate is one of the caller's applications a meeting might belong to, carrying the
// employer's display name and the identifiers of the invitations already linked to it.
type Candidate struct {
	ApplicationID int64
	Company       string
	UIDs          []string
}

// Match is the resolution: the application (0 when unresolved) and the tier that got
// there. Ask the tier whether it may be acted on.
type Match struct {
	ApplicationID int64
	Tier          Tier
}

// Resolve runs the cascade: the invitation's identifier first, then a unique employer
// name in the title. Anything else resolves to nothing, which is what keeps a meeting
// the candidate never told us about out of the database entirely.
func Resolve(event Event, candidates []Candidate) Match {
	if event.UID != "" {
		for _, c := range candidates {
			for _, uid := range c.UIDs {
				if uid != "" && uid == event.UID {
					return Match{ApplicationID: c.ApplicationID, Tier: TierUID}
				}
			}
		}
	}

	title := strings.ToLower(event.Title)
	if title == "" {
		return Match{Tier: TierNone}
	}

	var matched int64
	var matches int
	for _, c := range candidates {
		name := strings.ToLower(strings.TrimSpace(c.Company))
		if name == "" || !strings.Contains(title, name) {
			continue
		}
		matches++
		matched = c.ApplicationID
	}

	switch matches {
	case 0:
		return Match{Tier: TierNone}
	case 1:
		return Match{ApplicationID: matched, Tier: TierName}
	default:
		return Match{Tier: TierAmbiguous}
	}
}
