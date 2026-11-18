package handler

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
)

// StartSiteStatusSampler starts a background sampler that periodically records the site's
// own current status (the computation behind GET /api/v1/status) into site_status_daily,
// for the /status history strip. It samples immediately rather than waiting out the first
// interval, returns immediately, and stops when ctx is done.
//
// Blue and green processes both call this against the same table, deliberately:
// RecordSiteStatusSample's upsert keeps the worse of the stored and new severity, so write
// order never loses a real problem.
func StartSiteStatusSampler(ctx context.Context, pool *pgxpool.Pool, queries *db.Queries, interval time.Duration) {
	sample := func() {
		site, _ := currentSiteHealth(ctx, pool)
		if err := queries.RecordSiteStatusSample(ctx, severityFromStatus(providerStatus(site.Status))); err != nil {
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
