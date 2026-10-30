// Command adzuna-liveness closes the postings an aggregator itself has taken down when the crawl
// cannot settle them — adzuna's and wellfound's. It is a run-once-and-exit worker: for each
// source, pick a sample of open postings the crawl has stopped listing, ask the source's own
// page about each, close the ones it says are gone, and exit. (It began with adzuna alone and
// kept the name: the binary and its crawl-host unit are deployed under it.)
//
// Why a worker of its own. Neither existing close can reach these rows. The ingest sweep closes
// only what a crawl proves unseen.
//
//   - Adzuna's feed is far deeper than the crawl's request budget — 157,782 US it-jobs postings
//     in 14 days against 750 read per run (2026-10-23) — so most of the open catalogue is unseen
//     whether it is live or not. cmd/liveness falls back to an age guess for adzuna, and Adzuna
//     keeps postings far longer than any sane guess: 57% of its live US feed is older than 21
//     days. Meanwhile a posting it removed stayed open here for weeks.
//   - Wellfound is crawled as role slices and swept by company, so a posting whose company has
//     left every slice is never in a run's scope again (job-lifecycle.md (1)). Measured
//     2026-10-30: 954 open postings of ~380 companies unseen since mid-September, and 20 of 20
//     sampled answered 410 on Wellfound's own page while every live control answered 200.
//
// The evidence exists for both — the source's page answers 404/410 for a removed posting — but
// only through the Scrapling fetch service: Adzuna's page sits behind a challenge most requests
// fail, and Wellfound's answers 403 to any plain request; see classify.
//
// Why not inside cmd/liveness. The Scrapling fetch service runs on the crawl host and listens on
// loopback there, while cmd/liveness runs on the database host and holds a single-flight
// advisory lock a second instance would collide with.
//
// A run without SCRAPLING_FETCH_URL is a no-op that never opens the pool, which is also the
// rollback. It exits non-zero only when a verdict could not be written.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/job/liveness"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/scraplingfetch"
	"github.com/strelov1/freehire/internal/platform/worker"
)

const (
	// unseenFor is how long the crawl must have stopped listing a posting before it is worth
	// asking about. Short on purpose: an answer from the source is evidence, not a guess, so
	// there is no reason to wait out the sweep's 14-day window first — the wait only decides how
	// much of the sample is spent on postings that are merely below the crawl's reach.
	unseenFor = 3 * 24 * time.Hour
	// closeThreshold is 1, where cmd/liveness waits for two expired reads: there the second
	// read absorbs an employer's site mid-deploy, while a 404/410 here is the source stating
	// its own posting is gone. A wrong close is also cheap to undo — the crawl reopens a
	// posting it lists again.
	closeThreshold = 1
	// fetchTimeout bounds one request through the service, rotating-proxy retries included.
	fetchTimeout = 60 * time.Second
)

// route is how one source is asked about its postings.
type route struct {
	source string
	// pageURL builds the page to probe from a candidate row, refusing a row it cannot read.
	pageURL func(externalID, url string) (string, bool)
	// attempts bounds how many exit IPs one posting is tried from.
	attempts int
	// concurrency is how many postings are probed at once. Every request goes through the
	// Scrapling service the crawl fleet also uses, so this stays a small share of it.
	concurrency int
	// maxEnv names the knob bounding how many postings one run samples; defaultMax is its
	// default.
	maxEnv     string
	defaultMax int32
}

var routes = []route{
	{
		source:  "adzuna",
		pageURL: func(externalID, _ string) (string, bool) { return detailsURL(externalID) },
		// About one request in ten gets past Adzuna's challenge (measured 2026-10-23), so 25
		// answers roughly nine postings in ten; the rest are left for a later run's sample.
		attempts:    25,
		concurrency: 5,
		maxEnv:      "ADZUNA_LIVENESS_MAX_PER_RUN",
		// At ~0.7 s a request this is roughly an hour's work at the concurrency above.
		defaultMax: 2000,
	},
	{
		source:  "wellfound",
		pageURL: wellfoundPageURL,
		// Wellfound answered all 20 sampled postings on the first request; the retries only
		// absorb a transient refusal of one exit IP.
		attempts: 3,
		// One in flight, like the crawl's own cap (sources.wellfoundMaxInFlight): the exit IP
		// is shared with the crawl, whose role slices run at the same time, and Wellfound's
		// edge has punished concurrent requests before.
		concurrency: 1,
		maxEnv:      "WELLFOUND_LIVENESS_MAX_PER_RUN",
		defaultMax:  1000,
	},
}

func main() {
	worker.Main(run)
}

func run() int {
	rawURL := strings.TrimSpace(os.Getenv("SCRAPLING_FETCH_URL"))
	if rawURL == "" {
		log.Print("adzuna-liveness: SCRAPLING_FETCH_URL is not set — nothing to do")
		return 0
	}
	tr, err := scraplingfetch.New(scraplingfetch.Config{
		URL:   rawURL,
		Token: strings.TrimSpace(os.Getenv("SCRAPLING_FETCH_TOKEN")),
	}, scraplingfetch.ModeHTTP, true, fetchTimeout)
	if err != nil {
		log.Printf("config: %v", err)
		return 1
	}
	maxPerRun := make([]int32, len(routes))
	for i, r := range routes {
		if maxPerRun[i], err = worker.EnvInt32(r.maxEnv, r.defaultMax); err != nil {
			log.Printf("config: %v", err)
			return 1
		}
	}

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()
	queries := db.New(pool)
	client := &http.Client{Transport: tr}

	// The sources are different sites, so their passes run side by side; each keeps its own
	// concurrency against its own host.
	failed := make([]int, len(routes))
	var wg sync.WaitGroup
	for i, r := range routes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			failed[i] = r.run(ctx, queries, client, maxPerRun[i])
		}()
	}
	wg.Wait()

	total := 0
	for _, f := range failed {
		total += f
	}
	return worker.ExitCode(total, 0)
}

// run probes one sample of the route's source and returns how many verdicts it could not write.
func (r route) run(ctx context.Context, queries *db.Queries, client *http.Client, maxRows int32) int {
	cutoff := pgtype.Timestamptz{Time: time.Now().Add(-unseenFor), Valid: true}
	candidates, err := queries.SelectScraplingLivenessCandidates(ctx, db.SelectScraplingLivenessCandidatesParams{
		Source:  r.source,
		Cutoff:  cutoff,
		MaxRows: maxRows,
	})
	if err != nil {
		log.Printf("%s-liveness: select candidates: %v", r.source, err)
		return 1
	}
	log.Printf("%s-liveness: probing %d postings unseen for %s", r.source, len(candidates), unseenFor)

	var st stats
	sem := make(chan struct{}, r.concurrency)
	var wg sync.WaitGroup
	for _, c := range candidates {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			st.judge(ctx, queries, client, r, cutoff, c)
		}()
	}
	wg.Wait()

	log.Printf("%s-liveness done: probed=%d closed=%d live=%d unanswered=%d failed=%d", r.source,
		st.probed.Load(), st.closed.Load(), st.live.Load(), st.unanswered.Load(), st.failed.Load())
	return int(st.failed.Load())
}

// stats counts what one source's pass did. Atomic because postings are judged concurrently.
type stats struct {
	probed, closed, live, unanswered, failed atomic.Int64
}

// judge probes one candidate and writes the verdict. cutoff is the staleness bound the
// candidate was selected by: the close repeats it, so a posting the crawl listed again while
// the probe was in flight is left alone.
func (s *stats) judge(ctx context.Context, queries *db.Queries, client *http.Client, r route, cutoff pgtype.Timestamptz, c db.SelectScraplingLivenessCandidatesRow) {
	url, ok := r.pageURL(c.ExternalID, c.URL)
	if !ok {
		s.unanswered.Add(1)
		log.Printf("%s-liveness: %s has a posting this worker cannot read (%q, %q)", r.source, c.PublicSlug, c.ExternalID, c.URL)
		return
	}
	s.probed.Add(1)
	verdict, tries, err := probe(ctx, client, url, r.attempts)
	switch verdict {
	case liveness.Expired:
		res, err := queries.MarkLivenessExpired(ctx, db.MarkLivenessExpiredParams{
			ID: c.ID, Threshold: closeThreshold, SeenBefore: cutoff,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			// The crawl listed it again after the select: that read is newer than the probe's.
			s.live.Add(1)
			log.Printf("%s-liveness: %s re-listed by the crawl while probed, left open", r.source, c.PublicSlug)
			return
		}
		if err != nil {
			s.failed.Add(1)
			log.Printf("%s-liveness: close %s: %v", r.source, c.PublicSlug, err)
			return
		}
		if res.ClosedAt.Valid {
			s.closed.Add(1)
			log.Printf("%s-liveness: closed %s (gone at the source, attempt %d)", r.source, c.PublicSlug, tries)
		}
	case liveness.Live:
		s.live.Add(1)
		if c.LivenessStrikes != 0 {
			if err := queries.ResetLivenessStrikes(ctx, c.ID); err != nil {
				s.failed.Add(1)
				log.Printf("%s-liveness: reset %s: %v", r.source, c.PublicSlug, err)
			}
		}
	default:
		// No answer past a challenge, or the service unreachable: leave the row for a later
		// sample. Not a failure — an unanswered probe is the common case.
		s.unanswered.Add(1)
		if err != nil && ctx.Err() == nil {
			log.Printf("%s-liveness: %s unanswered after %d attempts: %v", r.source, c.PublicSlug, tries, err)
		}
	}
}
