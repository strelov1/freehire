package slots

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrNoOwnerZone → 500. An owner's row reached the engine without the IANA zone that
// gives their stored hours meaning. A broken row, not a bad request; guessing a zone
// would silently move every slot.
var ErrNoOwnerZone = errors.New("slots: the owner has no timezone")

// SlotRequest is everything the slot engine needs. Now is injected, keeping the
// pipeline pure: a cached window and a booking's re-derivation cannot disagree.
type SlotRequest struct {
	// Rules is the mentor's availability, weekly rules and dated overrides together.
	Rules []Rule
	// OwnerZone resolves the rules' wall-clock times into instants.
	OwnerZone *time.Location
	// Params is the session shape and the bounds on when it may be booked.
	Params SessionParams
	// Busy is the mentor's occupied time (bookings, calendar free/busy). Buffers are
	// applied here, not by the caller.
	Busy []Interval
	// From and To bound the window asked about, before the horizon narrows it.
	From time.Time
	To   time.Time
	// Now is the instant the notice period and the horizon are measured from.
	Now time.Time
	// ViewerZone is the IANA name the slots are expressed in. Unknown or absent falls
	// back to UTC, and SlotResult.Zone reports what was actually used.
	ViewerZone string
}

// SlotResult is the offerable slots and the zone they are expressed in. Each slot's
// times carry that zone as their location, so one value reads as local wall-clock
// time and compares as an absolute instant.
type SlotResult struct {
	Slots []Interval
	Zone  string
}

// Slots is the engine's whole pipeline: expand the schedule, subtract busy time and
// buffers, slice into sessions, drop what the notice period and the horizon forbid, and
// express the rest in the viewer's zone.
//
// An empty, backwards, or past window is not an error, just no slots. Only a request
// that could never yield a slot is refused: bad session parameters or no owner zone.
func Slots(req SlotRequest) (SlotResult, error) {
	if req.OwnerZone == nil {
		return SlotResult{}, ErrNoOwnerZone
	}
	if err := req.Params.Validate(); err != nil {
		return SlotResult{}, fmt.Errorf("slot request: %w", err)
	}
	params := req.Params

	viewerZone, zoneName := resolveViewerZone(req.ViewerZone)

	earliest := req.Now.Add(params.MinimumNotice)
	latest := req.Now.Add(params.Horizon)

	// The window never reaches into the past or past the horizon. The far end gets one
	// session of slack because the horizon bounds when a slot may BEGIN.
	//
	// The NEAR end is always moved DOWN to a day boundary in the mentor's zone, never
	// left at an arbitrary instant: sliceSlots anchors its grid to each range's start,
	// so a range starting at 18:33 would offer 18:33, 19:33 to a mentor who stated
	// 18:00. The HTTP endpoint defaults `from` to now, so clamping only an earlier
	// bound is not enough (a test supplying midnight hides this). A moving grid breaks
	// the mentor's stated hours, the cache key (it carries no `now`), and the booking's
	// re-derivation at a later `now`, which would refuse a slot the seeker clicked.
	//
	// Widening backwards is free: the notice filter below drops past slots.
	w := Interval{Start: startOfDayIn(req.From, req.OwnerZone), End: req.To}
	if today := startOfDayIn(req.Now, req.OwnerZone); w.Start.Before(today) {
		w.Start = today
	}
	if limit := latest.Add(params.Duration); w.End.After(limit) {
		w.End = limit
	}

	free := subtractBusy(expandSchedule(req.Rules, req.OwnerZone, w), req.Busy, params)

	// Non-nil even when empty: a nil slice marshals to `null` instead of `[]`.
	slots := []Interval{}
	for _, s := range sliceSlots(free, params) {
		if s.Start.Before(earliest) || s.Start.After(latest) {
			continue
		}
		slots = append(slots, Interval{Start: s.Start.In(viewerZone), End: s.End.In(viewerZone)})
	}

	return SlotResult{Slots: slots, Zone: zoneName}, nil
}

// resolveViewerZone turns an IANA name into a zone, falling back to UTC and reporting
// the fallback. It must never fall back to the OWNER's zone: a visitor shown the
// owner's times labelled as their own cannot notice.
//
// The shape check replaces a bare LoadLocation (as in mentorship's validateMentorZone):
// Go resolves "Local" to the SERVER's zone, and "EST" and "Factory" to non-places.
// Requiring a slash, or exactly "UTC", admits IANA names only.
func resolveViewerZone(name string) (*time.Location, string) {
	if name == "" {
		return time.UTC, "UTC"
	}
	if name != "UTC" && !strings.Contains(name, "/") {
		return time.UTC, "UTC"
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC, "UTC"
	}
	return loc, name
}
