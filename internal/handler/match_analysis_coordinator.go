package handler

import "sync"

// matchAnalysisCoordinator coalesces concurrent fit-analysis computes for the same (user, job)
// pair across the two entry points that can each decide to run the three-stage LLM chain:
// ensureCachedAnalysis (the cold-start autopilot's invisible pre-run, so cv_context has
// something to read) and StreamMatchAnalysis (the tailoring workspace's visible stepper,
// started at the same cold start). Both can reach here for the identical pair within
// milliseconds of each other; without coalescing that is two three-stage chains spent on one
// input — real wasted spend, since the cache exists specifically to avoid recomputing — racing
// to upsert the same row.
//
// Hand-rolled rather than golang.org/x/sync/singleflight: a follower must never touch the LLM —
// it waits, then reads whatever the leader left in the cache, over a completely different code
// path from the leader's own AnalyzeStream-and-emit loop. singleflight.Do blocks every caller
// (leader and followers alike) inside the call for the compute's whole duration, with no way
// for a follower to run its OWN concurrent work meanwhile — StreamMatchAnalysis's follower
// needs exactly that, to keep its own SSE connection's heartbeat ticking while it waits (see
// followMatchAnalysis). lead here returns immediately with an explicit role and a wait handle,
// so a follower can select on it alongside its own ticker instead of blocking inside Do.
//
// Billing is NOT decided here. Each caller that cares about AI credits (StreamMatchAnalysis)
// gates and debits for itself, whether it ends up leading or following — see its own comment
// on why that is safe to do independently without double-charging. ensureCachedAnalysis never
// touches credits at all, leader or follower, which is what keeps the cold-start pre-run free.
//
// Zero value is ready to use — the map is created lazily under the lock — so every existing
// matchHandlers{...} literal (test fixtures included) keeps compiling with nothing to wire.
type matchAnalysisCoordinator struct {
	mu       sync.Mutex
	inflight map[analysisKey]*analysisRun
}

// analysisKey is the pair a compute is coalesced on. Scoped to one user: two candidates
// analysing the same vacancy at once are not the same input (different résumé, different
// blockers) and must never coalesce.
type analysisKey struct {
	userID, jobID int64
}

// analysisRun is what a follower waits on: done closes once the leader's compute ends, and
// succeeded — safe to read only after that, never before — reports whether the cache is left
// in a trustworthy state (a fresh compute, or one found already cached) versus a failed
// attempt that left nothing new behind. A follower that skips this check and reads the cache
// unconditionally can serve a stale row as if it were this run's result — see
// followMatchAnalysis's own comment for the failure this caused live.
type analysisRun struct {
	done      chan struct{}
	succeeded bool
}

// lead claims (userID, jobID) for the calling goroutine. The first caller becomes the LEADER
// (isLeader=true) and owns running the chain; it MUST call done exactly once when its compute
// ends — success, failure, or "LLM unconfigured" all count, with succeeded reporting which —
// or every follower blocks forever. done is safe to call more than once (only the first call
// has any effect): a future edit that adds one more early return before an existing done() call
// must not turn into a double-close panic, which — running from the async SSE body-writer
// goroutine fasthttp does not recover panics in — would take the whole process down, not just
// the one request.
//
// A concurrent caller for the same pair is a FOLLOWER (isLeader=false, done=nil): it gets back
// the same *analysisRun the leader is working on, to wait on directly (<-run.done) and then
// consult (run.succeeded). lead itself never blocks — a follower decides for itself where to
// wait (StreamMatchAnalysis waits inside its own SSE body-writer, on the background context its
// leader compute already runs under, rather than blocking the request that is about to open the
// stream; ensureCachedAnalysis waits inline, exactly as it already blocked for a whole chain's
// duration running its OWN compute before this existed).
//
// run.done carries no cancellation and lead takes no ctx: nothing that calls this today needs
// the wait to be interruptible — see the two call sites' own comments.
func (co *matchAnalysisCoordinator) lead(userID, jobID int64) (done func(succeeded bool), run *analysisRun, isLeader bool) {
	key := analysisKey{userID, jobID}
	co.mu.Lock()
	if existing, running := co.inflight[key]; running {
		co.mu.Unlock()
		return nil, existing, false
	}
	run = &analysisRun{done: make(chan struct{})}
	if co.inflight == nil {
		co.inflight = make(map[analysisKey]*analysisRun)
	}
	co.inflight[key] = run
	co.mu.Unlock()

	var once sync.Once
	done = func(succeeded bool) {
		once.Do(func() {
			co.mu.Lock()
			delete(co.inflight, key)
			co.mu.Unlock()
			// Written before the close, which is what makes it safe for a follower to read
			// right after <-run.done unblocks: a channel close happens-before a receive that
			// observes it, per the Go memory model, so this write is visible by then with no
			// extra lock needed on the follower's side.
			run.succeeded = succeeded
			close(run.done)
		})
	}
	return done, run, true
}
