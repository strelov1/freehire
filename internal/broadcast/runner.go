package broadcast

import (
	"context"
	"fmt"
	"log"

	"github.com/strelov1/freehire/internal/db"
)

// Store is the slice of the database a campaign needs.
type Store interface {
	ListBroadcastCandidates(ctx context.Context, arg db.ListBroadcastCandidatesParams) ([]db.ListBroadcastCandidatesRow, error)
	CountBroadcastCandidates(ctx context.Context, campaign string) (int64, error)
	RecordBroadcastEmail(ctx context.Context, arg db.RecordBroadcastEmailParams) error
}

// DefaultMaxPerRun bounds one run. A campaign reaches the entire audience, so the
// cap is not about protecting the database — it is about how much of an untested
// letter can go out before a human sees the result.
const DefaultMaxPerRun = 500

// Runner sends one campaign.
type Runner struct {
	store  Store
	mailer *Mailer
	max    int32
}

// New builds a Runner. maxPerRun of zero means DefaultMaxPerRun.
func New(store Store, mailer *Mailer, maxPerRun int32) *Runner {
	if maxPerRun <= 0 {
		maxPerRun = DefaultMaxPerRun
	}
	return &Runner{store: store, mailer: mailer, max: maxPerRun}
}

// Stats is what one run did.
type Stats struct {
	Sent      int
	Failed    int
	Remaining int64
}

// Pending reports how many people the campaign would reach right now, without
// sending anything.
func (r *Runner) Pending(ctx context.Context, c Campaign) (int64, error) {
	return r.store.CountBroadcastCandidates(ctx, c.Name)
}

// Run sends the campaign to one capped batch and reports what is left.
//
// A failed send is recorded and stepped over, exactly as in the onboarding
// sequence: one address that SES rejects must not strand the rest of the list, and
// retrying it on the next run would spend the domain's reputation on an address
// that has already refused once.
func (r *Runner) Run(ctx context.Context, c Campaign) (Stats, error) {
	rows, err := r.store.ListBroadcastCandidates(ctx, db.ListBroadcastCandidatesParams{
		Campaign: c.Name,
		MaxRows:  r.max,
	})
	if err != nil {
		return Stats{}, fmt.Errorf("broadcast: listing %s candidates: %w", c.Name, err)
	}

	var stats Stats
	for _, row := range rows {
		sendErr := r.mailer.Send(ctx, c, row.Email)

		var errText string
		if sendErr != nil {
			errText = sendErr.Error()
			stats.Failed++
			log.Printf("broadcast: %s to user %d failed: %v", c.Name, row.ID, sendErr)
		} else {
			stats.Sent++
		}

		if err := r.store.RecordBroadcastEmail(ctx, db.RecordBroadcastEmailParams{
			UserID:   row.ID,
			Campaign: c.Name,
			Error:    errText,
		}); err != nil {
			// Sent but unrecorded: the next run will send it again. Loud, because a
			// duplicate campaign mail is the most visible thing this package can do.
			log.Printf("broadcast: recording %s for user %d failed (mail may repeat): %v", c.Name, row.ID, err)
		}
	}

	stats.Remaining, err = r.store.CountBroadcastCandidates(ctx, c.Name)
	if err != nil {
		// The batch went out; only the tally is missing. Report what is known.
		log.Printf("broadcast: counting the remainder of %s failed: %v", c.Name, err)
	}
	return stats, nil
}
