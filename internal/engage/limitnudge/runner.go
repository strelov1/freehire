// Package limitnudge sends a one-time, personal email to a free-tier account that has
// run into a daily plan ceiling — the moment the course material this feature was
// built from calls the highest-leverage touchpoint in a self-serve funnel: somebody
// who just felt the wall is more receptive to an upgrade than somebody who never has.
//
// It mirrors internal/engage/prowelcome's shape exactly (candidate query, one-time send
// ledger column, single personal letter) and sits beside it for the same reason:
// first-person prose with a human Reply-To, not a machine reporting a fact.
// cmd/limit-nudge-mail is the only caller.
//
// The trigger is written by internal/ai/plan.Store.Consume — the one chokepoint every
// metered feature refuses through — into plan_limit_hits (migration 0178); this
// package only reads that table.
package limitnudge

import (
	"context"
	"fmt"
	"log"

	"github.com/strelov1/freehire/internal/platform/db"
)

// Store is the slice of the database this feature reads and writes. Declared here
// rather than taking *db.Queries so the runner can be tested without Postgres — the
// same reasoning prowelcome.Store gives.
type Store interface {
	ListLimitHitUsersMissingNudgeEmail(ctx context.Context, arg db.ListLimitHitUsersMissingNudgeEmailParams) ([]db.ListLimitHitUsersMissingNudgeEmailRow, error)
	SetLimitNudgeSent(ctx context.Context, id int64) (int64, error)
}

// mailer is the narrow slice of *Mailer the runner needs, so a test can inject a fake
// without constructing a real Sender/Layout/Links.
type mailer interface {
	Send(ctx context.Context, userID int64, to string) error
}

// Runner performs one pass: nudge every free-tier account that hit a plan ceiling and
// has not yet been mailed about it.
type Runner struct {
	store      Store
	mailer     mailer
	windowDays int32
	maxRows    int32
}

// New builds a Runner. windowDays bounds how far back a hit still counts; maxRows
// bounds one pass — see cmd/limit-nudge-mail's own doc for the env vars that set both.
func New(store Store, m mailer, windowDays, maxRows int32) *Runner {
	return &Runner{store: store, mailer: m, windowDays: windowDays, maxRows: maxRows}
}

// Stats is what one pass did.
type Stats struct {
	Sent, Failed int
}

// Run nudges every candidate the store hands back.
//
// A failed candidate listing aborts the pass — that is a broken query or a broken
// database, and continuing would only produce more of the same error. A single failed
// *send*, by contrast, is counted and stepped over: leaving limit_nudge_sent_at unset is
// what lets a later run retry it, mirroring prowelcome's own Run.
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
		if err := r.mailer.Send(ctx, row.ID, row.Email); err != nil {
			stats.Failed++
			log.Printf("limitnudge: nudge mail to user %d failed: %v", row.ID, err)
			continue
		}
		if _, err := r.store.SetLimitNudgeSent(ctx, row.ID); err != nil {
			// The mail went out but the claim failed to record. Log loudly: the next
			// run resends rather than silently losing this account's nudge — a
			// harmless duplicate is the one visible symptom this can produce.
			stats.Failed++
			log.Printf("limitnudge: user %d nudged but limit_nudge_sent_at not stamped (will resend): %v", row.ID, err)
			continue
		}
		stats.Sent++
	}
	return stats, nil
}
