package broadcast

import (
	"context"
	"fmt"
	"log"

	"github.com/strelov1/freehire/internal/engage/maildaily"
	"github.com/strelov1/freehire/internal/platform/db"
)

// Store is the slice of the database a campaign needs.
type Store interface {
	maildaily.Store
	ListBroadcastCandidates(ctx context.Context, arg db.ListBroadcastCandidatesParams) ([]db.ListBroadcastCandidatesRow, error)
	CountBroadcastCandidates(ctx context.Context, arg db.CountBroadcastCandidatesParams) (int64, error)
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
	claims *maildaily.Claimer
	max    int32
}

// New builds a Runner. maxPerRun of zero means DefaultMaxPerRun.
func New(store Store, mailer *Mailer, maxPerRun int32) *Runner {
	if maxPerRun <= 0 {
		maxPerRun = DefaultMaxPerRun
	}
	return &Runner{store: store, mailer: mailer, claims: maildaily.New(store, maildaily.SourceBroadcast), max: maxPerRun}
}

// ValidHalf reports whether s names a side of a split test, or none.
func ValidHalf(s string) bool { return s == "" || s == "a" || s == "b" }

// Stats is what one run did.
type Stats struct {
	Sent   int
	Failed int
	// Deferred counts the people left for a later run because another sender already
	// took their mail for the day. They stay candidates and are part of Remaining.
	Deferred  int
	Remaining int64
}

// Pending reports how many people the campaign would reach right now, without
// sending anything.
func (r *Runner) Pending(ctx context.Context, c Campaign) (int64, error) {
	return r.store.CountBroadcastCandidates(ctx, db.CountBroadcastCandidatesParams{Campaign: c.Name, Half: c.Half, Timezones: c.Timezones})
}

// Run sends the campaign to one capped batch and reports what is left.
//
// A failed send is recorded and stepped over, exactly as in the onboarding
// sequence: one address that SES rejects must not strand the rest of the list, and
// retrying it on the next run would spend the domain's reputation on an address
// that has already refused once.
//
// Each send takes the person's daily claim first (maildaily). One already held leaves them
// a candidate with no ledger row; a failed send gives the claim back, since the ledger row
// that burns the campaign for them is already what records the failure.
func (r *Runner) Run(ctx context.Context, c Campaign) (Stats, error) {
	rows, err := r.store.ListBroadcastCandidates(ctx, db.ListBroadcastCandidatesParams{
		Campaign:  c.Name,
		Half:      c.Half,
		Timezones: c.Timezones,
		MaxRows:   r.max,
	})
	if err != nil {
		return Stats{}, fmt.Errorf("broadcast: listing %s candidates: %w", c.Name, err)
	}

	var stats Stats
	for _, row := range rows {
		claim, ok, err := r.claims.Take(ctx, row.ID)
		if err != nil {
			stats.Failed++
			log.Printf("broadcast: %s to user %d: %v", c.Name, row.ID, err)
			continue
		}
		if !ok {
			stats.Deferred++
			continue
		}

		sendErr := r.mailer.Send(ctx, c, row.ID, row.Email)

		var errText string
		if sendErr != nil {
			errText = sendErr.Error()
			stats.Failed++
			log.Printf("broadcast: %s to user %d failed: %v", c.Name, row.ID, sendErr)
			if err := r.claims.Release(ctx, claim); err != nil {
				log.Printf("broadcast: %v", err)
			}
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

	stats.Remaining, err = r.Pending(ctx, c)
	if err != nil {
		// The batch went out; only the tally is missing. Report what is known.
		log.Printf("broadcast: counting the remainder of %s failed: %v", c.Name, err)
	}
	return stats, nil
}
