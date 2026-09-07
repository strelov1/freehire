package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/ingest/catalogstats"
	"github.com/strelov1/freehire/internal/platform/cache"
	"github.com/strelov1/freehire/internal/platform/db"
)

type stubCounter struct {
	row db.CountCatalogueScaleRow
	err error
}

func (s stubCounter) CountCatalogueScale(context.Context) (db.CountCatalogueScaleRow, error) {
	return s.row, s.err
}

func TestPublishSnapshotStoresAReadableSnapshot(t *testing.T) {
	c := cache.NewMemory()
	ctx := context.Background()
	counts := stubCounter{row: db.CountCatalogueScaleRow{OpenJobs: 3_300_658, Companies: 294_282}}

	if err := publishSnapshot(ctx, counts, c, 95, 2_075_865); err != nil {
		t.Fatalf("publishSnapshot: %v", err)
	}

	got := catalogstats.Load(ctx, c, failingEstimator{})
	if !got.Exact {
		t.Fatal("Load reports a degraded snapshot right after publishing one")
	}
	if got.OpenJobs != 3_300_658 || got.Companies != 294_282 {
		t.Errorf("OpenJobs/Companies = %d/%d, want 3300658/294282", got.OpenJobs, got.Companies)
	}
	if got.TelegramChannels != 95 {
		t.Errorf("TelegramChannels = %d, want the 95 passed in", got.TelegramChannels)
	}
	if got.UniqueOpenJobs != 2_075_865 {
		t.Errorf("UniqueOpenJobs = %d, want the 2075865 passed in", got.UniqueOpenJobs)
	}
}

// The rollups are this worker's primary job and commit in their own transaction. A
// snapshot that cannot be computed or stored is worth logging, not worth failing a run
// that already did its work — so the failure must arrive as a value the caller chooses
// what to do with, not as a panic or a process exit.
func TestPublishSnapshotReportsFailureWithoutPanicking(t *testing.T) {
	ctx := context.Background()

	t.Run("counting fails", func(t *testing.T) {
		err := publishSnapshot(ctx, stubCounter{err: errors.New("scan failed")}, cache.NewMemory(), 95, 2_075_865)
		if err == nil {
			t.Error("publishSnapshot returned nil when the count failed — the caller has nothing to log")
		}
	})

	t.Run("storing fails", func(t *testing.T) {
		counts := stubCounter{row: db.CountCatalogueScaleRow{OpenJobs: 1, Companies: 1}}
		err := publishSnapshot(ctx, counts, unwritableCache{}, 95, 2_075_865)
		if err == nil {
			t.Error("publishSnapshot returned nil when the store failed")
		}
	})
}

// A transient Meilisearch outage on one run must not overwrite a real,
// previously-published unique-jobs count with a bare zero: that would read as "no
// unique jobs" rather than "not measured this run" — the same "stale but exact
// beats fresh but wrong" trade-off catalogstats.Load already makes for the whole
// snapshot (see load.go's snapshotTTL comment).
func TestResolveUniqueOpenJobsKeepsThePreviousFigureOnFailure(t *testing.T) {
	got := resolveUniqueOpenJobs(2_075_865, 0, errors.New("meilisearch unreachable"))
	if got != 2_075_865 {
		t.Errorf("resolveUniqueOpenJobs = %d, want the previous 2075865 kept on failure", got)
	}
}

func TestResolveUniqueOpenJobsUsesTheFreshFigureOnSuccess(t *testing.T) {
	got := resolveUniqueOpenJobs(2_075_865, 2_080_000, nil)
	if got != 2_080_000 {
		t.Errorf("resolveUniqueOpenJobs = %d, want the fresh 2080000 on success", got)
	}
}

type failingEstimator struct{}

func (failingEstimator) EstimateOpenJobs(context.Context) (int64, error) {
	return 0, errors.New("the estimate must not be reached when a snapshot exists")
}

type unwritableCache struct{ cache.Cache }

func (unwritableCache) Get(context.Context, string) ([]byte, bool, error) { return nil, false, nil }
func (unwritableCache) Set(context.Context, string, []byte, time.Duration) error {
	return errors.New("backend unreachable")
}
