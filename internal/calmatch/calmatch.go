// Package calmatch decides which application a calendar meeting belongs to.
//
// Exactly one signal is trusted, and it is the only one implemented: the meeting carries
// the identifier of an invitation the mail matcher already tied to an application, so the
// chain closes with nothing inferred.
//
// There is deliberately no weaker tier. A first version matched an employer's name in the
// event title and offered the result as a suggestion, and three things were wrong with it
// at once. It compared the title against `applications.company_slug`, which is
// hyphenated — no calendar entry says "acme-inc" — so it never fired for a multi-word
// employer. For a single-token one it was an unanchored substring match, which reads
// "Q3 ramp-up planning" as an interview with Ramp. And nothing could confirm or dismiss
// what it produced, so a wrong guess was permanent. A tier that rarely fires, misfires
// when it does, and cannot be corrected is worse than no tier: a meeting attached to the
// wrong employer sends someone to prepare for the wrong interview.
//
// Pure: no database access and no I/O, so the rule is testable on its own and the worker
// can be read without it.
package calmatch

// Tier records which signal resolved a meeting, or that none did.
type Tier int

const (
	// TierNone means nothing recognised the meeting. It is not stored at all.
	TierNone Tier = iota
	// TierUID means the meeting carries the identifier of an invitation already linked
	// to an application. Nothing is inferred, so this is the only tier that links.
	TierUID
)

// Tiers is the canonical vocabulary, so a test can walk it and a tier cannot be added
// without a verdict on whether it may link.
var Tiers = []Tier{TierNone, TierUID}

// Links reports whether this tier may attach a meeting without asking the candidate.
//
// A method rather than a convention because the difference is the whole design, and
// because a weaker tier will be proposed again — when it is, it arrives here and answers
// this question before it can do any harm. Unknown tiers do not link.
func (t Tier) Links() bool { return t == TierUID }

// Event is the projection of a calendar entry the matcher reads. It carries no attendee
// list, no description and no title: those are the parts of a calendar that belong to the
// candidate's private life, and the one rule here needs none of them.
type Event struct {
	// UID is the meeting's own identifier, shared with the invitation that announced it.
	UID string
}

// Candidate is one of the caller's applications a meeting might belong to, carrying the
// identifiers of the invitations already linked to it.
type Candidate struct {
	ApplicationID int64
	UIDs          []string
}

// Match is the resolution: the application (0 when unresolved) and the tier that got
// there. Ask the tier whether it may be acted on.
type Match struct {
	ApplicationID int64
	Tier          Tier
}

// Resolve attaches a meeting to an application by the identifier its invitation carried,
// or to nothing at all — which is what keeps a meeting the candidate never told us about
// out of the database entirely.
func Resolve(event Event, candidates []Candidate) Match {
	if event.UID == "" {
		return Match{Tier: TierNone}
	}
	for _, c := range candidates {
		for _, uid := range c.UIDs {
			if uid != "" && uid == event.UID {
				return Match{ApplicationID: c.ApplicationID, Tier: TierUID}
			}
		}
	}
	return Match{Tier: TierNone}
}
