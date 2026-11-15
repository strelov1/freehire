package slots

import (
	"errors"
	"fmt"
	"time"
)

// The sentinels the schedule value types raise, each with its HTTP mapping; the
// handler's error switch is the only place that maps them.
//
// 422 rather than 400: the body parsed fine, and what is wrong is what the VALUES say
// (a weekday of 9, a span that runs backwards).
var (
	// ErrInvalidTimeOfDay → 422. A stored availability time outside 00:00–24:00.
	ErrInvalidTimeOfDay = errors.New("slots: not a time of day")
	// ErrInvalidDate → 422. A calendar date that does not exist.
	ErrInvalidDate = errors.New("slots: not a calendar date")
	// ErrInvalidWeekday → 422. A weekday outside Sunday..Saturday.
	ErrInvalidWeekday = errors.New("slots: not a weekday")
	// ErrInvalidSpan → 422. An availability span that runs backwards, or an empty one
	// where emptiness means nothing (a weekly rule).
	ErrInvalidSpan = errors.New("slots: not an availability span")
	// ErrInvalidSessionParams → 422. Session figures that cannot yield a slot.
	ErrInvalidSessionParams = errors.New("slots: session parameters cannot yield a slot")
)

// minutesInADay is the exclusive upper bound on a wall-clock minute and the inclusive
// upper bound on a TimeOfDay: 24:00 is a valid END of a day, so a rule can run "until
// midnight" without spilling onto the next date.
const minutesInADay = 24 * 60

// weekdayUnset is a dated rule's weekday. The zero value is Sunday, so the sentinel
// is out of range instead, and NewWeeklyRule refuses it.
const weekdayUnset = time.Weekday(-1)

// TimeOfDay is a wall-clock time with no date and no zone, the shape a mentor's
// availability is stored in. It becomes an instant only against a date AND the
// mentor's IANA zone (see resolve). Minutes, not seconds: seconds would invite a slot
// grid no wall clock lands on.
type TimeOfDay struct {
	minutes int
}

// NewTimeOfDay builds a time of day from an hour and a minute, admitting 24:00 as the
// end of the day and nothing past it.
func NewTimeOfDay(hour, minute int) (TimeOfDay, error) {
	if hour < 0 || minute < 0 || minute > 59 {
		return TimeOfDay{}, fmt.Errorf("%w: %02d:%02d", ErrInvalidTimeOfDay, hour, minute)
	}
	total := hour*60 + minute
	if total > minutesInADay {
		return TimeOfDay{}, fmt.Errorf("%w: %02d:%02d", ErrInvalidTimeOfDay, hour, minute)
	}
	return TimeOfDay{minutes: total}, nil
}

// Minutes is the time as minutes since midnight.
func (t TimeOfDay) Minutes() int { return t.minutes }

// String renders the 24-hour clock.
func (t TimeOfDay) String() string { return fmt.Sprintf("%02d:%02d", t.minutes/60, t.minutes%60) }

// Date is a calendar date with no time and no zone: the date a dated rule names. It is
// not a time.Time, which is an instant and needs a zone.
type Date struct {
	year  int
	month time.Month
	day   int
}

// NewDate builds a calendar date, refusing one no calendar has. Go's time.Date
// normalises a bad date rather than rejecting it (February 31st becomes March 3rd), so
// round-tripping through it is the check: a date that survives unchanged is real.
func NewDate(year int, month time.Month, day int) (Date, error) {
	normalised := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	if normalised.Year() != year || normalised.Month() != month || normalised.Day() != day {
		return Date{}, fmt.Errorf("%w: %04d-%02d-%02d", ErrInvalidDate, year, int(month), day)
	}
	return Date{year: year, month: month, day: day}, nil
}

// Parts is the date as its three components, for building an instant in a zone.
func (d Date) Parts() (int, time.Month, int) { return d.year, d.month, d.day }

// String renders ISO-8601.
func (d Date) String() string { return fmt.Sprintf("%04d-%02d-%02d", d.year, int(d.month), d.day) }

// next is the following calendar date.
//
// The arithmetic goes through UTC on purpose. In America/Santiago, America/Havana and
// Atlantic/Azores the clocks move AT midnight, so once a year that midnight does not
// exist; Go normalises it backwards to 23:00 of the previous date, the cursor stops
// advancing, and the walk spins forever on an unauthenticated endpoint.
func (d Date) next() Date {
	at := time.Date(d.year, d.month, d.day+1, 0, 0, 0, 0, time.UTC)
	return Date{year: at.Year(), month: at.Month(), day: at.Day()}
}

// after reports whether this date falls later than the other, compared component-wise
// to avoid the zone hazard described on next.
func (d Date) after(other Date) bool {
	if d.year != other.year {
		return d.year > other.year
	}
	if d.month != other.month {
		return d.month > other.month
	}
	return d.day > other.day
}

// Weekday is the day of the week this date falls on. Read in UTC for the reason given
// on next; a date's weekday is the same in every zone.
func (d Date) Weekday() time.Weekday {
	return time.Date(d.year, d.month, d.day, 0, 0, 0, 0, time.UTC).Weekday()
}

// Rule is one row of a mentor's availability, in one of two shapes: WEEKLY, naming a
// weekday that recurs, or DATED, naming one calendar date that replaces whatever the
// weekly rules say for it.
//
// The fields are unexported and the two constructors are the only way in, so a rule
// with both a weekday and a date (which the database CHECK rejects) cannot be built.
type Rule struct {
	// id is the stored row's identifier, zero for a rule just built. The cabinet needs
	// it to delete one override without touching the rest.
	id      int64
	dated   bool
	weekday time.Weekday
	date    Date
	start   TimeOfDay
	end     TimeOfDay
}

// ID is the stored row's identifier, or zero for a rule that has not been read from one.
func (r Rule) ID() int64 { return r.id }

// WithID labels a rule with the row it came from. Only a repository calls it.
func (r Rule) WithID(id int64) Rule {
	r.id = id
	return r
}

// NewWeeklyRule builds a recurring rule. Its span must be non-empty; only a dated
// override may be empty.
func NewWeeklyRule(weekday time.Weekday, start, end TimeOfDay) (Rule, error) {
	if weekday < time.Sunday || weekday > time.Saturday {
		return Rule{}, fmt.Errorf("%w: %d", ErrInvalidWeekday, int(weekday))
	}
	if end.minutes <= start.minutes {
		return Rule{}, fmt.Errorf("%w: weekly %s–%s", ErrInvalidSpan, start, end)
	}
	return Rule{weekday: weekday, start: start, end: end}, nil
}

// NewDatedRule builds a rule for one calendar date. An EMPTY span is legal: it closes
// the day. A backwards span is not.
func NewDatedRule(date Date, start, end TimeOfDay) (Rule, error) {
	if end.minutes < start.minutes {
		return Rule{}, fmt.Errorf("%w: dated %s–%s", ErrInvalidSpan, start, end)
	}
	return Rule{dated: true, weekday: weekdayUnset, date: date, start: start, end: end}, nil
}

// IsDated reports whether this rule names a calendar date rather than a weekday.
func (r Rule) IsDated() bool { return r.dated }

// IsClosure reports whether this rule closes its date: a dated rule with an empty span.
// A weekly rule can never be one, because NewWeeklyRule refuses an empty span.
func (r Rule) IsClosure() bool { return r.dated && r.start.minutes == r.end.minutes }

// Weekday is the recurring day, or weekdayUnset for a dated rule.
func (r Rule) Weekday() time.Weekday { return r.weekday }

// Date is the named date, or the zero Date for a weekly rule.
func (r Rule) Date() Date { return r.date }

// Start is the rule's opening wall-clock time, in the mentor's own zone.
func (r Rule) Start() TimeOfDay { return r.start }

// End is the rule's closing wall-clock time, in the mentor's own zone.
func (r Rule) End() TimeOfDay { return r.end }

// SessionParams is what a mentor's one session offers and what bounds when it may be
// booked. It is passed to the engine rather than read from a mentor, so a
// per-session-type row can replace it later.
type SessionParams struct {
	// Duration is how long one session runs.
	Duration time.Duration
	// BufferBefore and BufferAfter are the gaps required around a session; between two
	// sessions the required gap is their sum.
	BufferBefore time.Duration
	BufferAfter  time.Duration
	// MinimumNotice is how far ahead of now the earliest offerable slot begins.
	MinimumNotice time.Duration
	// Horizon is how far ahead of now the latest offerable slot begins.
	Horizon time.Duration
}

// Validate reports whether the figures can yield a slot. Zero buffers and notice are
// ordinary; a zero duration or horizon is not.
//
// A method, not a constructor: these are plain figures a caller assembles directly,
// and a constructor's returned copy could be ignored, leaving unvalidated parameters
// that look checked.
func (p SessionParams) Validate() error {
	switch {
	case p.Duration <= 0:
		return fmt.Errorf("%w: duration %v", ErrInvalidSessionParams, p.Duration)
	case p.Horizon <= 0:
		return fmt.Errorf("%w: horizon %v", ErrInvalidSessionParams, p.Horizon)
	case p.BufferBefore < 0 || p.BufferAfter < 0:
		return fmt.Errorf("%w: negative buffer", ErrInvalidSessionParams)
	case p.MinimumNotice < 0:
		return fmt.Errorf("%w: notice %v", ErrInvalidSessionParams, p.MinimumNotice)
	}
	// Availability is stored to the minute, so a finer session could never start on a
	// schedule boundary. Refuse rather than silently round the mentor's hours.
	for _, d := range []time.Duration{p.Duration, p.BufferBefore, p.BufferAfter} {
		if d%time.Minute != 0 {
			return fmt.Errorf("%w: %v is not whole minutes", ErrInvalidSessionParams, d)
		}
	}
	return nil
}

// Interval is a half-open span of absolute time, [Start, End), so back-to-back
// sessions do not overlap. The database EXCLUDE constraint uses the same bounds.
type Interval struct {
	Start time.Time
	End   time.Time
}

// Overlaps reports whether two intervals share any instant.
func (i Interval) Overlaps(other Interval) bool {
	return i.Start.Before(other.End) && other.Start.Before(i.End)
}

// IsEmpty reports whether the interval holds no time. An empty interval from a closing
// override displaces that date's weekly rules and is dropped before slicing.
func (i Interval) IsEmpty() bool { return !i.End.After(i.Start) }
