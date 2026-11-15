package changehook

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

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
	// concurrency bounds one run: that many subscriptions are delivered at a time.
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

// Run delivers every due subscription, each from its stored position until it catches up with the
// feed. The feed query is the public feed's (ListJobChanges, here with company_slug): same ordering,
// "finished transactions only" bound and privacy rule, so a subscriber sees what GET /jobs/changes would.
func (w *Worker) Run(ctx context.Context) (Stats, error) {
	var st struct{ subs, changes, failed, disabled atomic.Int64 }
	started := pgtype.Timestamptz{Time: time.Now(), Valid: true}
	// Database errors fail the run (exit 1); an endpoint refusing is a delivery outcome.
	var errMu sync.Mutex
	var errs []error
	record := func(err error) {
		if err != nil {
			errMu.Lock()
			errs = append(errs, err)
			errMu.Unlock()
		}
	}
	var wg sync.WaitGroup
	for range concurrency {
		wg.Go(func() {
			// Each worker claims one subscription when it is free, so a lease starts when
			// delivery starts and never expires while the row waits in a queue.
			for ctx.Err() == nil {
				subs, err := w.q.ClaimDueChangeWebhooks(ctx, db.ClaimDueChangeWebhooksParams{LeaseSecs: leaseSecs, RowLimit: 1, DueAt: started})
				if err != nil {
					record(err)
					return
				}
				if len(subs) == 0 {
					return
				}
				n, outcome, err := w.deliver(ctx, subs[0])
				record(err)
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
	wg.Wait()
	stats := Stats{st.subs.Load(), st.changes.Load(), st.failed.Load(), st.disabled.Load()}
	return stats, errors.Join(append(errs, ctx.Err())...)
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
func (w *Worker) deliver(ctx context.Context, sub db.ClaimDueChangeWebhooksRow) (int64, outcome, error) {
	f := Filter{CompanySlugs: sub.CompanySlugs, Kinds: sub.Kinds}
	xid, id := sub.CursorXid, sub.CursorID
	var sent int64
	delivered := false
	var readErr error
	for range maxPages {
		rows, err := w.q.ListJobChangesForWebhook(ctx, db.ListJobChangesForWebhookParams{AfterXid: xid, AfterID: id, RowLimit: pageSize})
		if err != nil {
			readErr = fmt.Errorf("subscription %d: read feed: %w", sub.ID, err)
			break // the position so far is still stored below; not the endpoint's fault
		}
		if len(rows) == 0 {
			break
		}
		last := rows[len(rows)-1]
		if batch := f.Batch(rows); len(batch) > 0 {
			err := w.send.Send(ctx, sub.URL, sub.Secret, Event{ID: deliveryID(sub.ID, last), Type: EventChanges, Changes: batch})
			if err != nil {
				// Keep the pages the endpoint already accepted; the failure then counts from there,
				// as the stored count does once the advance has reset it.
				var advErr error
				if delivered {
					if advErr = w.advance(ctx, sub.ID, xid, id, true); advErr == nil {
						sub.ConsecutiveFailures = 0
					}
				}
				o, failErr := w.fail(ctx, sub, err)
				return sent, o, errors.Join(advErr, failErr)
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
	return sent, outcomeOK, errors.Join(readErr, w.advance(ctx, sub.ID, xid, id, delivered))
}

func (w *Worker) fail(ctx context.Context, sub db.ClaimDueChangeWebhooksRow, cause error) (outcome, error) {
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
		return outcomeFailed, fmt.Errorf("subscription %d: record failure: %w", sub.ID, err)
	}
	return outcomeFailed, nil
}

func (w *Worker) disable(ctx context.Context, sub db.ClaimDueChangeWebhooksRow, reason, lastErr string) (outcome, error) {
	if err := w.q.DisableChangeWebhook(ctx, db.DisableChangeWebhookParams{ID: sub.ID, Reason: reason, LastError: lastErr}); err != nil {
		return outcomeDisabled, fmt.Errorf("subscription %d: disable: %w", sub.ID, err)
	}
	return outcomeDisabled, nil
}

func (w *Worker) advance(ctx context.Context, id int64, xid, cid int64, delivered bool) error {
	if err := w.q.AdvanceChangeWebhook(context.WithoutCancel(ctx), db.AdvanceChangeWebhookParams{
		ID: id, CursorXid: xid, CursorID: cid, Delivered: delivered,
	}); err != nil {
		return fmt.Errorf("subscription %d: store position: %w", id, err)
	}
	return nil
}
