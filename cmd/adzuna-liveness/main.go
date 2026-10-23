// Command adzuna-liveness closes the adzuna postings Adzuna itself has taken down. It is a
// run-once-and-exit worker: pick a sample of open postings the crawl has stopped listing, ask
// Adzuna's own page about each, close the ones it says are gone, and exit.
//
// Why a worker of its own. Neither existing close can reach these rows. The ingest sweep closes
// only what a crawl proves unseen, and Adzuna's feed is far deeper than the crawl's request
// budget — 157,782 US it-jobs postings in 14 days against 750 read per run (2026-10-23) — so
// most of the open catalogue is unseen whether it is live or not. cmd/liveness falls back to an
// age guess for adzuna, and Adzuna keeps postings far longer than any sane guess: 57% of its
// live US feed is older than 21 days. Meanwhile a posting it removed stayed open here for weeks
// (freehire.me/jobs/...-hcw3z65z). The evidence exists — Adzuna's hosted page answers 404/410
// for a removed posting — it is just behind a challenge most requests fail; see classify.
//
// Why not inside cmd/liveness. The page is reachable only through the Scrapling fetch service,
// which runs on the crawl host and listens on loopback there, while cmd/liveness runs on the
// database host and holds a single-flight advisory lock a second instance would collide with.
//
// A run without SCRAPLING_FETCH_URL is a no-op that never opens the pool, which is also the
// rollback. It exits non-zero only when a verdict could not be written.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/job/liveness"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/scraplingfetch"
	"github.com/strelov1/freehire/internal/platform/worker"
)

const (
	// unseenFor is how long the crawl must have stopped listing a posting before it is worth
	// asking about. Short on purpose: an answer from Adzuna is evidence, not a guess, so there
	// is no reason to wait out the sweep's 14-day window first — the wait only decides how much
	// of the sample is spent on postings that are merely below the crawl's reach.
	unseenFor = 3 * 24 * time.Hour
	// attempts bounds how many exit IPs one posting is tried from. About one request in ten
	// gets past the challenge (measured 2026-10-23), so 25 answers roughly nine postings in
	// ten; the rest are left for a later run's sample.
	attempts = 25
	// concurrency is how many postings are probed at once. Every request goes through the
	// Scrapling service the crawl fleet also uses, so this stays a small share of it.
	concurrency = 5
	// closeThreshold is 1, where cmd/liveness waits for two expired reads: there the second
	// read absorbs an employer's site mid-deploy, while a 404/410 here is Adzuna stating its
	// own posting is gone. A wrong close is also cheap to undo — the crawl reopens a posting
	// it lists again.
	closeThreshold = 1
	// fetchTimeout bounds one request through the service, rotating-proxy retries included.
	fetchTimeout = 60 * time.Second
	// defaultMaxPerRun is how many postings one run samples. At ~0.7 s a request this is
	// roughly an hour's work at the concurrency above.
	defaultMaxPerRun = 2000
)

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
	maxPerRun, err := worker.EnvInt32("ADZUNA_LIVENESS_MAX_PER_RUN", defaultMaxPerRun)
	if err != nil {
		log.Printf("config: %v", err)
		return 1
	}

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()
	queries := db.New(pool)

	candidates, err := queries.SelectAdzunaLivenessCandidates(ctx, db.SelectAdzunaLivenessCandidatesParams{
		Cutoff:  pgtype.Timestamptz{Time: time.Now().Add(-unseenFor), Valid: true},
		MaxRows: maxPerRun,
	})
	if err != nil {
		log.Printf("select candidates: %v", err)
		return 1
	}
	log.Printf("adzuna-liveness: probing %d postings unseen for %s", len(candidates), unseenFor)

	client := &http.Client{Transport: tr}
	var st stats
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for _, c := range candidates {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(c db.SelectAdzunaLivenessCandidatesRow) {
			defer wg.Done()
			defer func() { <-sem }()
			st.judge(ctx, queries, client, c)
		}(c)
	}
	wg.Wait()

	log.Printf("adzuna-liveness done: probed=%d closed=%d live=%d unanswered=%d failed=%d",
		st.probed.Load(), st.closed.Load(), st.live.Load(), st.unanswered.Load(), st.failed.Load())
	return worker.ExitCode(int(st.failed.Load()), 0)
}

// stats counts what one run did. Atomic because postings are judged concurrently.
type stats struct {
	probed, closed, live, unanswered, failed atomic.Int64
}

func (s *stats) judge(ctx context.Context, queries *db.Queries, client *http.Client, c db.SelectAdzunaLivenessCandidatesRow) {
	url, ok := detailsURL(c.ExternalID)
	if !ok {
		s.unanswered.Add(1)
		log.Printf("adzuna-liveness: %s has an external_id this worker cannot read (%q)", c.PublicSlug, c.ExternalID)
		return
	}
	s.probed.Add(1)
	verdict, tries, err := probe(ctx, client, url, attempts)
	switch verdict {
	case liveness.Expired:
		res, err := queries.MarkLivenessExpired(ctx, db.MarkLivenessExpiredParams{ID: c.ID, Threshold: closeThreshold})
		if err != nil {
			s.failed.Add(1)
			log.Printf("adzuna-liveness: close %s: %v", c.PublicSlug, err)
			return
		}
		if res.ClosedAt.Valid {
			s.closed.Add(1)
			log.Printf("adzuna-liveness: closed %s (gone on Adzuna, attempt %d)", c.PublicSlug, tries)
		}
	case liveness.Live:
		s.live.Add(1)
		if c.LivenessStrikes != 0 {
			if err := queries.ResetLivenessStrikes(ctx, c.ID); err != nil {
				s.failed.Add(1)
				log.Printf("adzuna-liveness: reset %s: %v", c.PublicSlug, err)
			}
		}
	default:
		// No answer past the challenge, or the service unreachable: leave the row for a later
		// sample. Not a failure — an unanswered probe is the common case.
		s.unanswered.Add(1)
		if err != nil && ctx.Err() == nil {
			log.Printf("adzuna-liveness: %s unanswered after %d attempts: %v", c.PublicSlug, tries, err)
		}
	}
}
