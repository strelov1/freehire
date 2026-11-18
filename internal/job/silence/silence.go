// Package silence holds how long an application may go unanswered at each stage, and the
// arithmetic under it. It sits below internal/application/userjob because the ghost signal
// judges a posting by the silence of its applications.
package silence

import (
	"maps"
	"slices"
	"time"
)

// Silence states. An application reports one of these, or the empty string when
// it is settled and so waiting on nobody — a caller must be able to tell "not
// waiting" from "waiting and fine", which a reassuring `active` would hide.
const (
	// Active is an application inside its stage's tolerated silence.
	Active = "active"
	// Silent is an application past it.
	Silent = "silent"
	// Unconfirmed is an application that would read as silent but has mail
	// awaiting the user's confirmation — a question to answer, not a verdict.
	Unconfirmed = "unconfirmed"
)

// thresholds is how many days of silence each active stage tolerates,
// growing stricter as the application advances (TestSilenceThresholdsGrowStricter
// pins that direction). Terminal stages are absent: a settled application never
// accrues silence.
//
// Provenance of the values:
//
//	applied   21  measured
//	screening 18  interpolated between the measured anchors
//	responded 15  interpolated between the measured anchors
//	interview 12  measured; 7 marked nearly every card, a badge nobody reads
//	offer      5  judgement, no observed sample reached this stage
//	preparing 21  rarely read: cv-tailoring cards have no AppliedAt so TrackedJob.Silence()
//	              short-circuits, but a card moved back to preparing keeps AppliedAt and is
//	              judged by this value. Copies applied's.
//
// The interpolated pair steps evenly by three days so the shape shows which rungs were
// derived. Revisit the middle three once the sample grows.
//
// Values are calendar days: five is the shortest span that always contains two working
// days (three can be a weekend). Going below five needs business-day arithmetic.
var thresholds = map[string]int{
	"preparing": 21,
	"applied":   21,
	"screening": 18,
	"responded": 15,
	"interview": 12,
	"offer":     5,
}

// Days is whole days between last and now, floored at zero.
//
// Part-days do not count, and clock skew or a future last-activity stamp must not report
// negative silence. Shared so the tracking board, follow-up gate and ghost signal never
// disagree by a day.
func Days(now, last time.Time) int {
	if d := int(now.Sub(last).Hours() / 24); d > 0 {
		return d
	}
	return 0
}

// ThresholdDays returns how many days of silence `stage` tolerates, and
// whether it accrues silence at all. An unset stage is judged as `applied`: an
// application with no stage recorded is still an application. Terminal and
// unknown stages report false.
func ThresholdDays(stage string) (int, bool) {
	if stage == "" {
		stage = "applied"
	}
	days, ok := thresholds[stage]
	return days, ok
}

// StateFor maps an application's stage, its elapsed silence, and whether
// any unconfirmed suggestion points at it, to a silence state — or "" when the
// stage never accrues silence.
//
// The threshold is the last tolerated day, not the first offending one. A
// pending suggestion only ever softens a silence claim into a question: mail
// awaiting confirmation on an application that is answering promptly is not a
// problem to report, so it never turns `active` into anything.
func StateFor(stage string, daysSilent int, pendingSuggestion bool) string {
	threshold, ok := ThresholdDays(stage)
	if !ok {
		return ""
	}
	if daysSilent <= threshold {
		return Active
	}
	if pendingSuggestion {
		return Unconfirmed
	}
	return Silent
}

// Stages returns the stages that accrue silence. internal/application/userjob cross-checks it against
// its own active-stage ranking: a stage that advances but tolerates no measured silence
// would never report as silent, and a threshold for a stage that does not exist is dead.
func Stages() []string { return slices.Sorted(maps.Keys(thresholds)) }
