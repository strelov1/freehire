package mentorship

import (
	"slices"
	"testing"
	"time"
)

// A busy, awkward schedule: two weekly evenings, a morning, a narrowing override, a
// closure, and overlapping busy time. If anything in the pipeline depended on map
// iteration order or mutated its input, this is the request that would show it.
func awkwardRequest(t *testing.T) SlotRequest {
	t.Helper()
	zone := berlin(t)

	weekly := func(day time.Weekday, fromHour, toHour int) Rule {
		rule, err := NewWeeklyRule(day, mustTimeOfDay(t, fromHour, 0), mustTimeOfDay(t, toHour, 0))
		if err != nil {
			t.Fatalf("NewWeeklyRule: %v", err)
		}
		return rule
	}
	closure, err := NewDatedRule(mustDate(t, 2026, time.September, 16), mustTimeOfDay(t, 0, 0), mustTimeOfDay(t, 0, 0))
	if err != nil {
		t.Fatalf("NewDatedRule: %v", err)
	}

	params := validSession(t, SessionParams{
		Duration:      30 * time.Minute,
		BufferBefore:  10 * time.Minute,
		BufferAfter:   5 * time.Minute,
		MinimumNotice: 90 * time.Minute,
		Horizon:       21 * 24 * time.Hour,
	})

	return SlotRequest{
		Rules: []Rule{
			weekly(time.Tuesday, 18, 20),
			weekly(time.Wednesday, 18, 20),
			weekly(time.Tuesday, 9, 11),
			datedRule(t, 15, 10, 12),
			closure,
		},
		MentorZone: zone,
		Params:     params,
		Busy: []Interval{
			{
				Start: time.Date(2026, time.September, 15, 10, 30, 0, 0, zone),
				End:   time.Date(2026, time.September, 15, 11, 0, 0, 0, zone),
			},
			{
				Start: time.Date(2026, time.September, 15, 10, 45, 0, 0, zone),
				End:   time.Date(2026, time.September, 15, 11, 30, 0, 0, zone),
			},
		},
		Now:        time.Date(2026, time.September, 7, 9, 0, 0, 0, zone),
		From:       time.Date(2026, time.September, 7, 0, 0, 0, 0, zone),
		To:         time.Date(2026, time.September, 28, 0, 0, 0, 0, zone),
		ViewerZone: "Asia/Tokyo",
	}
}

func slotKeys(slots []Interval) []string {
	out := make([]string, 0, len(slots))
	for _, s := range slots {
		out = append(out, s.Start.UTC().Format(time.RFC3339)+"/"+s.End.UTC().Format(time.RFC3339))
	}
	return out
}

// The determinism the spec requires: the same request twice is the same answer. It is
// what lets a cached window and the re-derivation a booking runs agree.
//
// Be honest about what this holds and what it does not. Today it CANNOT fail: the only
// maps in the package (override.go) are membership sets whose iteration order never
// reaches the output, and mutation testing confirms it — removing either of the two
// upstream sorts leaves this green, because mergeOverlapping and subtractBusy re-sort
// downstream. Ordering is held by the per-stage tests in expand_test, override_test and
// anchor_test, each of which asserts its own stage's output order.
//
// What this guards is the future: the moment somebody ranges one of those maps to build
// output rather than to look a key up, slots start arriving in a different order per
// call. That is a bug which reproduces on nobody's machine and looks, in production,
// like "the slots move around sometimes".
func TestSlotsAreDeterministicAcrossRepeatedCalls(t *testing.T) {
	req := awkwardRequest(t)

	first := mustSlots(t, req)
	if len(first.Slots) == 0 {
		t.Fatal("the fixture produced no slots; it cannot prove anything")
	}

	for i := range 20 {
		got := mustSlots(t, req)
		if got.Zone != first.Zone {
			t.Fatalf("call %d: Zone = %q, want %q", i, got.Zone, first.Zone)
		}
		if !slices.Equal(slotKeys(got.Slots), slotKeys(first.Slots)) {
			t.Fatalf("call %d differs:\n got %v\nwant %v", i, slotKeys(got.Slots), slotKeys(first.Slots))
		}
	}
}

// The order a mentor happened to write their rules in is not information. Two mentors
// with the same schedule entered in a different order must offer the same slots.
func TestSlotsDoNotDependOnTheOrderOfRulesOrBusyTime(t *testing.T) {
	req := awkwardRequest(t)
	want := slotKeys(mustSlots(t, req).Slots)

	shuffled := req
	shuffled.Rules = slices.Clone(req.Rules)
	slices.Reverse(shuffled.Rules)
	shuffled.Busy = slices.Clone(req.Busy)
	slices.Reverse(shuffled.Busy)

	if got := slotKeys(mustSlots(t, shuffled).Slots); !slices.Equal(got, want) {
		t.Errorf("reversing the inputs changed the slots:\n got %v\nwant %v", got, want)
	}
}

// The engine is a function of its request, so it must not write to it. A caller reusing
// one request for several viewers' zones would otherwise see the second answer built on
// the first's leftovers.
func TestSlotsDoNotMutateTheRequest(t *testing.T) {
	req := awkwardRequest(t)
	rulesBefore := slices.Clone(req.Rules)
	busyBefore := slices.Clone(req.Busy)
	fromBefore, toBefore, nowBefore := req.From, req.To, req.Now

	mustSlots(t, req)

	if !slices.Equal(req.Rules, rulesBefore) {
		t.Error("Slots rewrote the rules it was given")
	}
	if !slices.Equal(req.Busy, busyBefore) {
		t.Error("Slots rewrote the busy intervals it was given")
	}
	if !req.From.Equal(fromBefore) || !req.To.Equal(toBefore) || !req.Now.Equal(nowBefore) {
		t.Error("Slots rewrote the window it was given")
	}
}

// Every slot the engine emits must satisfy every rule that produced it. Stated as
// properties rather than as a fixed expected list, so the assertion survives a change to
// the fixture.
func TestEverySlotSatisfiesEveryBound(t *testing.T) {
	req := awkwardRequest(t)
	got := mustSlots(t, req)

	earliest := req.Now.Add(req.Params.MinimumNotice)
	latest := req.Now.Add(req.Params.Horizon)
	margin := req.Params.BufferBefore + req.Params.BufferAfter

	for i, s := range got.Slots {
		if d := s.End.Sub(s.Start); d != req.Params.Duration {
			t.Errorf("slot %d lasts %v, want %v", i, d, req.Params.Duration)
		}
		if s.Start.Before(earliest) {
			t.Errorf("slot %d at %v is inside the notice period", i, s.Start)
		}
		if s.Start.After(latest) {
			t.Errorf("slot %d at %v is past the horizon", i, s.Start)
		}
		if day := s.Start.In(req.MentorZone); day.Month() == time.September && day.Day() == 16 {
			t.Errorf("slot %d at %v falls on a closed day", i, s.Start)
		}
		for _, b := range req.Busy {
			widened := Interval{Start: b.Start.Add(-margin), End: b.End.Add(margin)}
			if s.Overlaps(widened) {
				t.Errorf("slot %d at %v overlaps busy time widened by buffers", i, s.Start)
			}
		}
		if i > 0 && !got.Slots[i-1].Start.Before(s.Start) {
			t.Errorf("slot %d is not after slot %d", i, i-1)
		}
	}
}
