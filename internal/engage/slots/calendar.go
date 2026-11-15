package slots

import (
	"fmt"
	"slices"
	"time"
)

// CalendarStatus labels one interval of a mentor's own resolved calendar. It marshals
// as its string value.
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
// SlotRequest, but keeps booked and busy time separate so they can be told apart.
type CalendarRequest struct {
	// Rules is the mentor's availability, weekly rules and dated overrides together.
	Rules []Rule
	// OwnerZone resolves the rules' wall-clock times and is the zone the result is
	// reported in (a self-view has no separate viewer zone).
	OwnerZone *time.Location
	// Params is the session shape and the bounds on when it may be booked.
	Params SessionParams
	// Booked is the mentor's confirmed bookings. Buffers are applied here, not by the
	// caller, as in SlotRequest.Busy.
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
// Free ranges come from the same pipeline Slots uses (expandSchedule, subtractBusy,
// sliceSlots, the notice/horizon filter), so the two cannot disagree. Booked and busy
// are carved out ahead of free, in that order; the remainder is closed.
func Calendar(req CalendarRequest) (CalendarResult, error) {
	if req.OwnerZone == nil {
		return CalendarResult{}, ErrNoOwnerZone
	}
	if err := req.Params.Validate(); err != nil {
		return CalendarResult{}, fmt.Errorf("calendar request: %w", err)
	}
	params := req.Params
	zoneName := req.OwnerZone.String()

	w := Interval{Start: req.From, End: req.To}
	if w.IsEmpty() {
		return CalendarResult{Zone: zoneName}, nil
	}

	earliest := req.Now.Add(params.MinimumNotice)
	latest := req.Now.Add(params.Horizon)

	avail := expandSchedule(req.Rules, req.OwnerZone, w)
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

	// Priority order: booked, then busy (minus booked, in case both cover the same
	// span), then free.
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
