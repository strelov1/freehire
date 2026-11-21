package viewlog

import (
	"context"
	"time"

	"github.com/strelov1/freehire/internal/platform/cache"
)

// totalsKey is where cmd/rollup-stats publishes the site-wide page-view totals the public
// engagement endpoint quotes. One key, read by every API process.
const totalsKey = "viewlog:totals:v1"

// totalsTTL outlives the worker's schedule (several runs a day) by a wide margin, so a
// missed run serves yesterday's figure rather than none; ComputedAt says how old it is.
const totalsTTL = 7 * 24 * time.Hour

// totalsReadTimeout bounds one cache read, so an unreachable Redis costs the snapshot,
// not the response.
const totalsReadTimeout = 100 * time.Millisecond

// Totals is the published sum of job_daily_views.page_uniques and the first day it
// carries a count (db.GetPageViewTotals). Summing the rollup scans every row of a table
// that grows by a row per posting per day, so it is measured off the request path.
type Totals struct {
	Viewed int64 `json:"viewed"`
	// ViewedSince is nil while nothing has been rolled up.
	ViewedSince *time.Time `json:"viewed_since"`
	ComputedAt  time.Time  `json:"computed_at"`
}

// StoreTotals publishes the totals for every reader.
func StoreTotals(ctx context.Context, c cache.Cache, t Totals) error {
	return cache.SetJSON(ctx, c, totalsKey, t, totalsTTL)
}

// LoadTotals reads the published totals. found is false with no cache, a cold one, or
// one that could not be read; the caller decides what to serve then.
func LoadTotals(ctx context.Context, c cache.Cache) (Totals, bool, error) {
	if c == nil {
		return Totals{}, false, nil
	}
	ctx, cancel := context.WithTimeout(ctx, totalsReadTimeout)
	defer cancel()
	return cache.GetJSON[Totals](ctx, c, totalsKey)
}
