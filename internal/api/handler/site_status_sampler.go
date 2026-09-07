package handler

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
)

// StartSiteStatusSampler starts a background sampler that periodically
// records the site's own current status (the same computation
// currentSiteHealth uses for the live GET /api/v1/status read) into
// site_status_daily, so the /status page can render a day-by-day history
// strip. It does its first sample immediately rather than waiting out the
// first interval, mirroring cmd/server's existing startSuggestRefresh. It
// returns immediately; the sampler stops when ctx is done.
//
// Both blue and green processes call this independently against the same
// shared table — deliberate, not a bug: RecordSiteStatusSample's upsert
// takes the worse of the stored and new severity, so which process's write
// lands first or last for a given tick round never loses a real problem.
func StartSiteStatusSampler(ctx context.Context, pool *pgxpool.Pool, queries *db.Queries, interval time.Duration) {
	sample := func() {
		site, _ := currentSiteHealth(ctx, pool)
		if err := queries.RecordSiteStatusSample(ctx, severityFromStatus(site.Status)); err != nil {
			log.Printf("site-status sampler: record: %v", err)
		}
	}
	go func() {
		sample()
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				sample()
			}
		}
	}()
}
