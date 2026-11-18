// Command adzuna-liveness closes postings that an aggregator (adzuna, wellfound) has taken
// down when the crawl cannot settle them. It is a run-once-and-exit worker: per source, sample
// open postings the crawl has stopped listing, ask the source's own page about each, and close
// the ones it says are gone. It began with adzuna alone and kept the name; the binary and its
// crawl-host unit are deployed under it.
//
// Neither existing close can reach these rows; the ingest sweep closes only what a crawl
// proves unseen.
//
//   - Adzuna's feed is far deeper than the crawl's request budget, so most of the open
//     catalogue is unseen whether live or not. cmd/liveness falls back to an age guess for
//     adzuna, and Adzuna keeps postings far longer than any sane guess.
//   - Wellfound is crawled as role slices and swept by company, so a posting whose company
//     has left every slice is never in a run's scope again (job-lifecycle.md (1)).
//
// The evidence is the source's own page answering 404/410 for a removed posting, reachable
// only through the Scrapling fetch service: Adzuna's page sits behind a challenge most
// requests fail, and Wellfound answers 403 to any plain request; see classify.
//
// It is not part of cmd/liveness because the Scrapling service listens on loopback on the
// crawl host, while cmd/liveness runs on the database host and holds a single-flight advisory
// lock a second instance would collide with.
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
	"github.com/strelov1/freehire/internal/platform/pgconv"
	"github.com/strelov1/freehire/internal/platform/scraplingfetch"
	"github.com/strelov1/freehire/internal/platform/worker"
)

const (
	// unseenFor is how long the crawl must have stopped listing a posting before it is worth
	// asking about. Short on purpose: the source's answer is evidence, not a guess, so there is
	// no need to wait out the sweep's 14-day window.
	unseenFor = 3 * 24 * time.Hour
	// closeThreshold is 1, where cmd/liveness waits for two expired reads: a 404/410 here is the
	// source stating its own posting is gone, and a wrong close is cheap to undo because the
	// crawl reopens a posting it lists again.
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
		// About one request in ten gets past Adzuna's challenge, so 25 attempts answer roughly nine
		// postings in ten; the rest wait for a later run.
		attempts:    25,
		concurrency: 5,
		maxEnv:      "ADZUNA_LIVENESS_MAX_PER_RUN",
		// Roughly an hour's work at the concurrency above.
		defaultMax: 2000,
	},
	{
		source:  "wellfound",
		pageURL: wellfoundPageURL,
		// Retries only absorb a transient refusal of one exit IP.
		attempts: 3,
		// One in flight, like the crawl's own cap (sources.wellfoundMaxInFlight): the exit IP is
		// shared with the crawl, and Wellfound's edge punishes concurrent requests.
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
		return worker.Skip("SCRAPLING_FETCH_URL")
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
	cutoff := pgconv.Instant(time.Now().Add(-unseenFor))
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
