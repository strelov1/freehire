package mentorship

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/platform/cache"
)

// slotCacheTTL is how long a computed window is reused.
//
// One minute, and the figure follows from what a stale entry COSTS rather than from what
// it saves. A booking never reads this cache — Book re-derives from the live schedule —
// so the worst a stale window does is offer an hour that was taken up to a minute ago,
// and the seeker who clicks it gets the ordinary "no longer available" refusal. Against
// that: this is a computed endpoint on a host where most traffic is crawlers, and a
// minute collapses a robot walking a mentor's calendar into one computation.
const slotCacheTTL = time.Minute

// slotCacheKey identifies one computed window.
//
// The VIEWER'S ZONE is part of the key, and leaving it out is the bug this names: the
// cached value carries wall-clock times already rendered in somebody's zone, so a visitor
// in Tokyo served Berlin's entry sees times that are wrong and look entirely plausible.
// The window is keyed by its bounds rather than rounded to a month, because the endpoint
// takes arbitrary bounds and a rounded key would answer a different question than the one
// asked.
func slotCacheKey(mentorSlug, viewerZone string, from, to time.Time) string {
	return fmt.Sprintf("mentorslots:v1:%s:%s:%d:%d",
		mentorSlug, viewerZone, from.UTC().Unix(), to.UTC().Unix())
}

// cachedSlots is the stored shape. Interval's times carry a zone, which JSON round-trips
// as an offset rather than a zone name — so the zone is stored beside them and the values
// are re-expressed on read, instead of being trusted to come back in the zone they went
// in as.
type cachedSlots struct {
	Zone  string      `json:"zone"`
	Slots []time.Time `json:"slots"`
	Ends  []time.Time `json:"ends"`
}

// cachedMentorSlots serves a window from the cache when it can and computes it otherwise.
//
// THERE IS NO EXPLICIT INVALIDATION, and that is a decision rather than an omission. The
// cache interface this repository shares has Get and Set and no Delete, and a booking
// would have to invalidate every window overlapping it in every viewer's zone — a set the
// writer cannot enumerate. A one-minute TTL bounds the staleness instead, and the booking
// path does not read the cache at all, so the only symptom is a slot offered for up to a
// minute after it was taken, refused the moment somebody tries to take it.
//
// A cache that is down or misbehaving degrades to computing directly. A read never fails
// because a cache is unavailable — the same rule catalogstats follows.
func (s *Service) CachedMentorSlots(ctx context.Context, slug string, from, to time.Time, viewerZone string) (SlotResult, error) {
	// The publication check runs BEFORE the cache, always. Reading the cache first would
	// mean a mentor who pauses stays bookable until their entries expire — the pause
	// button would appear to do nothing for a minute, which is exactly when somebody uses
	// it. The check is one indexed read; the cache is here to save the slot COMPUTATION,
	// which is the expensive half.
	mentor, err := s.PublicProfile(ctx, slug)
	if err != nil {
		return SlotResult{}, err
	}
	if s.cache == nil {
		return s.slotsFor(ctx, mentor, from, to, viewerZone)
	}

	key := slotCacheKey(slug, viewerZone, from, to)
	if hit, ok, err := cache.GetJSON[cachedSlots](ctx, s.cache, key); err == nil && ok {
		if result, ok := hit.toResult(); ok {
			return result, nil
		}
	} else if err != nil {
		log.Printf("mentorship: reading the slot cache for %s: %v", slug, err)
	}

	result, err := s.slotsFor(ctx, mentor, from, to, viewerZone)
	if err != nil {
		return result, err
	}
	if err := cache.SetJSON(ctx, s.cache, key, toCachedSlots(result), slotCacheTTL); err != nil {
		log.Printf("mentorship: writing the slot cache for %s: %v", slug, err)
	}
	return result, nil
}

func toCachedSlots(r SlotResult) cachedSlots {
	out := cachedSlots{Zone: r.Zone, Slots: make([]time.Time, 0, len(r.Slots)), Ends: make([]time.Time, 0, len(r.Slots))}
	for _, s := range r.Slots {
		out.Slots = append(out.Slots, s.Start)
		out.Ends = append(out.Ends, s.End)
	}
	return out
}

// toResult re-expresses the stored instants in the stored zone. A zone that no longer
// loads makes the entry unusable rather than wrong — the caller recomputes.
func (c cachedSlots) toResult() (SlotResult, bool) {
	if len(c.Slots) != len(c.Ends) {
		return SlotResult{}, false
	}
	zone, err := time.LoadLocation(c.Zone)
	if err != nil {
		return SlotResult{}, false
	}
	slots := make([]Interval, 0, len(c.Slots))
	for i := range c.Slots {
		slots = append(slots, Interval{Start: c.Slots[i].In(zone), End: c.Ends[i].In(zone)})
	}
	return SlotResult{Slots: slots, Zone: c.Zone}, true
}
