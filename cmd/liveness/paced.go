package main

import (
	"cmp"
	"context"
	"log"
	"math/rand/v2"
	"slices"
	"sync"
	"time"

	"github.com/strelov1/freehire/internal/job/liveness"
	"github.com/strelov1/freehire/internal/platform/db"
)

// pacedVerdict is one paced probe's outcome: the verdict, its close reason, and whether the
// source refused to answer (a rate limit, a captcha) — the signal to stop probing that source
// for the rest of the run.
type pacedVerdict struct {
	verdict liveness.Verdict
	reason  string
	refused bool
}

// pacedPass is a source probed one request at a time instead of on applyVerdicts' fan-out:
// one whose rate limit is per IP and shared with the crawl, which reads the same site from
// the same host, so the pacing has to hold across the whole pass rather than per goroutine,
// and a refusal has to end the pass before it holds the crawl in the limit too.
type pacedPass struct {
	source string
	// interval spaces consecutive probes.
	interval time.Duration
	// maxPerRun bounds one run, so a backlog is worked off across runs instead of in one
	// pass that outlives the timer.
	maxPerRun int
	check     func(ctx context.Context, c candidate) pacedVerdict
}

// applyPaced probes a bounded sample of one source's candidates one at a time, spaced by
// p.interval, and stops at the first refusal.
//
// The sample puts a candidate already carrying a strike first and shuffles the rest. Shuffled,
// so successive runs reach every row of a backlog bigger than one run rather than re-reading
// the same first slice; struck first, because a close takes two consecutive Expired reads and
// a struck row left to the shuffle would wait for the dice to come round again — on a backlog
// several runs deep that doubles how long a dead posting stays open.
func (s *stats) applyPaced(ctx context.Context, queries *db.Queries, candidates []candidate, p pacedPass) {
	if len(candidates) == 0 {
		return
	}
	rand.Shuffle(len(candidates), func(i, j int) { candidates[i], candidates[j] = candidates[j], candidates[i] })
	slices.SortStableFunc(candidates, func(a, b candidate) int {
		return cmp.Compare(min(b.strikes, 1), min(a.strikes, 1))
	})
	if len(candidates) > p.maxPerRun {
		candidates = candidates[:p.maxPerRun]
	}
	tick := time.NewTicker(p.interval)
	defer tick.Stop()
	for i, c := range candidates {
		if i > 0 {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
		}
		v := p.check(ctx, c)
		if v.refused {
			log.Printf("liveness: %s refused probe %d of %d — stopping %s for this run", p.source, i+1, len(candidates), p.source)
			return
		}
		s.apply(ctx, queries, c, v.verdict, v.reason)
	}
}

// applyPacedPasses runs every pass at once, each over its own source's candidates. Each pass
// paces itself against its own site, and no two passes share a site, so running them side by
// side keeps the run as long as its slowest pass rather than the sum of them.
func (s *stats) applyPacedPasses(ctx context.Context, queries *db.Queries, bySource map[string][]candidate, passes []pacedPass) {
	var wg sync.WaitGroup
	for _, p := range passes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.applyPaced(ctx, queries, bySource[p.source], p)
		}()
	}
	wg.Wait()
}
