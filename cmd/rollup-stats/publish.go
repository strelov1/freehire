package main

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/strelov1/freehire/internal/ingest/catalogstats"
	"github.com/strelov1/freehire/internal/ingest/telegram"
	"github.com/strelov1/freehire/internal/platform/cache"
)

// publishSnapshot measures the catalogue and publishes the figures every public surface
// quotes (internal/ingest/catalogstats).
//
// It lives in this worker because the exact counts are a full catalogue scan and this
// worker is already scanning jobs for the rollups — one more aggregate on a run that is
// already doing heavier work, against a new cron unit that would cost real ops surface.
//
// The error is returned rather than acted on: the rollups are this worker's primary job
// and have already committed by the time this runs, so whether a failed snapshot should
// fail the run is the caller's decision, and the caller's answer is no.
func publishSnapshot(ctx context.Context, counts catalogstats.ExactCounter, c cache.Cache, telegramChannels int) error {
	snapshot, err := catalogstats.Compute(ctx, counts, telegramChannels)
	if err != nil {
		return err
	}
	if err := catalogstats.Store(ctx, c, snapshot); err != nil {
		return fmt.Errorf("publishing the catalogue snapshot: %w", err)
	}
	return nil
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
// catalogstats takes the count rather than finding it itself: the channel list is not
// part of the catalogue it measures, and reaching for it there would put a second,
// unrelated query behind Compute.
func configuredTelegramChannels(ctx context.Context, q telegram.ChannelLister) (int, error) {
	cfg, err := telegram.LoadChannels(ctx, q)
	if err != nil {
		return 0, err
	}
	return len(cfg.Channels), nil
}
