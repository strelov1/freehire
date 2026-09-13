package mentorship

import (
	"fmt"
	"slices"
	"time"
)

// CalendarStatus labels one interval of a mentor's own resolved calendar. It marshals as
// its own string value, so the API layer needs no translation table.
type CalendarStatus string

const (
	// StatusBooked is covered by a confirmed booking.
	StatusBooked CalendarStatus = "booked"
	// StatusBusy is covered by a synced busy interval from the mentor's connected
	// calendar.
	StatusBusy CalendarStatus = "busy"
	// StatusFree is available and currently offered to a seeker as a bookable slot.
	StatusFree CalendarStatus = "free"
	// StatusClosed is outside the mentor's stated availability, or inside it but
	// withheld by a buffer, minimum notice, or the booking horizon.
	StatusClosed CalendarStatus = "closed"
)

// CalendarInterval is one labeled span of a mentor's own calendar.
type CalendarInterval struct {
	Interval
	Status CalendarStatus
}

// CalendarRequest is everything Calendar needs to partition a window. It mirrors
// SlotRequest, with the mentor's occupied time split into its two sources rather than
// pre-unioned, because the whole point of this computation is to tell them apart.
type CalendarRequest struct {
	// Rules is the mentor's availability, weekly rules and dated overrides together.
	Rules []Rule
	// MentorZone resolves the rules' wall-clock times, and is also the zone the result
	// is reported in — this is a self-view, so there is no separate viewer zone.
	MentorZone *time.Location
	// Params is the session shape and the bounds on when it may be booked.
	Params SessionParams
	// Booked is the mentor's confirmed bookings. Buffers are applied here, not by the
	// caller, exactly as SlotRequest.Busy documents.
	Booked []Interval
	// Busy is the mentor's synced-calendar busy intervals.
	Busy []Interval
	// From and To bound the requested window — typically one calendar month.
	From time.Time
	To   time.Time
	// Now is the instant the notice period and the horizon are measured from.
	Now time.Time
}

// CalendarResult is the window's partition and the zone it is expressed in.
type CalendarResult struct {
	Intervals []CalendarInterval
	Zone      string
}

// Calendar partitions a window into non-overlapping, gapless intervals labeled booked,
// busy, free or closed — the mentor's own view of what Slots computes for a seeker.
//
// The free ranges are computed by the exact same pipeline Slots uses (expandSchedule,
// subtractBusy, sliceSlots, the notice/horizon filter), so the two can never disagree.
// Booked and busy are then carved out ahead of free, in that priority order, and
// whatever is left over — outside the mentor's stated hours, or inside them but withheld
// by a buffer, the notice period or the horizon — is closed.
func Calendar(req CalendarRequest) (CalendarResult, error) {
	if req.MentorZone == nil {
		return CalendarResult{}, ErrNoMentorZone
	}
	if err := req.Params.Validate(); err != nil {
		return CalendarResult{}, fmt.Errorf("calendar request: %w", err)
	}
	params := req.Params
	zoneName := req.MentorZone.String()

	w := Interval{Start: req.From, End: req.To}
	if w.IsEmpty() {
		return CalendarResult{Zone: zoneName}, nil
	}

	earliest := req.Now.Add(params.MinimumNotice)
	latest := req.Now.Add(params.Horizon)

	avail := expandSchedule(req.Rules, req.MentorZone, w)
	allBusy := make([]Interval, 0, len(req.Booked)+len(req.Busy))
	allBusy = append(allBusy, req.Booked...)
	allBusy = append(allBusy, req.Busy...)
	free := subtractBusy(avail, allBusy, params)

	var freeRanges []Interval
	for _, s := range sliceSlots(free, params) {
		if s.Start.Before(earliest) || s.Start.After(latest) {
			continue
		}
		freeRanges = append(freeRanges, s)
	}

	// Carved out in priority order: booked first, then busy (minus whatever is already
	// booked, in case a booking and a synced interval happen to cover the same span),
	// then free. Whatever remains is closed.
	noBuffers := SessionParams{}
	bookedRanges := mergeOverlapping(clipAll(req.Booked, w))
	remaining := subtractBusy([]Interval{w}, bookedRanges, noBuffers)

	busyRanges := subtractBusy(mergeOverlapping(clipAll(req.Busy, w)), bookedRanges, noBuffers)
	remaining = subtractBusy(remaining, busyRanges, noBuffers)

	remaining = subtractBusy(remaining, freeRanges, noBuffers)

	intervals := make([]CalendarInterval, 0,
		len(bookedRanges)+len(busyRanges)+len(freeRanges)+len(remaining))
	for _, iv := range bookedRanges {
		intervals = append(intervals, CalendarInterval{Interval: iv, Status: StatusBooked})
	}
	for _, iv := range busyRanges {
		intervals = append(intervals, CalendarInterval{Interval: iv, Status: StatusBusy})
	}
	for _, iv := range freeRanges {
		intervals = append(intervals, CalendarInterval{Interval: iv, Status: StatusFree})
	}
	for _, iv := range remaining {
		intervals = append(intervals, CalendarInterval{Interval: iv, Status: StatusClosed})
	}
	slices.SortFunc(intervals, func(a, b CalendarInterval) int {
		return compareIntervals(a.Interval, b.Interval)
	})

	return CalendarResult{Intervals: intervals, Zone: zoneName}, nil
}

// clipAll clips every interval to a window, dropping the ones that end up empty.
func clipAll(ivs []Interval, w Interval) []Interval {
	out := make([]Interval, 0, len(ivs))
	for _, iv := range ivs {
		if c := clip(iv, w); !c.IsEmpty() {
			out = append(out, c)
		}
	}
	return out
}
