// Package inboxdigest delivers an opt-in daily summary of classified hiring mail.
package inboxdigest

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/engage/emailnotify"
)

const BatchSize int32 = 200

type Candidate struct {
	UserID    int64
	Email     string
	Timezone  string
	EnabledAt time.Time
}

type Item struct {
	ID         int64     `json:"id"`
	Subject    string    `json:"subject"`
	Sender     string    `json:"sender"`
	Snippet    string    `json:"snippet"`
	Signal     string    `json:"signal"`
	ReceivedAt time.Time `json:"received_at"`
	Replied    bool      `json:"replied"`
}

type SchedulingActivity struct {
	ID      int64  `json:"id"`
	Subject string `json:"subject"`
	Kind    string `json:"kind"`
}

type Snapshot struct {
	IDs         []int64
	Total       int64
	Items       []Item
	ActivityIDs []int64
	Activity    []SchedulingActivity
}

type Store interface {
	Candidates(context.Context, time.Time, int32) ([]Candidate, error)
	Snapshot(context.Context, Candidate, time.Time) (Snapshot, error)
	Claim(context.Context, Candidate, Snapshot, time.Time) (bool, error)
	MarkSent(context.Context, Candidate, time.Time) error
	ReleasePaused(context.Context, Candidate, time.Time) error
}

type Stats struct{ Sent, Failed, Skipped int }

type Runner struct {
	store  Store
	mailer *Mailer
	sender emailnotify.Sender
}

func New(store Store, mailer *Mailer, sender emailnotify.Sender) *Runner {
	return &Runner{store: store, mailer: mailer, sender: sender}
}

func (r *Runner) Run(ctx context.Context, now time.Time) (Stats, error) {
	started := time.Now() // the selection query's time counts too
	candidates, err := r.store.Candidates(ctx, now, BatchSize)
	if err != nil {
		return Stats{}, err
	}
	var stats Stats
	for _, c := range candidates {
		at := now.Add(time.Since(started))
		snapshot, err := r.store.Snapshot(ctx, c, at)
		if err != nil {
			stats.Failed++
			log.Printf("inbox-digest: user %d snapshot: %v", c.UserID, err)
			continue
		}
		// Claim against the instant after the snapshot, so a run crossing local midnight books the new day.
		at = now.Add(time.Since(started))
		if len(snapshot.Items) == 0 && len(snapshot.Activity) == 0 {
			continue
		}
		message, err := r.mailer.Render(c, snapshot)
		if err != nil {
			stats.Failed++
			log.Printf("inbox-digest: user %d render: %v", c.UserID, err)
			continue
		}
		claimed, err := r.store.Claim(ctx, c, snapshot, at)
		if err != nil {
			stats.Failed++
			log.Printf("inbox-digest: user %d claim: %v", c.UserID, err)
			continue
		}
		if !claimed {
			continue
		}
		if err = r.sender.Send(ctx, message); err != nil {
			if errors.Is(err, emailnotify.ErrSendingPaused) {
				// No request was dispatched. Unlike an uncertain network result, this can be retried today.
				cleanup, cancel := detached(ctx)
				releaseErr := r.store.ReleasePaused(cleanup, c, at)
				cancel()
				if releaseErr != nil {
					stats.Failed++
					log.Printf("inbox-digest: user %d release: %v", c.UserID, releaseErr)
				} else {
					stats.Skipped++
				}
				break
			}
			stats.Failed++
			log.Printf("inbox-digest: user %d send: %v", c.UserID, err)
			continue
		}
		// The mail is out: a shutdown now must not leave the day unrecorded and resend it tomorrow.
		finalize, cancel := detached(ctx)
		err = r.store.MarkSent(finalize, c, at)
		cancel()
		if err != nil {
			stats.Failed++
			log.Printf("inbox-digest: user %d mark sent: %v", c.UserID, err)
			continue
		}
		stats.Sent++
	}
	return stats, nil
}

// detached outlives the run's cancellation for a short, bounded write.
func detached(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
}
