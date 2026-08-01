package inbox

import (
	"context"
	"fmt"

	"github.com/strelov1/freehire/internal/appevent"
	"github.com/strelov1/freehire/internal/db"
)

// EventRecorder is the pair of statements the ledger reconcile needs, and nothing else. Both
// *db.Queries and a WithTx copy of it satisfy this, which is what lets the interactive paths and
// the classification worker — one best-effort, one inside a transaction — share the rule.
type EventRecorder interface {
	RetractSupersededEmailEvent(ctx context.Context, arg db.RetractSupersededEmailEventParams) (int64, error)
	RecordEmailApplicationEvent(ctx context.Context, arg db.RecordEmailApplicationEventParams) error
}

// ReconcileMailEvent brings the employer-reply ledger in line with one message's current link.
//
// ORDER IS THE RULE: the retraction must land before the insert. Both statements are
// data-modifying CTEs, so they read the same pre-statement snapshot — run the other way round,
// the insert's conflict check still sees the superseded row as live and the message ends up with
// two live events, or none.
//
// It lives here rather than at each call site because it had two implementations: this one and a
// copy inside cmd/classify-mail, the highest-volume writer of employer_reply events and the one
// furthest from the rule's documentation — where no domain package's tests could reach it. Three
// separate comments asserted the rule had "one home" while it had two.
//
// The error is returned rather than handled: the callers genuinely differ. The interactive paths
// are best-effort — the mutation the user asked for has already succeeded, and the reconcile is
// idempotent, so the next mutation or the backfill carries it out — while the worker propagates
// it to roll back the transaction that persisted the link.
func ReconcileMailEvent(ctx context.Context, q EventRecorder, userID, emailID int64, mailSource string) error {
	source, err := appevent.SourceForMail(mailSource)
	if err != nil {
		return fmt.Errorf("ledger source: %w", err)
	}
	if _, err := q.RetractSupersededEmailEvent(ctx, db.RetractSupersededEmailEventParams{
		ID: emailID, UserID: userID,
	}); err != nil {
		return fmt.Errorf("retract superseded event: %w", err)
	}
	if err := q.RecordEmailApplicationEvent(ctx, db.RecordEmailApplicationEventParams{
		ID: emailID, UserID: userID, EventSource: source,
	}); err != nil {
		return fmt.Errorf("record reply event: %w", err)
	}
	return nil
}
