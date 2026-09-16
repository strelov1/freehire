package prowelcome

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/platform/db"
)

// Store is the slice of the database this feature reads and writes. Declared here
// rather than taking *db.Queries so the runner can be tested without Postgres — the
// same reasoning onboarding.Store gives.
type Store interface {
	ListNewlyPayingUsersMissingWelcomeEmail(ctx context.Context, maxRows int32) ([]db.ListNewlyPayingUsersMissingWelcomeEmailRow, error)
	SetProWelcomeSent(ctx context.Context, id int64) (int64, error)
}

// mailer is the narrow slice of *Mailer the runner needs, so a test can inject a fake
// without constructing a real Sender/Layout/Links.
type mailer interface {
	Send(ctx context.Context, userID int64, to string, tier plan.Tier, until time.Time) error
}

// Runner performs one pass: welcome every account newly entitled to a paying tier.
type Runner struct {
	store   Store
	mailer  mailer
	maxRows int32
}

// New builds a Runner. maxRows bounds one pass — see cmd/pro-welcome-mail's own doc for
// the env var that sets it.
func New(store Store, m mailer, maxRows int32) *Runner {
	return &Runner{store: store, mailer: m, maxRows: maxRows}
}

// Stats is what one pass did.
type Stats struct {
	Sent, Failed int
}

// Run welcomes every candidate the store hands back.
//
// A failed candidate listing aborts the pass — that is a broken query or a broken
// database, and continuing would only produce more of the same error. A single failed
// *send*, by contrast, is counted and stepped over: leaving pro_welcome_sent_at unset is
// what lets a later run retry it, per the pro-welcome-email spec's "sending fails" scenario
// — the opposite of onboarding's own deliver, which burns the slot on a failed send
// because that sequence is a courtesy, not a payment confirmation.
func (r *Runner) Run(ctx context.Context) (Stats, error) {
	var stats Stats

	rows, err := r.store.ListNewlyPayingUsersMissingWelcomeEmail(ctx, r.maxRows)
	if err != nil {
		return stats, fmt.Errorf("prowelcome: listing candidates: %w", err)
	}

	for _, row := range rows {
		now := time.Now().UTC()
		tier := plan.TierOf(row.ProUntil.Time, row.UltraUntil.Time, now)
		until := row.ProUntil.Time
		if tier == plan.TierUltra {
			until = row.UltraUntil.Time
		}

		if err := r.mailer.Send(ctx, row.ID, row.Email, tier, until); err != nil {
			stats.Failed++
			log.Printf("prowelcome: welcome mail to user %d failed: %v", row.ID, err)
			continue
		}
		if _, err := r.store.SetProWelcomeSent(ctx, row.ID); err != nil {
			// The mail went out but the claim failed to record. Log loudly: the next
			// run resends rather than silently losing this account's welcome — a
			// harmless duplicate is the one visible symptom this can produce.
			stats.Failed++
			log.Printf("prowelcome: user %d welcomed but pro_welcome_sent_at not stamped (will resend): %v", row.ID, err)
			continue
		}
		stats.Sent++
	}
	return stats, nil
}
