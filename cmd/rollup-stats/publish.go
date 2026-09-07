package main

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/strelov1/freehire/internal/ingest/catalogstats"
	"github.com/strelov1/freehire/internal/ingest/telegram"
	"github.com/strelov1/freehire/internal/platform/cache"
	"github.com/strelov1/freehire/internal/search/search"
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
func publishSnapshot(ctx context.Context, counts catalogstats.ExactCounter, c cache.Cache, telegramChannels int, uniqueOpenJobs int64) error {
	snapshot, err := catalogstats.Compute(ctx, counts, telegramChannels, uniqueOpenJobs)
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

// uniqueOpenJobs reads Meilisearch's own count of de-duplicated open postings: an
// unfiltered, one-hit search, whose `Total` is Meilisearch's estimated total for the
// query — the same field `/jobs/search` itself returns for zero filters. catalogstats
// takes the count rather than finding it itself: a live index isn't part of the
// catalogue Compute measures from Postgres, and reaching for Meilisearch there would
// put an unrelated dependency (and a second failure mode) behind it.
func uniqueOpenJobs(ctx context.Context, client *search.Client) (int64, error) {
	res, err := client.Search(ctx, search.SearchParams{Limit: 1})
	if err != nil {
		return 0, err
	}
	return res.Total, nil
}

// resolveUniqueOpenJobs decides the figure this run publishes: fresh when this
// run's Meilisearch call succeeded, otherwise previous — the figure already
// published by an earlier run. A transient outage must not overwrite a real count
// with a bare zero, which would read as "no unique jobs" rather than "not measured
// this run"; see uniqueOpenJobs' call site for the same reasoning Load already
// applies to the whole snapshot.
func resolveUniqueOpenJobs(previous, fresh int64, err error) int64 {
	if err != nil {
		return previous
	}
	return fresh
}
