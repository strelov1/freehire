package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/job/liveness"
	"github.com/strelov1/freehire/internal/platform/db"
)

// judge reads one candidate's evidence and reports a verdict. Evidence paths differ only in
// how the verdict is reached, never in what is done with it.
type judge func(ctx context.Context, c candidate) (liveness.Verdict, string)

// probeJudge GETs the posting's own URL and classifies the response.
func probeJudge(client *http.Client) judge {
	return func(ctx context.Context, c candidate) (liveness.Verdict, string) {
		status, finalURL, body, err := liveness.Fetch(ctx, client, c.url)
		if err != nil {
			// An unreachable page classifies Uncertain (status 0): no strike change, not a
			// worker failure.
			log.Printf("liveness: probe %s failed: %v", c.publicSlug, err)
		}
		return liveness.Classify(status, finalURL, body)
	}
}

// echoJobsJudge reads echojobs.io's per-posting page by external id; the stored jobs.url is
// the employer's own ATS link (see echojobs.go).
func echoJobsJudge(client *http.Client) judge {
	return func(ctx context.Context, c candidate) (liveness.Verdict, string) {
		return checkEchoJobsLive(ctx, client, c.externalID)
	}
}

// stats counts what one run did. Atomic because the fan-out judges candidates concurrently,
// and the exit code is read off failed.
type stats struct {
	probed, closed, struck, failed atomic.Int64
}

// applyVerdicts judges every candidate at `concurrency` and applies each verdict as it
// lands. The bound is per call rather than run-wide because the calls do not overlap: each
// waits for its own fan-out before the next begins.
func (s *stats) applyVerdicts(ctx context.Context, queries *db.Queries, candidates []candidate, decide judge) {
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for _, c := range candidates {
		wg.Add(1)
		sem <- struct{}{}
		go func(c candidate) {
			defer wg.Done()
			defer func() { <-sem }()
			verdict, reason := decide(ctx, c)
			s.apply(ctx, queries, c, verdict, reason)
		}(c)
	}
	wg.Wait()
}

// apply writes one verdict: advances/closes on Expired, clears strikes on Live, and leaves
// Uncertain untouched. Every evidence path in this worker ends here, so the
// close/strike/reset semantics live in exactly one place.
func (s *stats) apply(ctx context.Context, queries *db.Queries, c candidate, verdict liveness.Verdict, reason string) {
	s.probed.Add(1)
	switch verdict {
	case liveness.Expired:
		res, err := queries.MarkLivenessExpired(ctx, db.MarkLivenessExpiredParams{
			ID:         c.id,
			Threshold:  closeThreshold,
			SeenBefore: c.seenBefore,
		})
		if c.seenBefore.Valid && errors.Is(err, pgx.ErrNoRows) {
			// Ingest saw the posting again before the probe could record its verdict.
			return
		}
		if err != nil {
			// The verdict was reached but the DB update did not apply — a real
			// failure the exit code must surface, not a silent log-and-continue.
			s.failed.Add(1)
			log.Printf("liveness: mark expired %s: %v", c.publicSlug, err)
			return
		}
		if res.ClosedAt.Valid {
			s.closed.Add(1)
			log.Printf("liveness: closed %s (%s, %s)", c.publicSlug, c.source, reason)
		} else {
			s.struck.Add(1)
			log.Printf("liveness: strike %d/%d %s (%s)", res.LivenessStrikes, closeThreshold, c.publicSlug, reason)
		}
	case liveness.Live:
		// Clear any accumulated strikes. Skip the write when there is nothing to
		// clear so a healthy catalogue does not issue an UPDATE per open job.
		if c.strikes != 0 {
			if err := queries.ResetLivenessStrikes(ctx, c.id); err != nil {
				s.failed.Add(1)
				log.Printf("liveness: reset %s: %v", c.publicSlug, err)
			}
		}
	case liveness.Uncertain:
		// No signal either way — leave the strike count untouched.
	}
}

// applyHimalayasVerdicts checks himalayas candidates against the site's sitemap, since the
// job page is Cloudflare-blocked (see himalayas.go). The sitemap is fetched once per run;
// a fetch failure counts as one worker failure and skips every candidate (under-closing).
func applyHimalayasVerdicts(ctx context.Context, client *http.Client, queries *db.Queries, candidates []candidate, s *stats) {
	if len(candidates) == 0 {
		return
	}
	liveURLs, err := fetchHimalayasLiveJobURLs(ctx, client)
	if err != nil {
		s.failed.Add(1)
		log.Printf("liveness: himalayas sitemap fetch failed, skipping %d stale registered candidates: %v", len(candidates), err)
		return
	}
	s.applyVerdicts(ctx, queries, candidates, func(_ context.Context, c candidate) (liveness.Verdict, string) {
		if _, ok := liveURLs[c.url]; ok {
			return liveness.Live, ""
		}
		return liveness.Expired, "absent_from_sitemap"
	})
}
