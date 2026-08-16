package catalogstats

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/cache"
)

// countingEstimator records how often the approximate path was taken.
type countingEstimator struct {
	value int64
	err   error
	calls int
}

func (e *countingEstimator) EstimateOpenJobs(context.Context) (int64, error) {
	e.calls++
	return e.value, e.err
}

// recordingCache captures what Store asked for, so the retention decision can be
// asserted rather than left in a constant nobody reads.
type recordingCache struct {
	cache.Cache
	key string
	ttl time.Duration
}

func (r *recordingCache) Set(ctx context.Context, key string, val []byte, ttl time.Duration) error {
	r.key, r.ttl = key, ttl
	return r.Cache.Set(ctx, key, val, ttl)
}

// The snapshot must outlive the worker's schedule by a wide margin. A TTL near the cron
// interval means one skipped or slow run drops every surface back to the estimate; the
// whole point of publishing an exact figure is that a missed run degrades to
// stale-but-exact instead of fresh-but-wrong.
func TestStoreRetainsTheSnapshotBeyondTheWorkerSchedule(t *testing.T) {
	rec := &recordingCache{Cache: cache.NewMemory()}

	if err := Store(context.Background(), rec, storedSnapshot()); err != nil {
		t.Fatalf("Store: %v", err)
	}

	// rollup-stats runs intra-day, every few hours. Anything under half a day would
	// make a single missed run visible to users.
	const workerSchedule = 12 * time.Hour
	if rec.ttl <= workerSchedule {
		t.Errorf("Store TTL = %s, want comfortably more than the %s worker schedule — "+
			"a skipped run would otherwise drop every surface back to the estimate",
			rec.ttl, workerSchedule)
	}
	if rec.key != snapshotKey {
		t.Errorf("Store wrote %q, want %q — readers look under one shared key", rec.key, snapshotKey)
	}
}

// brokenCache reports a backend failure on every operation.
type brokenCache struct{}

func (brokenCache) Get(context.Context, string) ([]byte, bool, error) {
	return nil, false, errors.New("backend unreachable")
}
func (brokenCache) Set(context.Context, string, []byte, time.Duration) error {
	return errors.New("backend unreachable")
}

func storedSnapshot() Snapshot {
	return Snapshot{
		OpenJobs:         3_300_658,
		Companies:        294_282,
		Sources:          Sources(),
		ATSPlatforms:     ATSPlatforms(),
		TelegramChannels: 95,
		ComputedAt:       time.Unix(1_700_000_000, 0).UTC(),
	}
}

func TestLoadReturnsTheStoredSnapshot(t *testing.T) {
	c := cache.NewMemory()
	ctx := context.Background()
	want := storedSnapshot()

	if err := Store(ctx, c, want); err != nil {
		t.Fatalf("Store: %v", err)
	}

	est := &countingEstimator{value: 999}
	got := Load(ctx, c, est)

	if !got.Exact {
		t.Error("Exact = false for a published snapshot")
	}
	if got.Snapshot != want {
		t.Errorf("Snapshot = %+v, want %+v", got.Snapshot, want)
	}
	if est.calls != 0 {
		t.Errorf("the estimator was called %d times despite a cache hit", est.calls)
	}
}

func TestLoadDegradesOnAnEmptyCache(t *testing.T) {
	ctx := context.Background()
	est := &countingEstimator{value: 3_150_000}

	got := Load(ctx, cache.NewMemory(), est)

	if got.Exact {
		t.Error("Exact = true with nothing published — consumers cannot tell an estimate from a count")
	}
	if got.OpenJobs != 3_150_000 {
		t.Errorf("OpenJobs = %d, want the estimate 3150000", got.OpenJobs)
	}
	if est.calls != 1 {
		t.Errorf("estimator calls = %d, want 1", est.calls)
	}
	// Registry-derived figures cost nothing and need no cache, so a degraded read
	// should still carry them rather than reporting a catalogue with no sources.
	if got.Sources != Sources() || got.ATSPlatforms != ATSPlatforms() {
		t.Errorf("Sources/ATSPlatforms = %d/%d on the degraded path, want %d/%d",
			got.Sources, got.ATSPlatforms, Sources(), ATSPlatforms())
	}
}

// Running the API without Redis is a supported deployment, not a misconfiguration.
func TestLoadDegradesWithNoCacheConfigured(t *testing.T) {
	est := &countingEstimator{value: 3_150_000}

	got := Load(context.Background(), nil, est)

	if got.Exact {
		t.Error("Exact = true with no cache configured")
	}
	if got.OpenJobs != 3_150_000 {
		t.Errorf("OpenJobs = %d, want the estimate", got.OpenJobs)
	}
}

func TestLoadDegradesOnAnUnreachableCache(t *testing.T) {
	est := &countingEstimator{value: 3_150_000}

	got := Load(context.Background(), brokenCache{}, est)

	if got.Exact {
		t.Error("Exact = true against an unreachable cache")
	}
	if got.OpenJobs != 3_150_000 {
		t.Errorf("OpenJobs = %d, want the estimate", got.OpenJobs)
	}
}

// Both fallbacks failing is a degraded read, not a failed one: the caller is serving a
// page, and a missing figure beats a 500.
func TestLoadSurvivesTheEstimatorFailing(t *testing.T) {
	est := &countingEstimator{err: errors.New("database down")}

	got := Load(context.Background(), brokenCache{}, est)

	if got.Exact {
		t.Error("Exact = true with neither a snapshot nor an estimate")
	}
	if got.OpenJobs != 0 {
		t.Errorf("OpenJobs = %d, want 0 when no figure could be obtained", got.OpenJobs)
	}
}

// Load never recomputes, and that is enforced by its signature rather than by a test:
// it takes no ExactCounter, so no read path can reach the catalogue-wide scan even by
// mistake. A runtime assertion would be the weaker guarantee.

func TestLoadTreatsAnUndecodableSnapshotAsAMiss(t *testing.T) {
	c := cache.NewMemory()
	ctx := context.Background()

	if err := c.Set(ctx, snapshotKey, []byte(`{"open_jobs": "not a number"}`), time.Hour); err != nil {
		t.Fatalf("Set: %v", err)
	}

	est := &countingEstimator{value: 3_150_000}
	got := Load(ctx, c, est)

	if got.Exact {
		t.Error("Exact = true for an undecodable payload — a half-filled snapshot would be published")
	}
	if got.OpenJobs != 3_150_000 {
		t.Errorf("OpenJobs = %d, want the estimate", got.OpenJobs)
	}
}
