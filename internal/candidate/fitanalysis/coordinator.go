package fitanalysis

import "sync"

// coordinator coalesces concurrent fit-analysis computes for the same (candidate, job)
// pair across the entry points that can each decide to run the three-stage LLM chain:
// Service.Ensure (the cold-start autopilot's invisible pre-run, so cv_context has
// something to read) and Service.Run behind the tailoring workspace's visible stepper,
// started at the same cold start. Both can reach here for the identical pair within
// milliseconds of each other; without coalescing that is two three-stage chains spent on one
// input — real wasted spend, since the cache exists specifically to avoid recomputing —
// racing to upsert the same row.
//
// Hand-rolled rather than golang.org/x/sync/singleflight: a follower must never touch the LLM —
// it waits, then reads whatever the leader left in the cache, over a completely different code
// path from the leader's own AnalyzeStream-and-emit loop. singleflight.Do blocks every caller
// (leader and followers alike) inside the call for the compute's whole duration, with no way
// for a follower to run its OWN concurrent work meanwhile — a streaming follower needs exactly
// that, to keep its own SSE connection's heartbeat ticking while it waits (see Service.Follow
// and its caller). Claim here returns immediately with an explicit role and a wait handle, so a
// follower can select on it alongside its own ticker instead of blocking inside Do.
//
// Billing is NOT decided here. Each caller that cares about AI credits gates and debits for
// itself, whether it ends up leading or following — see Request.Chargeable on why that is safe
// to do independently without double-charging. Service.Ensure never touches credits at all,
// leader or follower, which is what keeps the cold-start pre-run free.
//
// Zero value is ready to use — the map is created lazily under the lock.
type coordinator struct {
	mu       sync.Mutex
	inflight map[analysisKey]*analysisRun
}

// analysisKey is the pair a compute is coalesced on. Scoped to one candidate: two
// candidates analysing the same vacancy at once are not the same input (different résumé,
// different blockers) and must never coalesce.
type analysisKey struct {
	userID, jobID int64
}

// analysisRun is what a follower waits on: done closes once the leader's compute ends, and
// succeeded — safe to read only after that, never before — reports whether the cache is left
// in a trustworthy state (a fresh compute, or one found already cached) versus a failed
// attempt that left nothing new behind. A follower that skips this check and reads the cache
// unconditionally can serve a stale row as if it were this run's result — see Service.Follow's
// own comment for the failure this caused live.
type analysisRun struct {
	done      chan struct{}
	succeeded bool
}

// Claim is one caller's role in the coalesced compute for a (candidate, job) pair.
//
// The LEADER owns running the chain and MUST call Release exactly once when its compute ends
// — success, failure, or "LLM unconfigured" all count — or every follower blocks forever.
// Release is safe to call more than once (only the first call has any effect): a future edit
// that adds one more early return before an existing Release must not turn into a double-close
// panic, which — running from an async SSE body-writer goroutine fasthttp does not recover
// panics in — would take the whole process down, not just the one request.
//
// A FOLLOWER holds the same underlying run and waits on it. Claim never blocks: a follower
// decides for itself where to wait, and both of today's callers wait from inside their own SSE
// body-writer rather than from the request — the streaming one so the request that is about to
// open the stream is not blocked, the autopilot one because a wait of a whole chain's duration
// in the handler is silence a proxy severs.
type Claim struct {
	leader  bool
	release func(succeeded bool)
	run     *analysisRun
}

// IsLeader reports whether this caller owns running the chain.
func (c *Claim) IsLeader() bool { return c != nil && c.leader }

// Release ends the leader's compute and wakes every follower. No-op for a follower.
func (c *Claim) Release(succeeded bool) {
	if c == nil || c.release == nil {
		return
	}
	c.release(succeeded)
}

// wait blocks until the leader's compute ends, then reports whether the cache is left in a
// trustworthy state. The wait carries no cancellation and takes no ctx: nothing that calls
// this needs it to be interruptible — both callers already run under a background context by
// the time they wait, deliberately, so a client that disconnects costs nothing extra.
func (c *Claim) wait() bool {
	if c == nil || c.run == nil {
		return false
	}
	<-c.run.done
	return c.run.succeeded
}

// Claim claims (userID, jobID) for the calling goroutine, returning the leader's claim or a
// follower's handle on the leader's run.
func (co *coordinator) Claim(userID, jobID int64) *Claim {
	key := analysisKey{userID, jobID}
	co.mu.Lock()
	if existing, running := co.inflight[key]; running {
		co.mu.Unlock()
		return &Claim{run: existing}
	}
	run := &analysisRun{done: make(chan struct{})}
	if co.inflight == nil {
		co.inflight = make(map[analysisKey]*analysisRun)
	}
	co.inflight[key] = run
	co.mu.Unlock()

	var once sync.Once
	return &Claim{
		leader: true,
		run:    run,
		release: func(succeeded bool) {
			once.Do(func() {
				co.mu.Lock()
				delete(co.inflight, key)
				co.mu.Unlock()
				// Written before the close, which is what makes it safe for a follower to read
				// right after the wait unblocks: a channel close happens-before a receive that
				// observes it, per the Go memory model, so this write is visible by then with no
				// extra lock needed on the follower's side.
				run.succeeded = succeeded
				close(run.done)
			})
		},
	}
}
