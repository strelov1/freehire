package promo

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

// DefaultRewardCeiling bounds how many rewards one referrer may earn when the environment
// says nothing — twelve, which is about half a year of a monthly subscription for free.
//
// The bound exists for the patient version of the abuse this design otherwise resists:
// attribution is a cookie the visitor controls, so a determined person with real cards can
// keep referring themselves. Each round costs them a real payment, which is not an
// arbitrage, but a ceiling means it is not even a long game.
const DefaultRewardCeiling = 12

// PendingReward is an attributed signup whose invitee holds a provider customer, so there
// is something to ask the provider about.
type PendingReward struct {
	ID              int64
	ReferrerID      int64
	RefereeCustomer string
}

// EarnedReward is a granted reward that has not yet reached the referrer's balance.
type EarnedReward struct {
	ID          int64
	ReferrerID  int64
	AmountCents int64
}

// Payments answers the one question that decides whether a referral earned anything: did
// this invitee pay us at least what the reward is worth?
//
// About money COLLECTED and not about a subscription being active, because a subscription
// can be active having collected nothing — a trial, or a total discount. And about ENOUGH
// money rather than any: an invitee who redeemed a 90% code pays a tenth of the list price,
// so a reward worth half of it would cost four times what the sale brought in, repeatably,
// for as long as that code has seats.
type Payments interface {
	HasCollectedAtLeast(ctx context.Context, customerID string, minCents int64) (bool, error)
}

// Credits places a reward on a referrer's balance. The implementation creates a provider
// customer for a referrer who has never bought anything; from here that is invisible, which
// is the point — this package does not know what a provider is.
type Credits interface {
	CreditAccount(ctx context.Context, userID, cents int64, idempotencyKey string) error
}

// GrantEarned moves rewards from attributed to earned, and reports how many moved.
//
// Called from a worker rather than from the webhook. The webhook is answered inside a
// window the provider enforces and applying an event must not grow work that can fail;
// this pass is guarded on the row's own status, so a crash half-way costs a repeat that
// changes nothing.
//
// priceCents is the list price at this moment. The amount is fixed into the row here and
// never recomputed, so a later price change cannot revalue credit already earned.
func (s *Service) GrantEarned(ctx context.Context, max int32, priceCents int64, payments Payments) (int, error) {
	rewards, err := s.repo.PendingRewards(ctx, max)
	if err != nil {
		return 0, fmt.Errorf("promo: reading pending rewards: %w", err)
	}

	amount := priceCents * int64(InvitePercent) / 100
	granted := 0
	ceiling := int64(s.ceiling())
	for _, reward := range rewards {
		// Read first as a CHEAP SKIP, not as the bound. A referrer already at the ceiling
		// costs no provider call this way, which is the whole reason it is here — the bound
		// itself is inside the UPDATE, because two passes reading the same count would each
		// grant one more than it allows.
		count, err := s.repo.CountGranted(ctx, reward.ReferrerID)
		if err != nil {
			return granted, fmt.Errorf("promo: counting rewards of user %d: %w", reward.ReferrerID, err)
		}
		if count >= ceiling {
			continue
		}

		// The threshold is the reward itself, which is the rule stated as arithmetic: a
		// referral never pays out more than it brought in.
		collected, err := payments.HasCollectedAtLeast(ctx, reward.RefereeCustomer, amount)
		if err != nil {
			// One unreachable customer must not end the pass: the row stays pending and the
			// next run tries again, which is the same shape as every other retry here.
			log.Printf("promo: reward %d: reading the invitee's invoices: %v", reward.ID, err)
			continue
		}
		if !collected {
			continue
		}

		moved, err := s.repo.Grant(ctx, reward.ID, amount, ceiling)
		if err != nil {
			return granted, fmt.Errorf("promo: granting reward %d: %w", reward.ID, err)
		}
		if moved {
			granted++
		}
	}
	return granted, nil
}

// DeliverEarned places earned rewards on their referrers' balances and reports how many
// were delivered.
//
// The stamp follows the credit and never precedes it. Stamping first would mean a failure
// between the two records money that never moved, and nothing would ever look at that row
// again. The other way round, a failure after the credit costs one repeat — which the
// idempotency key on the provider's side absorbs.
func (s *Service) DeliverEarned(ctx context.Context, max int32, credits Credits) (int, error) {
	rewards, err := s.repo.UndeliveredRewards(ctx, max)
	if err != nil {
		return 0, fmt.Errorf("promo: reading undelivered rewards: %w", err)
	}

	delivered := 0
	for _, reward := range rewards {
		key := fmt.Sprintf("invite_reward_%d", reward.ID)
		if err := credits.CreditAccount(ctx, reward.ReferrerID, reward.AmountCents, key); err != nil {
			return delivered, fmt.Errorf("promo: crediting user %d for reward %d: %w",
				reward.ReferrerID, reward.ID, err)
		}
		if _, err := s.repo.MarkDelivered(ctx, reward.ID); err != nil {
			return delivered, fmt.Errorf("promo: stamping reward %d delivered: %w", reward.ID, err)
		}
		delivered++
	}
	return delivered, nil
}

// ceiling is the configured bound, falling back when nothing sensible was configured.
func (s *Service) ceiling() int {
	if s.cfg.RewardCeiling > 0 {
		return s.cfg.RewardCeiling
	}
	return DefaultRewardCeiling
}

// ConfigFromEnv reads what this package cannot derive. It cannot fail.
//
// A ceiling that will not parse falls back and logs rather than stopping the process. The
// pass that carries it also reconciles subscriptions, and a typo in an optional bound must
// not be able to stop money being reconciled.
func ConfigFromEnv() Config {
	cfg := Config{
		// The same variable billing reads. A SITE_URL of its own would be a second name for
		// one fact, and the two would drift.
		SiteURL:       strings.TrimRight(strings.TrimSpace(os.Getenv("FRONTEND_ORIGIN")), "/"),
		RewardCeiling: DefaultRewardCeiling,
	}

	raw := strings.TrimSpace(os.Getenv("INVITE_REWARD_MAX_PER_USER"))
	if raw == "" {
		return cfg
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		log.Printf("promo: INVITE_REWARD_MAX_PER_USER=%q is not a positive integer; using %d",
			raw, DefaultRewardCeiling)
		return cfg
	}
	cfg.RewardCeiling = n
	return cfg
}
