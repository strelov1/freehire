package fitanalysis

import "sync"

// coordinator coalesces concurrent fit-analysis computes for the same (candidate, job) pair
// across the entry points that can each start the three-stage chain: Service.Ensure (the
// autopilot's cold-start pre-run) and Service.Run behind the tailoring workspace's stepper.
// Both can reach here for one pair within milliseconds; without coalescing that is two chains
// of wasted spend racing to upsert the same row.
//
// Hand-rolled rather than singleflight: singleflight.Do blocks leader and followers alike for
// the whole compute, but a streaming follower must keep its own SSE heartbeat ticking while it
// waits, and must never touch the LLM (it reads what the leader left in the cache). Claim
// returns immediately with a role and a wait handle, so a follower can select on it alongside
// its own ticker.
//
// Billing is NOT decided here: each caller that cares gates and debits for itself (see
// Request.Reserved). Service.Ensure never touches credits, which keeps the cold-start pre-run
// free.
//
// Zero value is ready to use; the map is created lazily under the lock.
type coordinator struct {
	mu       sync.Mutex
	inflight map[analysisKey]*analysisRun
}

// analysisKey is the pair a compute is coalesced on. Scoped to one candidate: different
// candidates analysing the same vacancy have different CVs and blockers and must not coalesce.
type analysisKey struct {
	userID, jobID int64
}

// analysisRun is what a follower waits on: done closes once the leader's compute ends, and
// succeeded (read only after done) reports whether the cache holds a trustworthy result (fresh
// compute or already cached) rather than a failed attempt that left nothing new. A follower
// that skips the check can serve a stale row as this run's result; see Service.Follow.
type analysisRun struct {
	done      chan struct{}
	succeeded bool
}

// Claim is one caller's role in the coalesced compute for a (candidate, job) pair.
//
// The LEADER owns running the chain and MUST call Release exactly once when its compute ends
// (success, failure, or "LLM unconfigured"), or every follower blocks forever. Release is safe
// to call more than once: it runs from an SSE body-writer goroutine fasthttp does not recover
// panics in, so a double-close would take the whole process down.
//
// A FOLLOWER holds the same run and waits on it. Claim never blocks: both callers wait from
// inside their own SSE body-writer, the streaming one so the request opening the stream is not
// blocked, the autopilot one because a chain-length wait in the handler is silence a proxy
// severs.
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
// trustworthy state. It takes no ctx: both callers already run under a background context, so
// a client disconnect costs nothing extra.
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
				// Written before the close so a follower reading after the wait sees it (a channel
				// close happens-before the receive that observes it).
				run.succeeded = succeeded
				close(run.done)
			})
		},
	}
}
