package inbox

import (
	"context"
	"fmt"

	"github.com/strelov1/freehire/internal/application/appevent"
	"github.com/strelov1/freehire/internal/platform/db"
)

// EventRecorder is the pair of statements the ledger reconcile needs. Both *db.Queries and a
// WithTx copy satisfy it, so the best-effort interactive paths and the transactional
// classification worker share the rule.
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
// The error is returned because callers differ: the interactive paths are best-effort (the
// user's mutation already succeeded and the reconcile is idempotent), while the worker
// propagates it to roll back the transaction that persisted the link.
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
