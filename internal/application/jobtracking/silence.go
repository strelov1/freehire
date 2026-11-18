package jobtracking

import (
	"time"

	"github.com/strelov1/freehire/internal/job/silence"
)

// Silence is how long an application has been waiting and what that means at the
// stage it is in. It is derived on read against the caller's clock, never stored
// or cached, since it changes as time passes.
type Silence struct {
	// LastActivityAt is the application's apply date, or the newest message
	// linked to it when that is later.
	LastActivityAt time.Time
	// DaysSilent is the whole days elapsed since. A part-day does not count.
	DaysSilent int
	// State is one of silence.Active / silence.Silent / silence.Unconfirmed.
	State string
}

// Silence derives the waiting state of a tracked row as of now, or nil when the
// row has nothing to report: a job merely viewed or saved, or an application in a
// settled stage. Nil rather than a zero-day `active` keeps "nothing owed"
// distinct from "owed and fine", which the board's marker depends on.
//
// A missing LastActivityAt falls back to the apply date; the query already does
// this, and it is repeated here so the domain keeps the guarantee.
func (j TrackedJob) Silence(now time.Time) *Silence {
	if j.AppliedAt == nil {
		return nil
	}
	stage := ""
	if j.Stage != nil {
		stage = *j.Stage
	}
	if _, accrues := silence.ThresholdDays(stage); !accrues {
		return nil
	}

	last := *j.AppliedAt
	if j.LastActivityAt != nil && j.LastActivityAt.After(last) {
		last = *j.LastActivityAt
	}
	days := silence.Days(now, last)
	return &Silence{
		LastActivityAt: last,
		DaysSilent:     days,
		State:          silence.StateFor(stage, days, j.HasPendingSuggestion),
	}
}
