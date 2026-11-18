package pipeline

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// pause is which of a board's pauses, if any, holds it back this run.
type pause int

const (
	notPaused pause = iota
	pausedCooldown
	pausedQuiet
)

// paused reports whether a board is currently backed off — in failure cooldown, or in the
// quiet pause of a board whose listing keeps coming back unchanged. A cooldown wins when both
// apply, since a failing board is the one an operator needs to see. It fails OPEN: a health
// lookup error is logged and the board is crawled anyway, so a health-store hiccup never stalls
// ingest. Always notPaused when no BoardHealth port is wired.
func (r Runner) paused(ctx context.Context, e sources.CompanyEntry) pause {
	if r.BoardHealth == nil {
		return notPaused
	}
	p, err := r.BoardHealth.Pauses(ctx, e.Provider, e.Board, e.Region)
	if err != nil {
		log.Printf("ingest: cooldown check %s/%s: %v", e.Provider, e.Board, err)
		return notPaused
	}
	now := time.Now()
	switch {
	case p.Cooldown.After(now):
		return pausedCooldown
	case p.Quiet.After(now):
		return pausedQuiet
	}
	return notPaused
}

// limiterDeadlineRefusal is what golang.org/x/time/rate's Limiter.Wait returns when the next
// token falls after the context's deadline. It refuses at once, AHEAD of the deadline, so the
// context still reports nothing — and the package exports no sentinel to match instead.
// TestRunCountsALimiterDeadlineRefusalAsInterrupted produces the error with a real limiter, so
// a change to the wording fails that test rather than silently cooling boards again.
const limiterDeadlineRefusal = "would exceed context deadline"

// errLimiterOutOfRun is Run's error when a rate limiter refused a board's request ahead of the
// deadline. It wraps context.DeadlineExceeded because that is what happened, one step early.
var errLimiterOutOfRun = fmt.Errorf("a rate limiter could not admit a request before the run's deadline: %w", context.DeadlineExceeded)

// limiterOutOfRun reports whether a fetch failed because a rate limiter could not admit its
// request before the run's deadline: the run ran out, not the board. Every paced adapter's
// queued boards fail this way in a run's last seconds, and recorded as failures they were
// cooled run after run (phenom, 2026-10-30: 160 of 383 boards).
func limiterOutOfRun(err error) bool {
	return strings.Contains(err.Error(), limiterDeadlineRefusal)
}

// boardHealthTimeout bounds one board-health write. The write runs on a context detached from
// the crawl's (see healthContext), so this is the only thing that stops a hung database from
// holding the run past its deadline.
const boardHealthTimeout = 10 * time.Second

// healthContext detaches a board-health write from the crawl's cancellation. The crawl's context
// is the one that ends at cmd/ingest's crawl deadline, and a board still in flight then — a big
// board whose fetch finished but whose writes did not — used to lose its health row with the
// rest: the write was refused with "context deadline exceeded", so a board that had answered
// looked as if it had never been crawled. The outcome is a fact about the board and is known by
// then; recording it is not crawl work the deadline exists to stop.
func healthContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), boardHealthTimeout)
}

// recordSuccess / recordFailure persist a board's outcome best-effort: a health-store
// error is logged and never propagated (health must not fail ingest), and both no-op
// when no BoardHealth port is wired.
//
// A board that answered is recorded as a success even when the crawl's context ended while its
// postings were being written: the board did its part, and a cut-short write is the run's
// budget, not the board's health. A failure seen AFTER the crawl's context ended is not
// recorded at all, because it cannot be told apart from the deadline itself cutting the fetch
// short — and counting it would cool a healthy board (CooldownFor, from the third in a row) and
// cool every board in flight at once on a provider whose run regularly meets its deadline.
// Recording nothing leaves the row as it was, which is what the board earned this run.
func (r Runner) recordSuccess(ctx context.Context, e sources.CompanyEntry, ingested int, reached bool, fingerprint string) {
	if r.BoardHealth == nil || r.recent() {
		return
	}
	hctx, cancel := healthContext(ctx)
	defer cancel()
	if err := r.BoardHealth.RecordSuccess(hctx, e.Provider, e.Board, e.Region, ingested, reached, fingerprint); err != nil {
		log.Printf("ingest: record board success %s/%s: %v", e.Provider, e.Board, err)
	}
}

func (r Runner) recordFailure(ctx context.Context, e sources.CompanyEntry, cause error) {
	if r.BoardHealth == nil || r.recent() {
		return
	}
	if ctx.Err() != nil {
		log.Printf("ingest: %s board %q: failure not recorded, the crawl stopped first (%v): %v", e.Provider, e.Board, ctx.Err(), cause)
		return
	}
	hctx, cancel := healthContext(ctx)
	defer cancel()
	if err := r.BoardHealth.RecordFailure(hctx, e.Provider, e.Board, e.Region, cause); err != nil {
		log.Printf("ingest: record board failure %s/%s: %v", e.Provider, e.Board, err)
	}
}
