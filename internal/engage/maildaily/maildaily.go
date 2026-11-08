// Package maildaily is the one-optional-letter-a-day rule every sender of optional mail
// shares: onboarding, the limit nudge, broadcasts and the CV lifecycle letters.
//
// A sender takes the account's claim for the UTC day BEFORE it calls SES, and sends only
// when the claim was written. Whoever reaches an account first wins the day; the others
// leave their item pending, write no ledger row and find it again on a later pass.
//
// A definite failure to send releases the claim, so a bounced attempt does not cost the
// account its day. A crash between the claim and the send keeps it, which fails toward
// fewer letters — the direction every mail rule here errs in.
package maildaily

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

// The senders that take the claim. The source is stored beside the claim so the
// one-letter-a-day report can say who held a day, and a sender can only release its own.
const (
	SourceOnboarding  = "onboarding"
	SourceLimitNudge  = "limitnudge"
	SourceBroadcast   = "broadcast"
	SourceCVLifecycle = "cv_lifecycle"
)

// Store is the slice of the database the claim needs. Every sender's own Store embeds it,
// so a runner's fake implements these two methods beside its own.
type Store interface {
	ClaimMailDay(ctx context.Context, arg db.ClaimMailDayParams) (pgtype.Date, error)
	ReleaseMailDay(ctx context.Context, arg db.ReleaseMailDayParams) error
}

// Claim is one account's slot for one UTC day.
type Claim struct {
	UserID int64
	Day    pgtype.Date
}

// Claimer takes and releases claims on behalf of one sender.
type Claimer struct {
	store  Store
	source string
}

// New builds a Claimer for the sender named by source.
func New(store Store, source string) *Claimer {
	return &Claimer{store: store, source: source}
}

// Take tries to take the account's slot for today. ok is false, with a nil error, when
// another sender already holds it: the caller must not send. A non-nil error is a database
// failure, and the caller must not send either — a claim that cannot be recorded cannot
// bound anything.
func (c *Claimer) Take(ctx context.Context, userID int64) (claim Claim, ok bool, err error) {
	day, err := c.store.ClaimMailDay(ctx, db.ClaimMailDayParams{UserID: userID, Source: c.source})
	if errors.Is(err, pgx.ErrNoRows) {
		return Claim{}, false, nil
	}
	if err != nil {
		return Claim{}, false, fmt.Errorf("maildaily: claiming the day for user %d: %w", userID, err)
	}
	return Claim{UserID: userID, Day: day}, true, nil
}

// Release gives the slot back after a definite failure to send. A failure to release is
// only logged by the caller's convention: the worst outcome is one letter fewer today.
func (c *Claimer) Release(ctx context.Context, claim Claim) error {
	if err := c.store.ReleaseMailDay(ctx, db.ReleaseMailDayParams{
		UserID: claim.UserID,
		Day:    claim.Day,
		Source: c.source,
	}); err != nil {
		return fmt.Errorf("maildaily: releasing the day for user %d: %w", claim.UserID, err)
	}
	return nil
}
