package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/redis/go-redis/v9"

	"github.com/strelov1/freehire/internal/ingest/catalogstats"
	"github.com/strelov1/freehire/internal/ingest/telegram"
	"github.com/strelov1/freehire/internal/platform/cache"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
	"github.com/strelov1/freehire/internal/search/search"
)

// publishSnapshot measures the catalogue and publishes the figures every public surface
// quotes (internal/ingest/catalogstats). It lives here because this worker already scans
// jobs for the rollups.
//
// The error is returned, not acted on: the rollups have already committed, and a failed
// snapshot does not fail the run. The open-jobs figure is also recorded as the day's
// catalogue size (catalog_size_daily), only once the snapshot is stored.
func publishSnapshot(ctx context.Context, counts catalogstats.ExactCounter, c cache.Cache, history sizeRecorder, telegramChannels int, uniqueOpenJobs int64) error {
	snapshot, err := catalogstats.Compute(ctx, counts, telegramChannels, uniqueOpenJobs)
	if err != nil {
		return err
	}
	if err := catalogstats.Store(ctx, c, snapshot); err != nil {
		return fmt.Errorf("publishing the catalogue snapshot: %w", err)
	}
	if err := history.RecordCatalogSize(ctx, db.RecordCatalogSizeParams{
		Day:        pgtype.Date{Time: snapshot.ComputedAt.Truncate(24 * time.Hour), Valid: true},
		OpenJobs:   snapshot.OpenJobs,
		ComputedAt: pgconv.Instant(snapshot.ComputedAt),
	}); err != nil {
		return fmt.Errorf("the snapshot is published, but recording the day's catalogue size failed: %w", err)
	}
	return nil
}

// sizeRecorder stores one day's measured catalogue size. The history must be written
// through the writer (*db.Queries), not the analytics replica the counts come from.
type sizeRecorder interface {
	RecordCatalogSize(ctx context.Context, arg db.RecordCatalogSizeParams) error
}

// snapshotCache builds the shared cache the snapshot is published to. A malformed
// REDIS_URL is reported, not fatal: this worker's primary job needs no cache at all.
func snapshotCache(redisURL string) (cache.Cache, func(), error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, nil, fmt.Errorf("redis: %w", err)
	}
	client := redis.NewClient(opts)
	return cache.NewRedisCache(client), func() { _ = client.Close() }, nil
}

// configuredTelegramChannels counts the channels the crawler is configured to read.
// catalogstats takes the count because the channel list is not part of the catalogue it
// measures.
func configuredTelegramChannels(ctx context.Context, q telegram.ChannelLister) (int, error) {
	cfg, err := telegram.LoadChannels(ctx, q)
	if err != nil {
		return 0, err
	}
	return len(cfg.Channels), nil
}

// uniqueOpenJobs reads Meilisearch's count of de-duplicated open postings: an unfiltered,
// one-hit search whose `Total` is the same field `/jobs/search` returns for zero filters.
// catalogstats takes the count because a live index is not part of what Compute measures
// from Postgres.
func uniqueOpenJobs(ctx context.Context, client *search.Client) (int64, error) {
	res, err := client.Search(ctx, search.SearchParams{Limit: 1})
	if err != nil {
		return 0, err
	}
	return res.Total, nil
}

// resolveUniqueOpenJobs decides the figure this run publishes: fresh when this run's
// Meilisearch call succeeded, otherwise previous. A transient outage must not overwrite a
// real count with a zero, which would read as "no unique jobs" instead of "not measured".
func resolveUniqueOpenJobs(previous, fresh int64, err error) int64 {
	if err != nil {
		return previous
	}
	return fresh
}
