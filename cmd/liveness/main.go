// Command liveness is the standalone orphan-job liveness worker. It probes the
// posting URL of every open job the ingest sweep never re-crawls — the non-board
// sources (telegram, habr_career, geekjob, …) whose closed_at would otherwise stay
// NULL forever — and closes a job once two consecutive probes report it dead.
//
// It is a run-once-and-exit worker (cron-scheduled beside ingest/enrich): select
// candidates, probe each over plain HTTP, classify, apply the strike/close/reset
// update, and exit. Re-running is safe; only a definitive death signal confirmed
// twice in a row closes a job, biasing toward leaving orphans open over a false
// close (orphans have no re-ingest to reopen them).
package main

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/strelov1/freehire/internal/config"
	"github.com/strelov1/freehire/internal/database"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/liveness"
	"github.com/strelov1/freehire/internal/safehttp"
	"github.com/strelov1/freehire/internal/sources"
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
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()
	queries := db.New(pool)

	// The candidate set is "every open job whose source is not a registered ATS
	// provider" — the registry keys are the exclusion list, so a new adapter never
	// silently becomes a probe target.
	atsProviders := providerKeys(sources.All(sources.NewClient()))
	// Guard: `source <> ALL('{}')` is vacuously TRUE in Postgres, so an empty
	// exclusion list would select EVERY open job — including board jobs the ingest
	// sweep owns. Refuse to run rather than risk URL-closing the whole catalogue.
	if len(atsProviders) == 0 {
		log.Fatal("liveness: no ATS providers registered — refusing to run (would probe every open job)")
	}

	candidates, err := queries.SelectOrphanLivenessCandidates(ctx, atsProviders)
	if err != nil {
		log.Fatalf("select candidates: %v", err)
	}
	log.Printf("liveness: %d orphan candidates (excluding %d ATS providers)", len(candidates), len(atsProviders))

	// Probe targets are orphan-job URLs that originated from attacker-influenced
	// sources (telegram posts), so the probe must refuse internal/metadata targets.
	client := safehttp.NewClient(probeTimeout)
	var probed, closed, struck int64

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
				// clears a strike.
				log.Printf("liveness: probe %s failed: %v", c.PublicSlug, ferr)
			}

			switch verdict, reason := liveness.Classify(status, finalURL, body); verdict {
			case liveness.Expired:
				res, err := queries.MarkLivenessExpired(ctx, db.MarkLivenessExpiredParams{
					ID:        c.ID,
					Threshold: closeThreshold,
				})
				if err != nil {
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
						log.Printf("liveness: reset %s: %v", c.PublicSlug, err)
					}
				}
			case liveness.Uncertain:
				// No signal either way — leave the strike count untouched.
			}
		}(c)
	}
	wg.Wait()

	log.Printf("liveness done: probed=%d closed=%d struck=%d", probed, closed, struck)
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
