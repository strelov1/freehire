package mentorship

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrNoMentorZone → 500. A mentor row reached the engine without the IANA zone that
// alone gives their stored hours a meaning. It is a broken row rather than a bad
// request, and guessing a zone would silently move every slot.
var ErrNoMentorZone = errors.New("mentorship: the mentor has no timezone")

// SlotRequest is everything the slot engine needs and nothing it could fetch itself.
// Now is injected rather than read from the clock, which is what makes the whole
// pipeline a pure function: the same request always yields the same slots, so a cached
// window and the re-derivation a booking runs cannot disagree.
type SlotRequest struct {
	// Rules is the mentor's availability, weekly rules and dated overrides together.
	Rules []Rule
	// MentorZone resolves the rules' wall-clock times into instants.
	MentorZone *time.Location
	// Params is the session shape and the bounds on when it may be booked.
	Params SessionParams
	// Busy is the mentor's occupied time — confirmed bookings, and the calendar's
	// free/busy intervals once that sync exists. Buffers are applied here, not by the
	// caller.
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
// times carry that zone as their location, so the same value reads as local wall-clock
// time and compares as an absolute instant — the API needs both and there is only one
// field.
type SlotResult struct {
	Slots []Interval
	Zone  string
}

// Slots is the engine's whole pipeline: expand the schedule, subtract busy time and
// buffers, slice into sessions, drop what the notice period and the horizon forbid, and
// express the rest in the viewer's zone.
//
// A window that is empty, backwards, or entirely in the past is not an error — it is a
// question with no slots as its answer. Only a request that could never yield a slot for
// anyone is refused: unusable session parameters, or a mentor with no zone.
func Slots(req SlotRequest) (SlotResult, error) {
	if req.MentorZone == nil {
		return SlotResult{}, ErrNoMentorZone
	}
	if err := req.Params.Validate(); err != nil {
		return SlotResult{}, fmt.Errorf("slot request: %w", err)
	}
	params := req.Params

	viewerZone, zoneName := resolveViewerZone(req.ViewerZone)

	earliest := req.Now.Add(params.MinimumNotice)
	latest := req.Now.Add(params.Horizon)

	// The window never reaches into the past, and never past the horizon. The far end
	// carries one session's slack because the horizon bounds when a slot may BEGIN, so
	// a slot starting exactly on it is offerable and must survive to be sliced.
	//
	// The NEAR end is always moved DOWN to a day boundary in the mentor's zone, never left
	// at an arbitrary instant. sliceSlots anchors its grid to each free range's start, and
	// a range starting at 18:33 produces 18:33, 19:33, 20:33 for a mentor who stated
	// 18:00 — a different grid every minute.
	//
	// Down, not "up to today when it is earlier": the caller that matters here is the HTTP
	// endpoint, which defaults `from` to the current instant. That is AFTER the start of
	// today, so a clamp that only raised an earlier bound left it exactly where the damage
	// is. The first version of this fix did that and was verified against a test supplying
	// midnight, which is already on the grid.
	//
	// Three things break when the grid moves: the mentor's stated hours are not what is
	// offered; the cache key (mentor, window, viewer zone) carries no `now`, so a cached
	// grid answers for a different minute; and the re-derivation a booking runs uses a
	// later `now`, so the slot the seeker clicked has ceased to exist by the time they
	// submit it — refused for a reason nobody can see.
	//
	// Widening the window backwards costs nothing: the notice filter below drops every
	// slot already past, which is the right place for it.
	w := Interval{Start: startOfDayIn(req.From, req.MentorZone), End: req.To}
	if today := startOfDayIn(req.Now, req.MentorZone); w.Start.Before(today) {
		w.Start = today
	}
	if limit := latest.Add(params.Duration); w.End.After(limit) {
		w.End = limit
	}

	free := subtractBusy(expandSchedule(req.Rules, req.MentorZone, w), req.Busy, params)

	// Non-nil even when empty: this renders straight into the `{"data": ...}` list shape,
	// and a nil slice marshals to `null` rather than `[]`. A client that gets null for
	// "no slots this month" has to special-case it, and one that forgets crashes.
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
// the fallback. It must never fall back to the MENTOR's zone: a visitor shown the
// mentor's local times, labelled as their own, has no way to notice.
//
// The shape check is the reason this is not a bare LoadLocation, and it carries the same
// argument validateMentorZone makes: Go resolves "Local" to the SERVER's zone, and "EST"
// and "Factory" to things that are not a place. A visitor served the host's wall clock,
// labelled as their own, has no way to notice. Requiring a slash — or exactly "UTC" —
// admits the IANA names and nothing else.
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
