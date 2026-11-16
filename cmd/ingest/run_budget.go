package main

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

// defaultSweepReserve is how much of the run's timeout is held back from the crawl for the
// sweep. Ten minutes covers the slowest common sweep (workday); a provider whose sweep runs
// longer sets INGEST_SWEEP_RESERVE_SECONDS rather than taxing every crawl.
const defaultSweepReserve = 10 * time.Minute

// maxWriteReserve bounds how much of the crawl is kept for writing what boards still in flight
// at its deadline had read (pipeline.Runner.WriteReserve); five minutes writes the largest
// boards.
const maxWriteReserve = 5 * time.Minute

// runBudget splits a run's timeout between the crawl and the sweep that follows it.
//
// The unit's timeout is a hard kill (SIGTERM, then in-flight work dies). When the crawl ran
// long the sweep was what died, leaving dead postings open. So the crawl gets its own deadline,
// reserve short of the timeout, and the sweep has the reserve to itself.
//
// timeout is zero when INGEST_RUN_TIMEOUT_SECONDS is unset (a process cannot read its unit's
// TimeoutStartSec), and the crawl then has no deadline. The scheduler passes it with every
// launch (ingestsched.SystemdLauncher); a static unit must set it, net of any wrapper wait.
type runBudget struct {
	timeout time.Duration
	reserve time.Duration
}

// runBudgetFor resolves INGEST_RUN_TIMEOUT_SECONDS and INGEST_SWEEP_RESERVE_SECONDS. A set
// value must be a positive number of seconds, and the reserve must leave the crawl something:
// one at or past the timeout would end every crawl before its first board.
func runBudgetFor(timeoutEnv, reserveEnv string) (runBudget, error) {
	b := runBudget{reserve: defaultSweepReserve}
	if reserveEnv != "" {
		secs, err := strconv.Atoi(reserveEnv)
		if err != nil || secs <= 0 {
			return runBudget{}, fmt.Errorf("INGEST_SWEEP_RESERVE_SECONDS must be a positive number of seconds, got %q", reserveEnv)
		}
		b.reserve = time.Duration(secs) * time.Second
	}
	if timeoutEnv == "" {
		return b, nil
	}
	secs, err := strconv.Atoi(timeoutEnv)
	if err != nil || secs <= 0 {
		return runBudget{}, fmt.Errorf("INGEST_RUN_TIMEOUT_SECONDS must be a positive number of seconds, got %q", timeoutEnv)
	}
	b.timeout = time.Duration(secs) * time.Second
	if b.reserve >= b.timeout {
		return runBudget{}, fmt.Errorf("INGEST_SWEEP_RESERVE_SECONDS (%s) must be shorter than INGEST_RUN_TIMEOUT_SECONDS (%s), or no crawl can start",
			b.reserve, b.timeout)
	}
	return b, nil
}

// crawlContext bounds the crawl at reserve short of the run's timeout. Without a known timeout
// it is parent, cancellable.
func (b runBudget) crawlContext(parent context.Context, started time.Time) (context.Context, context.CancelFunc) {
	if b.timeout == 0 {
		return context.WithCancel(parent)
	}
	return context.WithDeadline(parent, started.Add(b.timeout-b.reserve))
}

// writeReserve is the crawl's share kept for the writes after its fetches end: maxWriteReserve,
// but at most a quarter of the crawl. Zero when the run has no known timeout.
func (b runBudget) writeReserve() time.Duration {
	if b.timeout == 0 {
		return 0
	}
	return min(maxWriteReserve, (b.timeout-b.reserve)/4)
}

// sweepContext is what the sweep runs on: detached from parent's cancellation, so neither the
// crawl's deadline nor a stopping signal cancels a close, and bounded by the run's timeout
// (the unit kills the process then anyway). Without a known timeout it is bounded by the
// reserve from now.
//
// Detaching from the signal is deliberate: a close is one atomic statement, so a later kill
// only rolls back the one in flight, while honouring the signal would discard earned closes.
func (b runBudget) sweepContext(parent context.Context, started time.Time) (context.Context, context.CancelFunc) {
	detached := context.WithoutCancel(parent)
	if b.timeout == 0 {
		return context.WithTimeout(detached, b.reserve)
	}
	return context.WithDeadline(detached, started.Add(b.timeout))
}
