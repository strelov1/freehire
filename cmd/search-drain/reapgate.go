package main

import (
	"context"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// reapKey marks that a run has reaped within reapEvery. The reap's DELETE finds its few
// rows by walking the whole queue and looking each entry's job up, which on a large
// backlog costs minutes of disk reads (measured 2026-11-16: ~9 min for 72 rows in a 950k
// queue), so doing it every 10-minute run kept the database busy for nothing. The rows it
// removes are only skipped by Claim, never harmful, so once a day is enough.
const (
	reapKey   = "freehire:search-drain:reaped"
	reapEvery = 24 * time.Hour
)

// reapDue reports whether this run should reap: the first run in each reapEvery window
// claims the key and reaps. An unreadable Redis skips the reap, since it is housekeeping
// and the next run asks again.
func reapDue(redisURL string) func(context.Context) bool {
	return func(ctx context.Context) bool {
		opts, err := redis.ParseURL(redisURL)
		if err != nil {
			log.Printf("search-drain: reap gate: %v; skipping the reap", err)
			return false
		}
		opts.MaxRetries = -1
		client := redis.NewClient(opts)
		defer func() { _ = client.Close() }()
		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		claimed, err := client.SetNX(ctx, reapKey, time.Now().UTC().Format(time.RFC3339), reapEvery).Result()
		if err != nil {
			log.Printf("search-drain: reap gate unreadable, skipping the reap: %v", err)
			return false
		}
		return claimed
	}
}
