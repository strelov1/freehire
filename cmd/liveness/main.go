// Command liveness is the standalone orphan-job liveness worker. It probes the
// posting URL of every open job the ingest sweep never re-crawls — the non-board
// sources (telegram, habr_career, geekjob, …) whose closed_at would otherwise stay
// NULL forever — and closes a job once two consecutive probes report it dead.
//
// It is a run-once-and-exit worker (cron-scheduled beside ingest/enrich): select
// candidates, probe each over plain HTTP, classify, apply the strike/close/reset
// update, and exit. Re-running is safe; only a definitive death signal confirmed
// twice in a row closes a job, biasing toward leaving orphans open over a false
// close (orphans have no re-ingest to reopen them). It exits non-zero when a probe
// could not apply its DB update, so cron can alert.
package main

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/liveness"
	"github.com/strelov1/freehire/internal/safehttp"
	"github.com/strelov1/freehire/internal/sources"
	"github.com/strelov1/freehire/internal/worker"
)

const (
	// closeThreshold is the number of CONSECUTIVE expired probes that closes a job.
	// Two reads across separate runs absorb a transient death signal (an employer
	// site mid-deploy) without a probe-history table.
	closeThreshold = 2
	// probeTimeout bounds a single URL fetch so one slow host cannot stall the run.
	probeTimeout = 15 * time.Second
	// concurrency caps simultaneous probes: orphan postings span many hosts, so this
	// keeps the worker from hammering any single employer site while staying brisk.
	concurrency = 8
	// lockKey is the Postgres advisory-lock key that serializes liveness runs. Cron
	// offers no host-level guarantee against stacking, and two runs probing the same
	// orphan seconds apart would collapse the "two consecutive expired reads" grace
	// into one burst — closing a job on a transient blip. A second run that can't take
	// the lock exits cleanly. The value is an arbitrary constant unique to this worker.
	lockKey = 0x66686c76 // "fhlv" — freehire liveness
)

// unprobableSources are open-job sources excluded from the probe on top of the ATS
// registry: their stored URL is a container that outlives the vacancy, not the
// vacancy's own page, so a liveness probe can never reach a death verdict. Only
// telegram qualifies today — its URL is the Telegram post (see cmd/tg-extract), which
// stays live after the job is filled. (habr_career/geekjob are NOT here: their URL is
// the vacancy page itself and does 404 when the posting is removed.) These jobs have
// no lifecycle close signal at all; see the job-lifecycle spec's telegram limitation.
var unprobableSources = []string{"telegram"}

func main() {
	worker.Main(run)
}

func run() int {
	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()
	queries := db.New(pool)

	// Single-flight: hold a session-scoped advisory lock on a dedicated connection
	// for the whole run so overlapping cron invocations can't strike the same orphan
	// twice within one burst. A run that can't take the lock exits cleanly.
	lockConn, err := pool.Acquire(ctx)
	if err != nil {
		log.Printf("acquire lock connection: %v", err)
		return 1
	}
	var locked bool
	if err := lockConn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", int64(lockKey)).Scan(&locked); err != nil {
		lockConn.Release()
		log.Printf("liveness lock: %v", err)
		return 1
	}
	if !locked {
		lockConn.Release()
		log.Print("liveness: another run holds the lock — exiting")
		return 0
	}
	defer func() {
		// Best-effort unlock; releasing/closing the connection drops the session lock anyway.
		_, _ = lockConn.Exec(ctx, "SELECT pg_advisory_unlock($1)", int64(lockKey))
		lockConn.Release()
	}()

	// The candidate set is "every open job whose source is not a registered ATS
	// provider" — the registry keys are the exclusion list, so a new adapter never
	// silently becomes a probe target.
	atsProviders := providerKeys(sources.All(sources.NewClient()))
	// Guard: `source <> ALL('{}')` is vacuously TRUE in Postgres, so an empty
	// exclusion list would select EVERY open job — including board jobs the ingest
	// sweep owns. Refuse to run rather than risk URL-closing the whole catalogue.
	if len(atsProviders) == 0 {
		log.Print("liveness: no ATS providers registered — refusing to run (would probe every open job)")
		return 1
	}

	// Also exclude sources whose stored URL is not the vacancy's own page but a
	// container that outlives it — a Telegram job's URL is the post (https://t.me/
	// <chan>/<id>), which stays live long after the vacancy it advertised is filled,
	// so probing it always reads Live and never closes the job. Excluding it skips a
	// probe that can only waste a fetch, never reach a verdict. Appended AFTER the
	// guard above so the empty-ATS-registry safeguard still keys off atsProviders.
	excluded := append(atsProviders, unprobableSources...)

	candidates, err := queries.SelectOrphanLivenessCandidates(ctx, excluded)
	if err != nil {
		log.Printf("select candidates: %v", err)
		return 1
	}
	log.Printf("liveness: %d orphan candidates (excluding %d ATS providers + %d unprobable sources)", len(candidates), len(atsProviders), len(unprobableSources))

	// Probe targets are orphan-job URLs that originated from attacker-influenced
	// sources (telegram posts), so the probe must refuse internal/metadata targets.
	client := safehttp.NewClient(probeTimeout)
	var probed, closed, struck, failed int64

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for _, c := range candidates {
		wg.Add(1)
		sem <- struct{}{}
		go func(c db.SelectOrphanLivenessCandidatesRow) {
			defer wg.Done()
			defer func() { <-sem }()
			atomic.AddInt64(&probed, 1)

			status, finalURL, body, ferr := liveness.Fetch(ctx, client, c.URL)
			if ferr != nil {
				// A probe that could not reach the page is Uncertain (status 0), so the
				// switch below takes no action — a fetch failure never advances or
				// clears a strike, and is not counted as a worker failure.
				log.Printf("liveness: probe %s failed: %v", c.PublicSlug, ferr)
			}

			switch verdict, reason := liveness.Classify(status, finalURL, body); verdict {
			case liveness.Expired:
				res, err := queries.MarkLivenessExpired(ctx, db.MarkLivenessExpiredParams{
					ID:        c.ID,
					Threshold: closeThreshold,
				})
				if err != nil {
					// The verdict was reached but the DB update did not apply — a real
					// failure the exit code must surface, not a silent log-and-continue.
					atomic.AddInt64(&failed, 1)
					log.Printf("liveness: mark expired %s: %v", c.PublicSlug, err)
					return
				}
				if res.ClosedAt.Valid {
					atomic.AddInt64(&closed, 1)
					log.Printf("liveness: closed %s (%s, %s)", c.PublicSlug, c.Source, reason)
				} else {
					atomic.AddInt64(&struck, 1)
					log.Printf("liveness: strike %d/%d %s (%s)", res.LivenessStrikes, closeThreshold, c.PublicSlug, reason)
				}
			case liveness.Live:
				// Clear any accumulated strikes. Skip the write when there is nothing to
				// clear so a healthy catalogue does not issue an UPDATE per open job.
				if c.LivenessStrikes != 0 {
					if err := queries.ResetLivenessStrikes(ctx, c.ID); err != nil {
						atomic.AddInt64(&failed, 1)
						log.Printf("liveness: reset %s: %v", c.PublicSlug, err)
					}
				}
			case liveness.Uncertain:
				// No signal either way — leave the strike count untouched.
			}
		}(c)
	}
	wg.Wait()

	log.Printf("liveness done: probed=%d closed=%d struck=%d failed=%d", probed, closed, struck, failed)
	return worker.ExitCode(int(failed), 0)
}

// providerKeys returns the registered ATS provider keys — the sources the ingest
// sweep owns and the liveness probe must exclude.
func providerKeys(registry map[string]sources.Source) []string {
	keys := make([]string, 0, len(registry))
	for k := range registry {
		keys = append(keys, k)
	}
	return keys
}
