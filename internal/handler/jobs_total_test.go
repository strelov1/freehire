package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/cache"
	"github.com/strelov1/freehire/internal/catalogstats"
)

// The list's meta.total is the most-quoted number freehire publishes — the /about strip,
// the /open page and every API consumer read it. It must be the exact published count
// when one exists, and must still answer when nothing does.
func TestOpenJobTotal_PrefersThePublishedSnapshot(t *testing.T) {
	c := cache.NewMemory()
	ctx := context.Background()
	if err := catalogstats.Store(ctx, c, publishedSnapshot()); err != nil {
		t.Fatalf("Store: %v", err)
	}

	h := &jobsHandlers{cache: c, estimator: stubEstimator{value: 9_999_999}}

	if got := h.openJobTotal(ctx); got != 3_300_658 {
		t.Errorf("openJobTotal = %d, want the snapshot's exact 3300658, not the estimate", got)
	}
}

func TestOpenJobTotal_FallsBackToTheEstimate(t *testing.T) {
	h := &jobsHandlers{cache: cache.NewMemory(), estimator: stubEstimator{value: 3_150_000}}

	if got := h.openJobTotal(context.Background()); got != 3_150_000 {
		t.Errorf("openJobTotal = %d, want the estimate 3150000", got)
	}
}

// Before this change a failing count returned 500 and took the whole page of jobs with
// it. The page is the endpoint's actual payload; a missing total is not worth losing it.
func TestOpenJobTotal_SurvivesTheEstimatorFailing(t *testing.T) {
	h := &jobsHandlers{cache: cache.NewMemory(), estimator: stubEstimator{err: errors.New("database down")}}

	if got := h.openJobTotal(context.Background()); got != 0 {
		t.Errorf("openJobTotal = %d, want 0 when no figure is obtainable", got)
	}
}
