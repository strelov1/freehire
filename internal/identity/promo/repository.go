package promo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgerr"
)

// QueriesRepository is the production Repository over the sqlc queries. It only maps "no
// rows" and unique violations to the sentinels the service branches on.
type QueriesRepository struct {
	q *db.Queries
}

// NewQueriesRepository constructs a QueriesRepository.
func NewQueriesRepository(q *db.Queries) *QueriesRepository {
	return &QueriesRepository{q: q}
}

// Compile-time assertion that QueriesRepository satisfies Repository.
var _ Repository = (*QueriesRepository)(nil)

// PreviewCode reads a usable code's percentage. No rows covers every refusal reason; see
// ErrNotUsable.
func (r *QueriesRepository) PreviewCode(ctx context.Context, code string) (int16, error) {
	pct, err := r.q.PreviewPromoCode(ctx, code)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotUsable
	}
	if err != nil {
		return 0, fmt.Errorf("promo: reading code: %w", err)
	}
	return pct, nil
}

// Redeem claims a seat and records the redemption in one statement.
//
// A unique violation means this account redeemed another code concurrently: an ineligible
// caller, not a failure. The statement rolls back whole, so no seat is lost.
func (r *QueriesRepository) Redeem(ctx context.Context, userID int64, code string) (int16, error) {
	pct, err := r.q.RedeemPromoCode(ctx, db.RedeemPromoCodeParams{Code: code, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) || pgerr.IsUniqueViolation(err) {
		return 0, ErrNotUsable
	}
	if err != nil {
		return 0, fmt.Errorf("promo: redeeming code for user %d: %w", userID, err)
	}
	return pct, nil
}

// HasRedeemed reports whether this account has spent its one redemption.
func (r *QueriesRepository) HasRedeemed(ctx context.Context, userID int64) (bool, error) {
	return r.q.HasRedeemedPromoCode(ctx, userID)
}

// RedeemedPercent is what the account's redeemed code is worth; no redemption is zero, not
// an error.
func (r *QueriesRepository) RedeemedPercent(ctx context.Context, userID int64) (int16, error) {
	pct, err := r.q.RedeemedPromoPercent(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("promo: reading the redeemed code of user %d: %w", userID, err)
	}
	return pct, nil
}

// EnsureInviteCode stores the offered code if the account has none and returns the code it
// holds.
//
// The upsert conflicts on the account; a collision on the code arrives as a unique
// violation, which the service answers by drawing again.
func (r *QueriesRepository) EnsureInviteCode(ctx context.Context, userID int64, code string) (string, error) {
	held, err := r.q.EnsureInviteCode(ctx, db.EnsureInviteCodeParams{UserID: userID, Code: code})
	if pgerr.IsUniqueViolation(err) {
		return "", ErrCodeTaken
	}
	if err != nil {
		return "", fmt.Errorf("promo: storing an invite code for user %d: %w", userID, err)
	}
	return held, nil
}

// ReferrerByCode resolves an invite code to its owner; no rows means a code nobody minted.
func (r *QueriesRepository) ReferrerByCode(ctx context.Context, code string) (int64, error) {
	owner, err := r.q.ReferrerByInviteCode(ctx, code)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotUsable
	}
	if err != nil {
		return 0, fmt.Errorf("promo: resolving an invite code: %w", err)
	}
	return owner, nil
}

// Attribute records the referral, reporting whether this call wrote it.
//
// ON CONFLICT DO NOTHING on the invitee: an account is worth one reward for life, so a
// repeat affects no rows instead of failing.
func (r *QueriesRepository) Attribute(ctx context.Context, referrerID, refereeID int64) (bool, error) {
	rows, err := r.q.AttributeInvite(ctx, db.AttributeInviteParams{
		ReferrerID: referrerID,
		RefereeID:  refereeID,
	})
	if err != nil {
		return false, fmt.Errorf("promo: attributing user %d to %d: %w", refereeID, referrerID, err)
	}
	return rows > 0, nil
}

// Stats aggregates one account's invitations.
func (r *QueriesRepository) Stats(ctx context.Context, userID int64) (Stats, error) {
	row, err := r.q.InviteStats(ctx, userID)
	if err != nil {
		return Stats{}, fmt.Errorf("promo: reading invite stats for user %d: %w", userID, err)
	}
	return Stats{
		Invitees:    row.Invitees,
		Rewarded:    row.Rewarded,
		CreditCents: row.CreditCents,
	}, nil
}

// HasPendingInvite reports whether this account still owes itself the invitee's
// first-month discount.
func (r *QueriesRepository) HasPendingInvite(ctx context.Context, userID int64) (bool, error) {
	return r.q.PendingInviteDiscount(ctx, userID)
}

// PendingRewards are attributed signups whose invitee holds a provider customer.
func (r *QueriesRepository) PendingRewards(ctx context.Context, max int32) ([]PendingReward, error) {
	rows, err := r.q.PendingInviteRewards(ctx, max)
	if err != nil {
		return nil, fmt.Errorf("promo: reading pending rewards: %w", err)
	}
	out := make([]PendingReward, 0, len(rows))
	for _, row := range rows {
		// The query already filters on the binding; skip rather than ask the provider about an
		// empty customer id.
		if !row.StripeCustomerID.Valid {
			continue
		}
		out = append(out, PendingReward{
			ID:              row.ID,
			ReferrerID:      row.ReferrerID,
			RefereeCustomer: row.StripeCustomerID.String,
		})
	}
	return out, nil
}

// CountGranted is how many rewards a referrer has already earned.
func (r *QueriesRepository) CountGranted(ctx context.Context, referrerID int64) (int64, error) {
	return r.q.CountGrantedInviteRewards(ctx, referrerID)
}

// Grant moves one reward to earned, reporting whether this call moved it. No rows means
// already granted or referrer at the ceiling; the caller treats both the same.
func (r *QueriesRepository) Grant(ctx context.Context, id, amountCents, ceiling int64) (bool, error) {
	rows, err := r.q.GrantInviteReward(ctx, db.GrantInviteRewardParams{
		ID:          id,
		AmountCents: amountCents,
		Ceiling:     ceiling,
	})
	if err != nil {
		return false, fmt.Errorf("promo: granting reward %d: %w", id, err)
	}
	return rows > 0, nil
}

// UndeliveredRewards are earned rewards not yet placed on a referrer's balance.
func (r *QueriesRepository) UndeliveredRewards(ctx context.Context, max int32) ([]EarnedReward, error) {
	rows, err := r.q.UndeliveredInviteRewards(ctx, max)
	if err != nil {
		return nil, fmt.Errorf("promo: reading undelivered rewards: %w", err)
	}
	out := make([]EarnedReward, 0, len(rows))
	for _, row := range rows {
		out = append(out, EarnedReward{
			ID:          row.ID,
			ReferrerID:  row.ReferrerID,
			AmountCents: row.AmountCents,
		})
	}
	return out, nil
}

// MarkDelivered stamps a reward as placed, reporting whether this call stamped it.
func (r *QueriesRepository) MarkDelivered(ctx context.Context, id int64) (bool, error) {
	rows, err := r.q.MarkInviteRewardDelivered(ctx, id)
	if err != nil {
		return false, fmt.Errorf("promo: stamping reward %d delivered: %w", id, err)
	}
	return rows > 0, nil
}
