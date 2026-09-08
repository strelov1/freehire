package mentorship

import (
	"errors"
	"testing"
	"time"
)

func TestTimeOfDayAcceptsTheDayAndMidnightAtBothEnds(t *testing.T) {
	for _, tc := range []struct {
		name    string
		hour    int
		minute  int
		wantMin int
	}{
		{"midnight", 0, 0, 0},
		{"a working hour", 18, 30, 18*60 + 30},
		{"the last minute", 23, 59, 23*60 + 59},
		{"end of day", 24, 0, 24 * 60},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NewTimeOfDay(tc.hour, tc.minute)
			if err != nil {
				t.Fatalf("NewTimeOfDay(%d, %d) = %v", tc.hour, tc.minute, err)
			}
			if got.Minutes() != tc.wantMin {
				t.Errorf("Minutes() = %d, want %d", got.Minutes(), tc.wantMin)
			}
		})
	}
}

// 24:00 is the end of a day and nothing past it is; a mentor available "until midnight"
// must be expressible without spilling into the next date, which is what an availability
// row cannot do.
func TestTimeOfDayRefusesWhatIsNotATimeOfDay(t *testing.T) {
	for _, tc := range []struct {
		name   string
		hour   int
		minute int
	}{
		{"past the end of the day", 24, 1},
		{"an hour that does not exist", 25, 0},
		{"a negative hour", -1, 0},
		{"a minute that does not exist", 12, 60},
		{"a negative minute", 12, -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewTimeOfDay(tc.hour, tc.minute); !errors.Is(err, ErrInvalidTimeOfDay) {
				t.Errorf("NewTimeOfDay(%d, %d) error = %v, want ErrInvalidTimeOfDay", tc.hour, tc.minute, err)
			}
		})
	}
}

func TestWeeklyRuleCarriesItsWeekdayAndHours(t *testing.T) {
	from, to := mustTimeOfDay(t, 18, 0), mustTimeOfDay(t, 20, 0)

	rule, err := NewWeeklyRule(time.Tuesday, from, to)
	if err != nil {
		t.Fatalf("NewWeeklyRule: %v", err)
	}
	if rule.IsDated() {
		t.Error("a weekly rule reports itself as dated")
	}
	if rule.Weekday() != time.Tuesday {
		t.Errorf("Weekday() = %v, want Tuesday", rule.Weekday())
	}
	if rule.Start() != from || rule.End() != to {
		t.Errorf("hours = %v–%v, want %v–%v", rule.Start(), rule.End(), from, to)
	}
}

// A weekly row saying "Tuesday, from 18:00 to 18:00" states nothing. Only a dated
// override may be empty, where emptiness is the whole point — it closes that day.
func TestWeeklyRuleRefusesAnEmptyOrBackwardsSpan(t *testing.T) {
	for _, tc := range []struct {
		name       string
		start, end TimeOfDay
	}{
		{"empty", mustTimeOfDay(t, 18, 0), mustTimeOfDay(t, 18, 0)},
		{"backwards", mustTimeOfDay(t, 20, 0), mustTimeOfDay(t, 18, 0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewWeeklyRule(time.Tuesday, tc.start, tc.end); !errors.Is(err, ErrInvalidSpan) {
				t.Errorf("error = %v, want ErrInvalidSpan", err)
			}
		})
	}
}

func TestWeeklyRuleRefusesADayOutsideTheWeek(t *testing.T) {
	from, to := mustTimeOfDay(t, 18, 0), mustTimeOfDay(t, 20, 0)

	for _, day := range []time.Weekday{-1, 7, 99} {
		if _, err := NewWeeklyRule(day, from, to); !errors.Is(err, ErrInvalidWeekday) {
			t.Errorf("NewWeeklyRule(%d) error = %v, want ErrInvalidWeekday", day, err)
		}
	}
}

func TestDatedRuleCarriesItsDateAndHours(t *testing.T) {
	date := mustDate(t, 2026, time.September, 15)
	from, to := mustTimeOfDay(t, 10, 0), mustTimeOfDay(t, 12, 0)

	rule, err := NewDatedRule(date, from, to)
	if err != nil {
		t.Fatalf("NewDatedRule: %v", err)
	}
	if !rule.IsDated() {
		t.Error("a dated rule does not report itself as dated")
	}
	if rule.Date() != date {
		t.Errorf("Date() = %v, want %v", rule.Date(), date)
	}
	if rule.IsClosure() {
		t.Error("a 10:00–12:00 override reports itself as a closure")
	}
}

// The whole trick: an override with equal start and end closes its date. It must be
// constructible, because a mentor's day off is one inserted row and not a deletion.
func TestAnEmptyDatedRuleIsAClosure(t *testing.T) {
	date := mustDate(t, 2026, time.September, 16)
	noon := mustTimeOfDay(t, 12, 0)

	rule, err := NewDatedRule(date, noon, noon)
	if err != nil {
		t.Fatalf("NewDatedRule with an empty span: %v", err)
	}
	if !rule.IsClosure() {
		t.Error("an empty override does not report itself as a closure")
	}
}

func TestDatedRuleRefusesABackwardsSpan(t *testing.T) {
	date := mustDate(t, 2026, time.September, 15)
	start, end := mustTimeOfDay(t, 20, 0), mustTimeOfDay(t, 18, 0)

	if _, err := NewDatedRule(date, start, end); !errors.Is(err, ErrInvalidSpan) {
		t.Errorf("error = %v, want ErrInvalidSpan", err)
	}
}

// A rule is one kind or the other, and the type is what enforces it: there is no
// constructor that takes both a weekday and a date, so no caller can build the row the
// database CHECK would reject.
func TestADatedRuleHasNoWeekdayAndAWeeklyRuleHasNoDate(t *testing.T) {
	from, to := mustTimeOfDay(t, 9, 0), mustTimeOfDay(t, 17, 0)

	weekly, err := NewWeeklyRule(time.Monday, from, to)
	if err != nil {
		t.Fatalf("NewWeeklyRule: %v", err)
	}
	if weekly.Date() != (Date{}) {
		t.Errorf("a weekly rule carries a date: %v", weekly.Date())
	}

	dated, err := NewDatedRule(mustDate(t, 2026, time.September, 15), from, to)
	if err != nil {
		t.Fatalf("NewDatedRule: %v", err)
	}
	if dated.Weekday() != weekdayUnset {
		t.Errorf("a dated rule carries a weekday: %v", dated.Weekday())
	}
}

func TestDateRefusesADayThatDoesNotExist(t *testing.T) {
	for _, tc := range []struct {
		name  string
		year  int
		month time.Month
		day   int
	}{
		{"the 31st of February", 2026, time.February, 31},
		{"the 29th of a common February", 2026, time.February, 29},
		{"the 31st of a thirty-day month", 2026, time.September, 31},
		{"day zero", 2026, time.September, 0},
		{"month zero", 2026, 0, 15},
		{"month thirteen", 2026, 13, 15},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewDate(tc.year, tc.month, tc.day); !errors.Is(err, ErrInvalidDate) {
				t.Errorf("NewDate(%d, %v, %d) error = %v, want ErrInvalidDate", tc.year, tc.month, tc.day, err)
			}
		})
	}
}

// A leap day is a real date and must not be caught by the guard above.
func TestDateAcceptsALeapDay(t *testing.T) {
	if _, err := NewDate(2028, time.February, 29); err != nil {
		t.Errorf("NewDate(2028-02-29) = %v, want a valid date", err)
	}
}

func TestSessionParamsAcceptOrdinaryFigures(t *testing.T) {
	params := SessionParams{
		Duration:      60 * time.Minute,
		BufferBefore:  10 * time.Minute,
		BufferAfter:   15 * time.Minute,
		MinimumNotice: 2 * time.Hour,
		Horizon:       30 * 24 * time.Hour,
	}

	if err := params.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

// Zero buffers and zero notice are ordinary choices; a zero duration or horizon is not,
// because either makes the mentor bookable for nothing or for no window at all.
func TestSessionParamsRefuseFiguresThatCannotYieldASlot(t *testing.T) {
	valid := SessionParams{
		Duration:      60 * time.Minute,
		MinimumNotice: 2 * time.Hour,
		Horizon:       30 * 24 * time.Hour,
	}

	for _, tc := range []struct {
		name   string
		mutate func(*SessionParams)
	}{
		{"a zero duration", func(p *SessionParams) { p.Duration = 0 }},
		{"a negative duration", func(p *SessionParams) { p.Duration = -time.Minute }},
		{"a zero horizon", func(p *SessionParams) { p.Horizon = 0 }},
		{"a negative horizon", func(p *SessionParams) { p.Horizon = -time.Hour }},
		{"a negative before-buffer", func(p *SessionParams) { p.BufferBefore = -time.Minute }},
		{"a negative after-buffer", func(p *SessionParams) { p.BufferAfter = -time.Minute }},
		{"a negative notice", func(p *SessionParams) { p.MinimumNotice = -time.Minute }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params := valid
			tc.mutate(&params)
			if err := params.Validate(); !errors.Is(err, ErrInvalidSessionParams) {
				t.Errorf("error = %v, want ErrInvalidSessionParams", err)
			}
		})
	}
}

// A duration that is not a whole number of minutes cannot come back out of a TIME column
// or go into a slot grid, and silently rounding one would move a mentor's stated hours.
func TestSessionParamsRefuseSubMinutePrecision(t *testing.T) {
	params := SessionParams{
		Duration:      90*time.Second + 500*time.Millisecond,
		MinimumNotice: 2 * time.Hour,
		Horizon:       30 * 24 * time.Hour,
	}

	if err := params.Validate(); !errors.Is(err, ErrInvalidSessionParams) {
		t.Errorf("error = %v, want ErrInvalidSessionParams", err)
	}
}

// validSession asserts a test's own fixture is a session the engine would accept, so no
// test can pass or fail because of parameters production would have refused.
func validSession(t *testing.T, p SessionParams) SessionParams {
	t.Helper()
	if err := p.Validate(); err != nil {
		t.Fatalf("fixture session parameters are invalid: %v", err)
	}
	return p
}

func TestIntervalReportsOverlapOnHalfOpenBounds(t *testing.T) {
	at := func(hour int) time.Time {
		return time.Date(2026, time.September, 15, hour, 0, 0, 0, time.UTC)
	}
	eighteenToNineteen := Interval{Start: at(18), End: at(19)}

	for _, tc := range []struct {
		name  string
		other Interval
		want  bool
	}{
		{"identical", Interval{Start: at(18), End: at(19)}, true},
		{"straddling the start", Interval{Start: at(17), End: at(19)}, true},
		{"contained", Interval{Start: at(18), End: at(19)}, true},
		{"abutting after", Interval{Start: at(19), End: at(20)}, false},
		{"abutting before", Interval{Start: at(17), End: at(18)}, false},
		{"disjoint", Interval{Start: at(21), End: at(22)}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := eighteenToNineteen.Overlaps(tc.other); got != tc.want {
				t.Errorf("Overlaps = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIntervalIsEmptyWhenItHasNoWidth(t *testing.T) {
	at := time.Date(2026, time.September, 15, 18, 0, 0, 0, time.UTC)

	if !(Interval{Start: at, End: at}).IsEmpty() {
		t.Error("a zero-width interval does not report itself as empty")
	}
	if (Interval{Start: at, End: at.Add(time.Minute)}).IsEmpty() {
		t.Error("a one-minute interval reports itself as empty")
	}
}

func mustTimeOfDay(t *testing.T, hour, minute int) TimeOfDay {
	t.Helper()
	got, err := NewTimeOfDay(hour, minute)
	if err != nil {
		t.Fatalf("NewTimeOfDay(%d, %d): %v", hour, minute, err)
	}
	return got
}

func mustDate(t *testing.T, year int, month time.Month, day int) Date {
	t.Helper()
	got, err := NewDate(year, month, day)
	if err != nil {
		t.Fatalf("NewDate(%d, %v, %d): %v", year, month, day, err)
	}
	return got
}
