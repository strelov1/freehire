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
