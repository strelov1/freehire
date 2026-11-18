// Package limitnudge sends a one-time, personal email to a free-tier account that has
// run into a daily plan ceiling, when it is most receptive to an upgrade.
//
// It mirrors internal/engage/prowelcome's shape (candidate query, one-time send
// ledger column, single personal letter with a human Reply-To).
// cmd/limit-nudge-mail is the only caller.
//
// The trigger is written by internal/ai/plan.Store.Consume, the one chokepoint every
// metered feature refuses through, into plan_limit_hits; this package only reads it.
package limitnudge

import (
	"context"
	"fmt"
	"log"

	"github.com/strelov1/freehire/internal/engage/maildaily"
	"github.com/strelov1/freehire/internal/platform/db"
)

// Store is the slice of the database this feature reads and writes, declared here so
// the runner can be tested without Postgres.
type Store interface {
	maildaily.Store
	ListLimitHitUsersMissingNudgeEmail(ctx context.Context, arg db.ListLimitHitUsersMissingNudgeEmailParams) ([]db.ListLimitHitUsersMissingNudgeEmailRow, error)
	SetLimitNudgeSent(ctx context.Context, id int64) (int64, error)
}

// mailer is the slice of *Mailer the runner needs, so tests can inject a fake.
type mailer interface {
	Send(ctx context.Context, userID int64, to string) error
}

// Runner performs one pass: nudge every free-tier account that hit a plan ceiling and
// has not yet been mailed about it.
type Runner struct {
	store      Store
	mailer     mailer
	claims     *maildaily.Claimer
	windowDays int32
	maxRows    int32
}

// New builds a Runner. windowDays bounds how far back a hit still counts; maxRows
// bounds one pass.
func New(store Store, m mailer, windowDays, maxRows int32) *Runner {
	return &Runner{
		store: store, mailer: m, claims: maildaily.New(store, maildaily.SourceLimitNudge),
		windowDays: windowDays, maxRows: maxRows,
	}
}

// Stats is what one pass did.
type Stats struct {
	Sent, Failed int
	// Deferred counts the nudges left pending because another sender already took the
	// account's mail for the day; they are candidates again on the next run.
	Deferred int
}

// Run nudges every candidate the store hands back.
//
// A failed candidate listing aborts the pass. A single failed *send* is counted and
// stepped over; leaving limit_nudge_sent_at unset lets a later run retry it.
//
// Every send takes the account's daily claim first (maildaily). One already held
// leaves the account pending (no send, no stamp); a failed send gives the claim back.
func (r *Runner) Run(ctx context.Context) (Stats, error) {
	var stats Stats

	rows, err := r.store.ListLimitHitUsersMissingNudgeEmail(ctx, db.ListLimitHitUsersMissingNudgeEmailParams{
		WindowDays: r.windowDays,
		MaxRows:    r.maxRows,
	})
	if err != nil {
		return stats, fmt.Errorf("limitnudge: listing candidates: %w", err)
	}

	for _, row := range rows {
		claim, ok, err := r.claims.Take(ctx, row.ID)
		if err != nil {
			stats.Failed++
			log.Printf("limitnudge: nudge mail to user %d: %v", row.ID, err)
			continue
		}
		if !ok {
			stats.Deferred++
			continue
		}
		if err := r.mailer.Send(ctx, row.ID, row.Email); err != nil {
			stats.Failed++
			log.Printf("limitnudge: nudge mail to user %d failed: %v", row.ID, err)
			if err := r.claims.Release(ctx, claim); err != nil {
				log.Printf("limitnudge: %v", err)
			}
			continue
		}
		if _, err := r.store.SetLimitNudgeSent(ctx, row.ID); err != nil {
			// The mail went out but the stamp failed: the next run resends, a harmless
			// duplicate, rather than losing the nudge.
			stats.Failed++
			log.Printf("limitnudge: user %d nudged but limit_nudge_sent_at not stamped (will resend): %v", row.ID, err)
			continue
		}
		stats.Sent++
	}
	return stats, nil
}
