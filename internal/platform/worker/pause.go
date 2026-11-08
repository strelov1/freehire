package worker

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"sync"
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

// PauseKey is the pause key named by suffix. A caller that owns a family of
// keys of its own (the mail classes) builds them here so the prefix has one author.
func PauseKey(suffix string) string { return pauseKeyPrefix + suffix }

// Paused reports whether an external switch is holding this worker back, where
// job is the worker binary's name. It answers only "may this run start"; a run
// already in flight is never interrupted. extraKeys are further keys that hold
// this worker, for a caller that classifies workers in a way this package does
// not know (see Main).
func Paused(ctx context.Context, redisURL, job string, extraKeys ...string) bool {
	return AnyHeld(ctx, redisURL, append([]string{pauseAllKey, PauseKey(job)}, extraKeys...)...)
}

// AnyHeld reports whether any of keys exists, with the bypass and the fail-open
// behaviour of Paused: FREEHIRE_IGNORE_PAUSE lifts every hold, and an unreadable
// Redis holds nothing.
func AnyHeld(ctx context.Context, redisURL string, keys ...string) bool {
	if ignore, err := strconv.ParseBool(os.Getenv(IgnorePauseEnv)); err == nil && ignore {
		return false
	}

	held, err := keysHeld(ctx, redisURL, keys...)
	if err != nil {
		// Fail open, and say so. A facility for shedding load must never become
		// the reason the catalogue stops updating, but a silent degrade would be
		// indistinguishable from a switch that was genuinely clear.
		log.Printf("worker: pause switch unreadable, running anyway: %v", err)
		return false
	}
	return held
}

// keysHeld reports whether any of keys exists, or why it could not tell.
func keysHeld(ctx context.Context, redisURL string, keys ...string) (bool, error) {
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

	held, err := client.Exists(ctx, keys...).Result()
	return held > 0, err
}

// SearchDrainJob is the pause key's suffix for cmd/search-drain, named here because the
// rebuild that holds it is a different binary.
const SearchDrainJob = "search-drain"

// HoldPause holds another worker back for as long as the caller needs, through the same
// switch an operator uses, until the returned release is called. The key carries a TTL
// that is refreshed while the caller runs, so a holder that dies cannot leave the worker
// off: the drain was once forgotten that way for hours (2026-08-06, 2026-08-15). It only
// stops new work; a worker in flight has to look for itself (see Paused).
func HoldPause(ctx context.Context, redisURL, job string, ttl time.Duration) (release func(), err error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}
	client := redis.NewClient(opts)
	key := pauseKeyPrefix + job
	if err := client.Set(ctx, key, "held by "+workerJob(), ttl).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(ttl / 3)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if err := client.Expire(ctx, key, ttl).Err(); err != nil {
					log.Printf("worker: refreshing the %s pause: %v", job, err)
				}
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			close(done)
			<-stopped
			// A cancelled run still lifts the hold; it must not wait out the TTL.
			cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			if err := client.Del(cctx, key).Err(); err != nil {
				log.Printf("worker: lifting the %s pause, it expires on its own: %v", job, err)
			}
			_ = client.Close()
		})
	}, nil
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
