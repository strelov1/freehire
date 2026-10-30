package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/platform/cache"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/embedprogress"
)

type stubCoverage struct {
	row   db.SemanticCoverageCountsRow
	err   error
	depth int64
}

func (s stubCoverage) SemanticOutboxClaimableDepth(context.Context) (int64, error) {
	return s.depth, nil
}

func (s stubCoverage) SemanticCoverageCounts(context.Context, string) (db.SemanticCoverageCountsRow, error) {
	return s.row, s.err
}

var measuredAt = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

func TestPublishCoverageStoresTheSnapshot(t *testing.T) {
	ctx := context.Background()
	c := cache.NewMemory()

	if err := publishCoverage(ctx, stubCoverage{row: db.SemanticCoverageCountsRow{Eligible: 1_900_000, Covered: 1_850_000}, depth: 60_000}, c, func() time.Time { return measuredAt }); err != nil {
		t.Fatalf("publishCoverage: %v", err)
	}
	got, ok := embedprogress.Load(ctx, c)
	if !ok || got.Eligible != 1_900_000 || got.Covered != 1_850_000 || got.QueueDepth == nil || *got.QueueDepth != 60_000 || !got.MeasuredAt.Equal(measuredAt) {
		t.Errorf("published = %+v (ok %v)", got, ok)
	}
}

func TestPublishCoverageKeepsThePreviousSnapshotOnFailure(t *testing.T) {
	ctx := context.Background()
	c := cache.NewMemory()
	previous := embedprogress.Snapshot{Eligible: 10, Covered: 5, MeasuredAt: measuredAt}
	if err := embedprogress.Store(ctx, c, previous); err != nil {
		t.Fatal(err)
	}

	if err := publishCoverage(ctx, stubCoverage{err: errors.New("canceling statement due to statement timeout")}, c, time.Now); err == nil {
		t.Error("a failed count reported success")
	}
	if got, _ := embedprogress.Load(ctx, c); got != previous {
		t.Errorf("snapshot after a failed count = %+v, want the previous %+v kept", got, previous)
	}
}
