package changehook

import (
	"context"
	"errors"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/strelov1/freehire/internal/platform/db"
)

const (
	// maxFailures consecutive failed deliveries disable a subscription: with the backoff
	// capped at an hour that is about a day of an endpoint being down.
	maxFailures = 25
	// leaseSecs hides a claimed subscription from other runs while this one delivers it.
	leaseSecs = 300
	// maxPages bounds one subscription's catch-up per run, so one slow mirror cannot hold
	// the run; the rest is read on the next one.
	maxPages = 20
	// claimLimit and concurrency bound one run.
	claimLimit  = 500
	concurrency = 8
)

// Worker is one delivery pass over the due subscriptions.
type Worker struct {
	q    *db.Queries
	send *Sender
}

func NewWorker(q *db.Queries, send *Sender) *Worker { return &Worker{q: q, send: send} }

// Stats is what one run did. Failed counts subscriptions whose delivery failed, Disabled
// those it gave up on.
type Stats struct{ Subscriptions, Changes, Failed, Disabled int64 }

// Run delivers every due subscription, each from its stored position until it catches up with
// the feed. The feed query is the public feed's (ListJobChanges): the same ordering, the same
// "finished transactions only" bound and the same privacy rule, so a subscriber sees exactly
// what a client polling GET /jobs/changes would.
func (w *Worker) Run(ctx context.Context) (Stats, error) {
	subs, err := w.q.ClaimDueChangeWebhooks(ctx, db.ClaimDueChangeWebhooksParams{LeaseSecs: leaseSecs, RowLimit: claimLimit})
	if err != nil {
		return Stats{}, err
	}
	var st struct{ subs, changes, failed, disabled atomic.Int64 }
	jobs := make(chan db.ClaimDueChangeWebhooksRow)
	var wg sync.WaitGroup
	for range concurrency {
		wg.Go(func() {
			for sub := range jobs {
				n, outcome := w.deliver(ctx, sub)
				st.subs.Add(1)
				st.changes.Add(n)
				switch outcome {
				case outcomeFailed:
					st.failed.Add(1)
				case outcomeDisabled:
					st.failed.Add(1)
					st.disabled.Add(1)
				}
			}
		})
	}
	for _, s := range subs {
		jobs <- s
	}
	close(jobs)
	wg.Wait()
	return Stats{st.subs.Load(), st.changes.Load(), st.failed.Load(), st.disabled.Load()}, ctx.Err()
}

type outcome int

const (
	outcomeOK outcome = iota
	outcomeFailed
	outcomeDisabled
)

// deliver walks one subscription forward page by page. The position moves only after the
// endpoint accepted the page's batch (or the page held nothing it wanted), so a failure
// repeats the same batch: at-least-once, deduplicated by the receiver through seq.
func (w *Worker) deliver(ctx context.Context, sub db.ClaimDueChangeWebhooksRow) (int64, outcome) {
	f := Filter{CompanySlugs: sub.CompanySlugs, Kinds: sub.Kinds}
	xid, id := sub.CursorXid, sub.CursorID
	var sent int64
	delivered := false
	for range maxPages {
		rows, err := w.q.ListJobChanges(ctx, db.ListJobChangesParams{AfterXid: xid, AfterID: id, RowLimit: pageSize})
		if err != nil {
			log.Printf("changehook: subscription %d: read feed: %v", sub.ID, err)
			break // the lease expires and the next run retries; not the endpoint's fault
		}
		if len(rows) == 0 {
			break
		}
		last := rows[len(rows)-1]
		if batch := f.Batch(rows); len(batch) > 0 {
			err := w.send.Send(ctx, sub.URL, sub.Secret, Event{ID: deliveryID(sub.ID, last), Type: EventChanges, Changes: batch})
			if err != nil {
				return sent, w.fail(ctx, sub, err)
			}
			sent += int64(len(batch))
			delivered = true
		}
		xid, id = last.Xid, last.ID
		if len(rows) < pageSize {
			break
		}
	}
	// Always: it stores the position and releases the lease, so the next run is not held off.
	if err := w.q.AdvanceChangeWebhook(context.WithoutCancel(ctx), db.AdvanceChangeWebhookParams{
		ID: sub.ID, CursorXid: xid, CursorID: id, Delivered: delivered,
	}); err != nil {
		log.Printf("changehook: subscription %d: store position: %v", sub.ID, err)
	}
	return sent, outcomeOK
}

func (w *Worker) fail(ctx context.Context, sub db.ClaimDueChangeWebhooksRow, cause error) outcome {
	ctx = context.WithoutCancel(ctx) // a run cut short still records what happened
	msg := cause.Error()
	switch {
	case errors.Is(cause, ErrGone):
		return w.disable(ctx, sub, "endpoint answered 410 Gone", msg)
	case sub.ConsecutiveFailures+1 >= maxFailures:
		return w.disable(ctx, sub, "endpoint kept failing", msg)
	}
	if err := w.q.FailChangeWebhook(ctx, db.FailChangeWebhookParams{
		ID: sub.ID, LastError: msg, BackoffSecs: int64(backoff(sub.ConsecutiveFailures) / time.Second),
	}); err != nil {
		log.Printf("changehook: subscription %d: record failure: %v", sub.ID, err)
	}
	return outcomeFailed
}

func (w *Worker) disable(ctx context.Context, sub db.ClaimDueChangeWebhooksRow, reason, lastErr string) outcome {
	if err := w.q.DisableChangeWebhook(ctx, db.DisableChangeWebhookParams{ID: sub.ID, Reason: reason, LastError: lastErr}); err != nil {
		log.Printf("changehook: subscription %d: disable: %v", sub.ID, err)
	}
	return outcomeDisabled
}
