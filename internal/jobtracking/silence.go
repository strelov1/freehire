package jobtracking

import (
	"time"

	"github.com/strelov1/freehire/internal/userjob"
)

// Silence is how long an application has been waiting and what that means at the
// stage it is in. It is derived on read against the caller's clock, never
// stored: the value changes as time passes with nothing about the application
// changing, so it must never be cached apart from the instant it was taken.
type Silence struct {
	// LastActivityAt is the application's apply date, or the newest message
	// linked to it when that is later.
	LastActivityAt time.Time
	// DaysSilent is the whole days elapsed since. A part-day does not count.
	DaysSilent int
	// State is one of userjob.SilenceActive / SilenceSilent / SilenceUnconfirmed.
	State string
}

// Silence derives the waiting state of a tracked row as of now, or nil when the
// row has nothing to report: a job merely viewed or saved is not waiting on
// anyone, and neither is an application in a settled stage. Returning nil rather
// than a zero-day `active` keeps "nothing owed" distinguishable from "owed and
// fine" — a distinction the board's marker depends on.
//
// An application whose LastActivityAt is missing falls back to its apply date.
// The query already guarantees that fallback; repeating it here keeps the domain
// from reintroducing a hole the SQL closed.
func (j TrackedJob) Silence(now time.Time) *Silence {
	if j.AppliedAt == nil {
		return nil
	}
	stage := ""
	if j.Stage != nil {
		stage = *j.Stage
	}
	if _, accrues := userjob.SilenceThresholdDays(stage); !accrues {
		return nil
	}

	last := *j.AppliedAt
	if j.LastActivityAt != nil && j.LastActivityAt.After(last) {
		last = *j.LastActivityAt
	}
	days := int(now.Sub(last).Hours() / 24)
	if days < 0 {
		days = 0
	}
	return &Silence{
		LastActivityAt: last,
		DaysSilent:     days,
		State:          userjob.SilenceStateFor(stage, days, j.HasPendingSuggestion),
	}
}
