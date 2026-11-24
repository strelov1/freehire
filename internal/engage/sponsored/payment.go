package sponsored

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/identity/billing"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// ErrRefundPending is wrapped by Reject when the rejection committed but the refund did not finish.
var ErrRefundPending = errors.New("sponsored: campaign rejected, refund pending")

// Refunder returns a payment to the payer. *billing.Service is the production one; the key
// makes a repeated call a no-op at the provider.
type Refunder interface {
	RefundPaymentIntent(ctx context.Context, paymentIntentID, idempotencyKey string) error
}

// WithRefunder sets how money goes back; without one a refund stays pending for RefundPending.
func (s *Service) WithRefunder(r Refunder) *Service {
	s.refunder = r
	return s
}

var _ billing.TopupCrediter = (*Service)(nil)

// CreditTopup applies one verified payment (billing re-read it from the provider). It is
// idempotent by session id: a replay finds the row and changes nothing.
//
// A payment for a campaign that is already rejected or ended is recorded but NOT added to the
// budget (it can never serve) and is refunded. The refund is best effort here: the credit has
// succeeded, so the event may be stamped processed, and RefundPending (cmd/billing-sync) retries.
func (s *Service) CreditTopup(ctx context.Context, t billing.Topup) error {
	var dead bool
	err := s.inTx(ctx, func(q *db.Queries) error {
		c, err := q.GetSponsoredCampaignForUpdate(ctx, t.CampaignID)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && c.OwnerUserID != pgconv.Int8(&t.UserID)) {
			return fmt.Errorf("%w: campaign %d, user %d", billing.ErrTopupInvalid, t.CampaignID, t.UserID)
		}
		if err != nil {
			return err
		}
		from := Status(c.Status)
		dead = from == StatusRejected || from == StatusEnded

		if _, err := q.InsertSponsoredTopup(ctx, db.InsertSponsoredTopupParams{
			CampaignID: t.CampaignID, StripeSessionID: t.SessionID,
			PaymentIntentID: pgconv.Text(t.PaymentIntentID),
			AmountCents:     t.AmountCents, Credited: !dead,
		}); errors.Is(err, pgx.ErrNoRows) {
			dead = false // already recorded: its own pass (or RefundPending) owns any refund
			return nil
		} else if err != nil {
			return err
		}
		if dead {
			return nil
		}

		to := from
		switch {
		case from == StatusDraft || from == StatusPendingPayment:
			to = StatusPendingReview
		case from == StatusExhausted && c.ReviewedAt.Valid:
			to = StatusActive
		}
		if err := q.CreditSponsoredBudget(ctx, db.CreditSponsoredBudgetParams{ID: t.CampaignID, AmountCents: t.AmountCents, Status: string(to)}); err != nil {
			return err
		}
		if to != from {
			return q.RequeueSearchOutbox(ctx, c.JobID)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if dead {
		if _, err := s.refund(ctx, &t.CampaignID, 10); err != nil {
			log.Printf("sponsored: refund of a payment for dead campaign %d left for the reconciler: %v", t.CampaignID, err)
		}
	}
	return nil
}

// Approve moves a campaign in review to active.
func (s *Service) Approve(ctx context.Context, campaignID, moderatorID int64) error {
	return s.review(ctx, campaignID, moderatorID, StatusActive, "")
}

// Reject moves a campaign in review to rejected and refunds everything paid into it. The status
// change commits first and on its own: a refund that fails does not undo the rejection, it is
// returned to the caller and retried by RefundPending.
func (s *Service) Reject(ctx context.Context, campaignID, moderatorID int64, reason string) error {
	if err := s.review(ctx, campaignID, moderatorID, StatusRejected, reason); err != nil {
		return err
	}
	if _, err := s.refund(ctx, &campaignID, 100); err != nil {
		return fmt.Errorf("%w: campaign %d: %w", ErrRefundPending, campaignID, err)
	}
	return nil
}

func (s *Service) review(ctx context.Context, campaignID, moderatorID int64, to Status, reason string) error {
	return s.inTx(ctx, func(q *db.Queries) error {
		c, err := q.GetSponsoredCampaignForUpdate(ctx, campaignID)
		if err != nil {
			return err
		}
		if Status(c.Status) != StatusPendingReview || !CanTransition(StatusPendingReview, to) {
			return fmt.Errorf("%w: %s -> %s", ErrBadTransition, c.Status, to)
		}
		if err := q.ReviewSponsoredCampaign(ctx, db.ReviewSponsoredCampaignParams{
			ID: campaignID, Status: string(to), ReviewedBy: pgtype.Int8{Int64: moderatorID, Valid: true},
			RejectReason: pgconv.Text(reason),
		}); err != nil {
			return err
		}
		return q.RequeueSearchOutbox(ctx, c.JobID)
	})
}

// RefundPending refunds every top-up that must go back and has not (rejected campaigns, payments
// that arrived for a dead one), up to max, and returns how many Stripe confirmed. One failure
// does not stop the rest. Safe to run from several places: the idempotency key is per top-up.
func (s *Service) RefundPending(ctx context.Context, max int32) (int, error) {
	return s.refund(ctx, nil, max)
}

func (s *Service) refund(ctx context.Context, campaignID *int64, max int32) (int, error) {
	if s.refunder == nil {
		return 0, errors.New("sponsored: no refunder is wired")
	}
	q := db.New(s.pool)
	arg := pgtype.Int8{}
	if campaignID != nil {
		arg = pgtype.Int8{Int64: *campaignID, Valid: true}
	}
	rows, err := q.ListSponsoredTopupsToRefund(ctx, db.ListSponsoredTopupsToRefundParams{CampaignID: arg, MaxRows: max})
	if err != nil {
		return 0, err
	}
	var done int
	var errs []error
	for _, r := range rows {
		if err := s.refunder.RefundPaymentIntent(ctx, r.PaymentIntentID.String, fmt.Sprintf("sponsored-refund-%d", r.ID)); err != nil {
			errs = append(errs, fmt.Errorf("top-up %d: %w", r.ID, err))
			continue
		}
		// Stamped only after the provider confirmed; a crash in between replays the same key.
		if err := q.MarkSponsoredTopupRefunded(ctx, r.ID); err != nil {
			errs = append(errs, fmt.Errorf("top-up %d refunded but not stamped: %w", r.ID, err))
			continue
		}
		done++
	}
	return done, errors.Join(errs...)
}
