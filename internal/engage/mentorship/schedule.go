package mentorship

import (
	"errors"
	"fmt"
	"time"
)

// The sentinels the schedule value types raise. Each carries its HTTP mapping here, in
// the manner of internal/engage/referral: the handler's error switch is the only place
// that maps them, and a sentinel without a mapping is a sentinel nobody can answer.
var (
	// ErrInvalidTimeOfDay → 400. A stored availability time outside 00:00–24:00.
	ErrInvalidTimeOfDay = errors.New("mentorship: not a time of day")
	// ErrInvalidDate → 400. A calendar date that does not exist.
	ErrInvalidDate = errors.New("mentorship: not a calendar date")
	// ErrInvalidWeekday → 400. A weekday outside Sunday..Saturday.
	ErrInvalidWeekday = errors.New("mentorship: not a weekday")
	// ErrInvalidSpan → 400. An availability span that runs backwards, or an empty one
	// where emptiness means nothing (a weekly rule).
	ErrInvalidSpan = errors.New("mentorship: not an availability span")
	// ErrInvalidSessionParams → 400. Session figures that cannot yield a slot.
	ErrInvalidSessionParams = errors.New("mentorship: session parameters cannot yield a slot")
)

// minutesInADay is the exclusive upper bound on a wall-clock minute and the inclusive
// upper bound on a TimeOfDay: 24:00 is a valid END of a day and not a valid start of
// anything, which is what lets a mentor be available "until midnight" without an
// availability row spilling onto the next date.
const minutesInADay = 24 * 60

// weekdayUnset is what a dated rule's weekday reads as. time.Weekday's zero value is
// Sunday, a real day, so a rule cannot use the zero value to mean "no weekday" — the
// sentinel is out of range instead, and NewWeeklyRule refuses it like any other.
const weekdayUnset = time.Weekday(-1)

// TimeOfDay is a wall-clock time carrying no date and no zone, the shape a mentor's
// availability is stored in. It is resolved into an instant only against a date AND the
// mentor's own IANA zone — see resolve in slots.go. Minutes, not seconds: a mentor
// states hours, a TIME column stores what we put in it, and admitting seconds would
// invite a slot grid that no wall clock lands on.
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

// Minutes is the time as minutes since midnight, which is how the slot engine adds it
// to a date.
func (t TimeOfDay) Minutes() int { return t.minutes }

// String renders the 24-hour clock, so a test failure reads as a time.
func (t TimeOfDay) String() string { return fmt.Sprintf("%02d:%02d", t.minutes/60, t.minutes%60) }

// Date is a calendar date with no time and no zone: the date a dated rule names. It is
// not a time.Time, because a time.Time is an instant and an instant needs a zone —
// exactly the conflation this package must not make.
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

// String renders ISO-8601, so a test failure reads as a date.
func (d Date) String() string { return fmt.Sprintf("%04d-%02d-%02d", d.year, int(d.month), d.day) }

// next is the following calendar date.
//
// The arithmetic goes through UTC deliberately, and this is not a detail. Doing it in a
// real zone means asking for a midnight, and three live zones — America/Santiago,
// America/Havana, Atlantic/Azores — move their clocks AT midnight, so once a year that
// midnight does not exist. Go normalises such a time BACKWARDS, to 23:00 on the previous
// date, and a cursor built that way stops advancing: the walk spins forever on an
// unauthenticated endpoint. UTC has no transitions, so every midnight in it is real.
func (d Date) next() Date {
	at := time.Date(d.year, d.month, d.day+1, 0, 0, 0, 0, time.UTC)
	return Date{year: at.Year(), month: at.Month(), day: at.Day()}
}

// after reports whether this date falls later than the other. Compared component-wise
// rather than as instants, because a date is not an instant and turning it into one is
// what the zone hazard above lives in.
func (d Date) after(other Date) bool {
	if d.year != other.year {
		return d.year > other.year
	}
	if d.month != other.month {
		return d.month > other.month
	}
	return d.day > other.day
}

// Weekday is the day of the week this date falls on. Read in UTC for the reason next
// gives: the weekday of a calendar date is the same in every zone, and asking a real
// zone for it would mean naming a wall-clock time that might not exist.
func (d Date) Weekday() time.Weekday {
	return time.Date(d.year, d.month, d.day, 0, 0, 0, 0, time.UTC).Weekday()
}

// Rule is one row of a mentor's availability, in one of two shapes: WEEKLY, naming a
// weekday that recurs, or DATED, naming one calendar date that replaces whatever the
// weekly rules say for it.
//
// The fields are unexported and the two constructors are the only way in, so a rule
// carrying both a weekday and a date — the row the database CHECK rejects — cannot be
// built here at all.
type Rule struct {
	dated   bool
	weekday time.Weekday
	date    Date
	start   TimeOfDay
	end     TimeOfDay
}

// NewWeeklyRule builds a recurring rule. Its span must be non-empty: a weekly row
// saying "Tuesday, 18:00 to 18:00" states nothing, and only a dated override is allowed
// to be empty — there, emptiness is the point.
func NewWeeklyRule(weekday time.Weekday, start, end TimeOfDay) (Rule, error) {
	if weekday < time.Sunday || weekday > time.Saturday {
		return Rule{}, fmt.Errorf("%w: %d", ErrInvalidWeekday, int(weekday))
	}
	if end.minutes <= start.minutes {
		return Rule{}, fmt.Errorf("%w: weekly %s–%s", ErrInvalidSpan, start, end)
	}
	return Rule{weekday: weekday, start: start, end: end}, nil
}

// NewDatedRule builds a rule for one calendar date. An EMPTY span is deliberately
// legal: it is how a mentor closes a day, displacing that date's weekly rules with
// nothing. A backwards span is not.
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
// booked. It is passed to the slot engine rather than read from a mentor, which is the
// seam that lets a per-session-type row take its place later without the engine
// changing shape.
type SessionParams struct {
	// Duration is how long one session runs.
	Duration time.Duration
	// BufferBefore is the gap required before a session starts, and BufferAfter the gap
	// after one ends. Both apply between two sessions, so the gap the engine requires
	// between them is their sum.
	BufferBefore time.Duration
	BufferAfter  time.Duration
	// MinimumNotice is how far ahead of now the earliest offerable slot begins.
	MinimumNotice time.Duration
	// Horizon is how far ahead of now the latest offerable slot begins.
	Horizon time.Duration
}

// Validate reports whether the figures can yield a slot. Zero buffers and zero notice
// are ordinary choices; a zero duration or horizon is not — the first makes a mentor
// bookable for no time, the second bookable in no window.
//
// A method rather than a New- constructor, unlike every other type in this file: those
// have unexported fields and cannot be built without one, while these are plain figures
// a caller assembles directly. A constructor here would only echo back what it was
// given, and a caller who ignored the returned copy would be running unvalidated
// parameters that looked checked.
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
	// Availability is stored to the minute, so a session measured finer than that could
	// never start on a boundary the schedule produces. Rounding it silently would move
	// hours the mentor stated; refusing it says so.
	for _, d := range []time.Duration{p.Duration, p.BufferBefore, p.BufferAfter} {
		if d%time.Minute != 0 {
			return fmt.Errorf("%w: %v is not whole minutes", ErrInvalidSessionParams, d)
		}
	}
	return nil
}

// Interval is a half-open span of absolute time, [Start, End). Half-open is what makes
// back-to-back sessions legal: 18:00–19:00 and 19:00–20:00 do not overlap, and the
// database EXCLUDE constraint uses the same bounds so Go and Postgres agree on it.
type Interval struct {
	Start time.Time
	End   time.Time
}

// Overlaps reports whether two intervals share any instant.
func (i Interval) Overlaps(other Interval) bool {
	return i.Start.Before(other.End) && other.Start.Before(i.End)
}

// IsEmpty reports whether the interval holds no time at all. An empty interval survives
// the expansion of a closing override just long enough to displace that date's weekly
// rules, and is dropped before slicing.
func (i Interval) IsEmpty() bool { return !i.End.After(i.Start) }
