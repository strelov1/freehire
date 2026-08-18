package worker

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// pauseAllKey holds the entire cron fleet, and pauseKeyPrefix + a binary name
// holds one worker. Presence is the signal; the stored value is ignored, so an
// operator can leave themselves a note in it.
const (
	pauseKeyPrefix = "freehire:pause:"
	pauseAllKey    = pauseKeyPrefix + "all"
)

// pauseTimeout bounds the switch lookup. An unhealthy-but-reachable Redis must
// delay a worker's start by a known amount rather than by go-redis's dial
// default — the switch is a convenience, and a convenience may not decide how
// long the catalogue waits.
const pauseTimeout = 250 * time.Millisecond

// IgnorePauseEnv names the environment variable that bypasses the pause switch.
// systemd timer units do not carry it, so it admits only a run an operator
// started by hand — which is the point: hold the fleet, then run one thing
// against a quiet host.
//
// Parsed as a boolean rather than treated as present-or-absent, unlike the keys
// themselves: an operator who wrote =0 means the opposite of a bypass, and
// reading that as one would run the very fleet they just held.
const IgnorePauseEnv = "FREEHIRE_IGNORE_PAUSE"

// Paused reports whether an external switch is holding this worker back, where
// job is the worker binary's name. It answers only "may this run start"; a run
// already in flight is never interrupted.
func Paused(ctx context.Context, redisURL, job string) bool {
	if ignore, err := strconv.ParseBool(os.Getenv(IgnorePauseEnv)); err == nil && ignore {
		return false
	}

	held, err := switchHeld(ctx, redisURL, job)
	if err != nil {
		// Fail open, and say so. A facility for shedding load must never become
		// the reason the catalogue stops updating, but a silent degrade would be
		// indistinguishable from a switch that was genuinely clear.
		log.Printf("worker: pause switch unreadable, running anyway: %v", err)
		return false
	}
	return held
}

// switchHeld reports whether either pause key exists, or why it could not tell.
func switchHeld(ctx context.Context, redisURL, job string) (bool, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return false, err
	}
	// The budget goes on the client, not only on the context: go-redis applies
	// its own DialTimeout (5s) and retries a failed command three times with
	// backoff, so a context deadline alone lets a silent Redis hold up every
	// worker start for far longer than the deadline suggests.
	opts.DialTimeout = pauseTimeout
	opts.ReadTimeout = pauseTimeout
	opts.WriteTimeout = pauseTimeout
	opts.MaxRetries = -1

	client := redis.NewClient(opts)
	defer func() { _ = client.Close() }()

	ctx, cancel := context.WithTimeout(ctx, pauseTimeout)
	defer cancel()

	held, err := client.Exists(ctx, pauseAllKey, pauseKeyPrefix+job).Result()
	return held > 0, err
}

// workerJob names the running binary, which is both the per-worker pause key's
// suffix and the `job` label on its metrics — so the key an operator types is
// the label they read on the dashboard.
//
// It deliberately ignores os.Args[1:]. cmd/ingest takes a board file and gets a
// distinct metrics `instance` from it, but the pause key stays the binary's, so
// one key holds all ~140 board timers instead of needing one key each.
func workerJob() string {
	return filepath.Base(os.Args[0])
}
