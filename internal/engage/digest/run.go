// Package digest runs one email digest pass: snapshot → render → claim → send →
// record. The inbox and application digests differ only in their snapshot, SQL and mail,
// so each keeps those and shares this loop.
package digest

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/engage/emailnotify"
)

// BatchSize is how many accounts one page of recipients holds.
const BatchSize int32 = 200

// Candidate is one account a digest may be due to. Both digests select the same columns.
type Candidate struct {
	UserID    int64
	Email     string
	Timezone  string
	EnabledAt time.Time
}

type Stats struct{ Sent, Failed, Skipped int }

// Snapshot is what one digest reads for an account. Empty snapshots claim nothing.
type Snapshot interface{ Empty() bool }

// Store is the digest-specific persistence: who is due, what to show, and the ledger.
type Store[S Snapshot] interface {
	// Candidates lists due accounts after the cursor, at most BatchSize; the second
	// result is false when no page follows.
	Candidates(ctx context.Context, at time.Time, after int64) ([]Candidate, bool, error)
	Snapshot(ctx context.Context, c Candidate, at time.Time) (S, error)
	Claim(ctx context.Context, c Candidate, s S, at time.Time) (bool, error)
	MarkSent(ctx context.Context, c Candidate, at time.Time) error
	ReleasePaused(ctx context.Context, c Candidate, at time.Time) error
}

// Run delivers one pass of a digest. Paging goes through the store, so a digest that
// caps itself at one batch stops there. A pause ends the pass: every later send would
// hit the same pause.
func Run[S Snapshot](ctx context.Context, now time.Time, name string, store Store[S], sender emailnotify.Sender,
	render func(c Candidate, s S) (emailnotify.Message, error)) (Stats, error) {
	started := time.Now() // the selection query's time counts too
	var stats Stats
	var after int64
	for {
		candidates, more, err := store.Candidates(ctx, now.Add(time.Since(started)), after)
		if err != nil {
			return stats, err
		}
		for _, c := range candidates {
			after = c.UserID
			at := now.Add(time.Since(started))
			snapshot, err := store.Snapshot(ctx, c, at)
			if err != nil {
				stats.Failed++
				log.Printf("%s: user %d snapshot: %v", name, c.UserID, err)
				continue
			}
			// Claim against the instant after the snapshot, so a run crossing local
			// midnight (or the week's start) books the new day.
			at = now.Add(time.Since(started))
			if snapshot.Empty() {
				continue
			}
			message, err := render(c, snapshot)
			if err != nil {
				stats.Failed++
				log.Printf("%s: user %d render: %v", name, c.UserID, err)
				continue
			}
			claimed, err := store.Claim(ctx, c, snapshot, at)
			if err != nil {
				stats.Failed++
				log.Printf("%s: user %d claim: %v", name, c.UserID, err)
				continue
			}
			if !claimed {
				continue
			}
			if err = sender.Send(ctx, message); err != nil {
				if errors.Is(err, emailnotify.ErrSendingPaused) {
					// No request was dispatched. Unlike an uncertain network result, this can be retried today.
					cleanup, cancel := detached(ctx)
					releaseErr := store.ReleasePaused(cleanup, c, at)
					cancel()
					if releaseErr != nil {
						stats.Failed++
						log.Printf("%s: user %d release: %v", name, c.UserID, releaseErr)
					} else {
						stats.Skipped++
					}
					return stats, nil
				}
				stats.Failed++
				log.Printf("%s: user %d send: %v", name, c.UserID, err)
				continue
			}
			// The mail is out: a shutdown now must not leave the period unrecorded and resend it.
			finalize, cancel := detached(ctx)
			err = store.MarkSent(finalize, c, at)
			cancel()
			if err != nil {
				stats.Failed++
				log.Printf("%s: user %d mark sent: %v", name, c.UserID, err)
				continue
			}
			stats.Sent++
		}
		if !more {
			return stats, nil
		}
	}
}

// detached outlives the run's cancellation for a short, bounded write.
func detached(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
}
