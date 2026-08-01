package userjob

import "time"

// Silence states. An application reports one of these, or the empty string when
// it is settled and so waiting on nobody — a caller must be able to tell "not
// waiting" from "waiting and fine", which a reassuring `active` would hide.
const (
	// SilenceActive is an application inside its stage's tolerated silence.
	SilenceActive = "active"
	// SilenceSilent is an application past it.
	SilenceSilent = "silent"
	// SilenceUnconfirmed is an application that would read as silent but has mail
	// awaiting the user's confirmation — a question to answer, not a verdict.
	SilenceUnconfirmed = "unconfirmed"
)

// silenceThresholds is how many days of silence each active stage tolerates,
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
var silenceThresholds = map[string]int{
	"applied":   21,
	"screening": 18,
	"responded": 15,
	"interview": 12,
	"offer":     5,
}

// DaysSilent is whole days between last and now, floored at zero.
//
// Part-days do not count and a negative is impossible: clock skew, or a last-activity stamp a
// moment in the future, must not report negative silence. It lives here beside the ladder it
// feeds because three surfaces — the tracking board, the follow-up gate and the ghost signal —
// each had their own copy, held together by comments naming one another. The ladder was already
// shared; the arithmetic under it was not, and a day's disagreement between the badge and the
// offer is exactly what the shared ladder exists to prevent.
func DaysSilent(now, last time.Time) int {
	if d := int(now.Sub(last).Hours() / 24); d > 0 {
		return d
	}
	return 0
}

// SilenceThresholdDays returns how many days of silence `stage` tolerates, and
// whether it accrues silence at all. An unset stage is judged as `applied`: an
// application with no stage recorded is still an application. Terminal and
// unknown stages report false.
func SilenceThresholdDays(stage string) (int, bool) {
	if stage == "" {
		stage = "applied"
	}
	days, ok := silenceThresholds[stage]
	return days, ok
}

// SilenceStateFor maps an application's stage, its elapsed silence, and whether
// any unconfirmed suggestion points at it, to a silence state — or "" when the
// stage never accrues silence.
//
// The threshold is the last tolerated day, not the first offending one. A
// pending suggestion only ever softens a silence claim into a question: mail
// awaiting confirmation on an application that is answering promptly is not a
// problem to report, so it never turns `active` into anything.
func SilenceStateFor(stage string, daysSilent int, pendingSuggestion bool) string {
	threshold, ok := SilenceThresholdDays(stage)
	if !ok {
		return ""
	}
	if daysSilent <= threshold {
		return SilenceActive
	}
	if pendingSuggestion {
		return SilenceUnconfirmed
	}
	return SilenceSilent
}
