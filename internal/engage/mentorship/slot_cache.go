package mentorship

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/engage/slots"
	"github.com/strelov1/freehire/internal/platform/cache"
)

// slotCacheTTL is how long a computed window is reused.
// One minute, chosen by what a stale entry costs: Book never reads this cache, so the worst case is
// an hour offered up to a minute after it was taken, refused with the ordinary "no longer
// available". Meanwhile a minute collapses a crawler walking a mentor's calendar into one computation.
const slotCacheTTL = time.Minute

// slotCacheKey identifies one computed window.
// The VIEWER'S ZONE is part of the key: the cached value carries wall-clock times already
// rendered in a zone, so another zone's entry would show plausible but wrong times. The window
// is keyed by its exact bounds, not rounded to a month, since the endpoint takes arbitrary bounds.
func slotCacheKey(mentorSlug, viewerZone string, from, to time.Time) string {
	return fmt.Sprintf("mentorslots:v1:%s:%s:%d:%d",
		mentorSlug, viewerZone, from.UTC().Unix(), to.UTC().Unix())
}

// cachedSlots is the stored shape. JSON round-trips Interval's times as an offset, not a zone
// name, so the zone is stored beside them and the values are re-expressed on read.
type cachedSlots struct {
	Zone  string      `json:"zone"`
	Slots []time.Time `json:"slots"`
	Ends  []time.Time `json:"ends"`
}

// cachedMentorSlots serves a window from the cache when it can and computes it otherwise.
// There is NO explicit invalidation, by decision: the shared cache interface has no Delete, and
// a booking would have to invalidate every overlapping window in every viewer's zone, a set the
// writer cannot enumerate. The one-minute TTL bounds staleness instead (see slotCacheTTL).
//
// A cache that is down or misbehaving degrades to computing directly.
func (s *Service) CachedMentorSlots(ctx context.Context, slug string, from, to time.Time, viewerZone string) (slots.SlotResult, error) {
	// The publication check runs BEFORE the cache: reading the cache first would keep a paused
	// mentor bookable until their entries expire. The check is one indexed read; the cache saves
	// the slot computation, the expensive half.
	mentor, err := s.PublicProfile(ctx, slug)
	if err != nil {
		return slots.SlotResult{}, err
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

func toCachedSlots(r slots.SlotResult) cachedSlots {
	out := cachedSlots{Zone: r.Zone, Slots: make([]time.Time, 0, len(r.Slots)), Ends: make([]time.Time, 0, len(r.Slots))}
	for _, s := range r.Slots {
		out.Slots = append(out.Slots, s.Start)
		out.Ends = append(out.Ends, s.End)
	}
	return out
}

// toResult re-expresses the stored instants in the stored zone. A zone that no longer loads
// makes the entry unusable, and the caller recomputes.
func (c cachedSlots) toResult() (slots.SlotResult, bool) {
	if len(c.Slots) != len(c.Ends) {
		return slots.SlotResult{}, false
	}
	zone, err := time.LoadLocation(c.Zone)
	if err != nil {
		return slots.SlotResult{}, false
	}
	intervals := make([]slots.Interval, 0, len(c.Slots))
	for i := range c.Slots {
		intervals = append(intervals, slots.Interval{Start: c.Slots[i].In(zone), End: c.Ends[i].In(zone)})
	}
	return slots.SlotResult{Slots: intervals, Zone: c.Zone}, true
}
