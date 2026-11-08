// Package silence holds the ladder of how long an application may go unanswered at each
// stage, and the arithmetic under it. It lives apart from internal/application/userjob because the
// ghost signal judges a POSTING by how long the applications against it have been silent,
// and a posting is not an application — so the ladder has to sit below both.
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
// The values do not share a provenance, and a table of five specific numbers
// reads as measurement whether or not it is one, so each carries its own:
//
//	applied   21  measured — 92 observed applications, marks 16% of them
//	screening 18  interpolated between the measured anchors
//	responded 15  interpolated between the measured anchors
//	interview 12  measured — 6 observed applications, marks 3. Raised from 7,
//	              which marked 5 of the 6: a badge on nearly every card is one
//	              nobody reads.
//	offer      5  judgement, from a job seeker's experience. No application in
//	              the sample has reached this stage; the single message ever
//	              classified an offer is genuine but from a job search three
//	              years earlier and informs nothing.
//	preparing 21  rarely read: cv-tailoring places a card here with no AppliedAt,
//	              so TrackedJob.Silence() short-circuits; but a card moved back to
//	              `preparing` after applying keeps AppliedAt (TrackJob never clears
//	              it) and is judged by this value. Copies `applied`'s value as the
//	              nearest meaningful anchor rather than inventing a new one.
//
// The interpolated pair steps evenly by three days between 21 and 12 rather than
// taking distinct-looking values, so the shape of the ladder shows at a glance
// which rungs were derived rather than observed. Revisit the middle three once
// the sample grows.
//
// All values are calendar days, which sets a floor under the strictest: five is
// the shortest span that always contains two working days, while three can be
// consumed entirely by a weekend. Going below five needs business-day
// arithmetic — its own calendar, holidays and employer time zone.
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
// Part-days do not count and a negative is impossible: clock skew, or a last-activity stamp a
// moment in the future, must not report negative silence. It lives here beside the ladder it
// feeds because three surfaces — the tracking board, the follow-up gate and the ghost signal —
// each had their own copy, held together by comments naming one another. The ladder was already
// shared; the arithmetic under it was not, and a day's disagreement between the badge and the
// offer is exactly what the shared ladder exists to prevent.
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
