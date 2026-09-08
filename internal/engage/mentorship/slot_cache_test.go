package mentorship

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/platform/cache"
)

// countingCache is an in-memory cache that records how it was used, and can be made to
// fail either half — which is the only way to test that a read never fails because a
// cache is unavailable.
type countingCache struct {
	entries map[string][]byte
	gets    int
	sets    int
	getErr  error
	setErr  error
}

func newCountingCache() *countingCache {
	return &countingCache{entries: map[string][]byte{}}
}

func (c *countingCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	c.gets++
	if c.getErr != nil {
		return nil, false, c.getErr
	}
	val, ok := c.entries[key]
	return val, ok, nil
}

func (c *countingCache) Set(_ context.Context, key string, val []byte, _ time.Duration) error {
	c.sets++
	if c.setErr != nil {
		return c.setErr
	}
	c.entries[key] = val
	return nil
}

// Compile-time proof the fake satisfies the interface the service takes.
var _ cache.Cache = (*countingCache)(nil)

// cachedMentor is an approved mentor with a schedule, wired to a countable repository so
// a test can tell a cache hit from a recomputation.
func cachedMentor(t *testing.T, repo *fakeRepo) Profile {
	t.Helper()
	profile := bookableMentor(t, repo)
	return profile
}

func cachingService(repo *fakeRepo, c cache.Cache) *Service {
	return New(repo, Config{
		Cache: c,
		Now: func() time.Time {
			return time.Date(2026, time.September, 7, 9, 0, 0, 0, time.UTC)
		},
	})
}

func slotWindowFor(t *testing.T) (from, to time.Time) {
	t.Helper()
	zone := berlin(t)
	return time.Date(2026, time.September, 8, 0, 0, 0, 0, zone),
		time.Date(2026, time.September, 9, 0, 0, 0, 0, zone)
}

// The second read of one window is served from the cache rather than recomputed.
func TestASecondReadOfAWindowIsServedFromTheCache(t *testing.T) {
	repo := newFakeRepo()
	mentor := cachedMentor(t, repo)
	store := newCountingCache()
	svc := cachingService(repo, store)
	from, to := slotWindowFor(t)

	first, err := svc.CachedMentorSlots(context.Background(), mentor.Slug, from, to, "Europe/Berlin")
	if err != nil {
		t.Fatalf("first read: %v", err)
	}
	if store.sets != 1 {
		t.Errorf("the first read wrote %d cache entries, want 1", store.sets)
	}

	second, err := svc.CachedMentorSlots(context.Background(), mentor.Slug, from, to, "Europe/Berlin")
	if err != nil {
		t.Fatalf("second read: %v", err)
	}
	if store.sets != 1 {
		t.Errorf("the second read wrote %d entries in total, want the first one only", store.sets)
	}
	if len(first.Slots) == 0 || len(second.Slots) != len(first.Slots) {
		t.Fatalf("cached read returned %d slots, computed one %d", len(second.Slots), len(first.Slots))
	}
	for i := range first.Slots {
		if !second.Slots[i].Start.Equal(first.Slots[i].Start) {
			t.Errorf("slot %d differs: %v vs %v", i, second.Slots[i].Start, first.Slots[i].Start)
		}
	}
}

// The requirement the cache key exists for: the stored value carries wall-clock times
// already rendered in somebody's zone, so serving a Berlin entry to a viewer in Tokyo
// would show times that are wrong and look entirely plausible.
func TestTwoViewersInDifferentZonesDoNotShareACacheEntry(t *testing.T) {
	repo := newFakeRepo()
	mentor := cachedMentor(t, repo)
	store := newCountingCache()
	svc := cachingService(repo, store)
	from, to := slotWindowFor(t)

	berlinRead, err := svc.CachedMentorSlots(context.Background(), mentor.Slug, from, to, "Europe/Berlin")
	if err != nil {
		t.Fatalf("Berlin read: %v", err)
	}
	tokyoRead, err := svc.CachedMentorSlots(context.Background(), mentor.Slug, from, to, "Asia/Tokyo")
	if err != nil {
		t.Fatalf("Tokyo read: %v", err)
	}

	if store.sets != 2 {
		t.Errorf("two zones produced %d cache entries, want 2", store.sets)
	}
	if berlinRead.Zone != "Europe/Berlin" || tokyoRead.Zone != "Asia/Tokyo" {
		t.Fatalf("zones came back as %q and %q", berlinRead.Zone, tokyoRead.Zone)
	}
	if len(berlinRead.Slots) == 0 || len(tokyoRead.Slots) == 0 {
		t.Fatal("one of the reads returned nothing")
	}

	// Same instants, different wall clocks — which is exactly what a shared entry would
	// have destroyed.
	if !tokyoRead.Slots[0].Start.Equal(berlinRead.Slots[0].Start) {
		t.Error("the two zones disagree about which instant the first slot is")
	}
	if tokyoRead.Slots[0].Start.Hour() == berlinRead.Slots[0].Start.Hour() {
		t.Error("both zones rendered the same wall-clock hour — one of them is somebody else's")
	}
}

// A cached entry must come back in the zone it was stored for. JSON round-trips a
// time.Time as an OFFSET rather than a zone name, so a naive implementation returns
// +02:00 instead of Europe/Berlin and the next daylight-saving transition renders wrong.
func TestACachedEntryComesBackInItsOwnZone(t *testing.T) {
	repo := newFakeRepo()
	mentor := cachedMentor(t, repo)
	store := newCountingCache()
	svc := cachingService(repo, store)
	from, to := slotWindowFor(t)

	if _, err := svc.CachedMentorSlots(context.Background(), mentor.Slug, from, to, "Asia/Tokyo"); err != nil {
		t.Fatalf("first read: %v", err)
	}
	cached, err := svc.CachedMentorSlots(context.Background(), mentor.Slug, from, to, "Asia/Tokyo")
	if err != nil {
		t.Fatalf("cached read: %v", err)
	}

	if cached.Zone != "Asia/Tokyo" {
		t.Errorf("Zone = %q, want Asia/Tokyo", cached.Zone)
	}
	if len(cached.Slots) == 0 {
		t.Fatal("no slots")
	}
	if name := cached.Slots[0].Start.Location().String(); name != "Asia/Tokyo" {
		t.Errorf("slot location = %q, want Asia/Tokyo — a stored offset is not a zone", name)
	}
}

// A read never fails because a cache is unavailable — the rule catalogstats already
// follows. Both halves are tested: a failing Get must fall through to computing, and a
// failing Set must not fail the read that produced the value.
func TestAnUnavailableCacheDegradesRatherThanFailing(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setUp func(*countingCache)
	}{
		{"the read fails", func(c *countingCache) { c.getErr = errors.New("connection refused") }},
		{"the write fails", func(c *countingCache) { c.setErr = errors.New("connection refused") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepo()
			mentor := cachedMentor(t, repo)
			store := newCountingCache()
			tc.setUp(store)
			svc := cachingService(repo, store)
			from, to := slotWindowFor(t)

			got, err := svc.CachedMentorSlots(context.Background(), mentor.Slug, from, to, "Europe/Berlin")
			if err != nil {
				t.Fatalf("CachedMentorSlots: %v — a read must not fail because a cache is down", err)
			}
			if len(got.Slots) == 0 {
				t.Error("the degraded read returned no slots")
			}
		})
	}
}

// With no cache configured at all the endpoint still works — that is how the feature
// ships before Redis is wired for it, and what rolling the cache back looks like.
func TestWithNoCacheTheReadStillWorks(t *testing.T) {
	repo := newFakeRepo()
	mentor := cachedMentor(t, repo)
	svc := New(repo, Config{Now: func() time.Time {
		return time.Date(2026, time.September, 7, 9, 0, 0, 0, time.UTC)
	}})
	from, to := slotWindowFor(t)

	got, err := svc.CachedMentorSlots(context.Background(), mentor.Slug, from, to, "Europe/Berlin")
	if err != nil {
		t.Fatalf("CachedMentorSlots: %v", err)
	}
	if len(got.Slots) == 0 {
		t.Error("no slots without a cache")
	}
}

// A cache entry for an unpublished mentor must never be served: the publication check
// happens BEFORE the cache is consulted, so pausing takes effect immediately rather than
// after the entry expires.
func TestAPausedMentorIsNotServedFromTheCache(t *testing.T) {
	repo := newFakeRepo()
	mentor := cachedMentor(t, repo)
	store := newCountingCache()
	svc := cachingService(repo, store)
	from, to := slotWindowFor(t)

	if _, err := svc.CachedMentorSlots(context.Background(), mentor.Slug, from, to, "Europe/Berlin"); err != nil {
		t.Fatalf("first read: %v", err)
	}
	if _, err := repo.SetPaused(context.Background(), mentor.UserID, true); err != nil {
		t.Fatalf("SetPaused: %v", err)
	}

	if _, err := svc.CachedMentorSlots(context.Background(), mentor.Slug, from, to, "Europe/Berlin"); !errors.Is(err, ErrProfileNotFound) {
		t.Errorf("error = %v, want ErrProfileNotFound — a paused mentor's cached window was served", err)
	}
}
