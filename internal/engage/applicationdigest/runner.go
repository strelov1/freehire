// Package applicationdigest delivers an opt-in weekly summary of application activity.
package applicationdigest

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

type Followup struct {
	ID                                     int64
	RoleTitle, CompanySlug, JobSlug, Stage string
	DaysSilent                             int
	FollowedUpAt                           time.Time
}
type Snapshot struct {
	ActivityCount                                          int64
	AsOf                                                   time.Time
	AppliedCount, RepliesCount, InterviewCount, OfferCount int64
	StageCounts                                            map[string]int64
	Followups                                              []Followup
}

func (s Snapshot) Empty() bool {
	return s.ActivityCount+s.AppliedCount+s.RepliesCount+s.InterviewCount+s.OfferCount == 0 && len(s.Followups) == 0
}

type Store interface {
	Candidates(context.Context, time.Time, int64, int32) ([]Candidate, error)
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
	var stats Stats
	var after int64
	for {
		candidates, err := r.store.Candidates(ctx, now.Add(time.Since(started)), after, BatchSize)
		if err != nil {
			return stats, err
		}
		for _, c := range candidates {
			after = c.UserID
			at := now.Add(time.Since(started))
			snapshot, err := r.store.Snapshot(ctx, c, at)
			if err != nil {
				stats.Failed++
				log.Printf("application-digest: user %d snapshot: %v", c.UserID, err)
				continue
			}
			// Recheck the due week against the instant after the snapshot.
			at = now.Add(time.Since(started))
			if snapshot.Empty() {
				continue
			}
			message, err := r.mailer.Render(c, snapshot)
			if err != nil {
				stats.Failed++
				log.Printf("application-digest: user %d render: %v", c.UserID, err)
				continue
			}
			claimed, err := r.store.Claim(ctx, c, snapshot, at)
			if err != nil {
				stats.Failed++
				log.Printf("application-digest: user %d claim: %v", c.UserID, err)
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
						log.Printf("application-digest: user %d release: %v", c.UserID, releaseErr)
					} else {
						stats.Skipped++
					}
					return stats, nil
				}
				stats.Failed++
				log.Printf("application-digest: user %d send: %v", c.UserID, err)
				continue
			}
			// The mail is out: a shutdown now must not leave the week unrecorded.
			finalize, cancel := detached(ctx)
			err = r.store.MarkSent(finalize, c, at)
			cancel()
			if err != nil {
				stats.Failed++
				log.Printf("application-digest: user %d mark sent: %v", c.UserID, err)
				continue
			}
			stats.Sent++
		}
		if len(candidates) < int(BatchSize) {
			return stats, nil
		}
	}
}

// detached outlives the run's cancellation for a short, bounded write.
func detached(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
}
