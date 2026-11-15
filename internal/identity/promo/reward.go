package promo

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

// DefaultRewardCeiling bounds rewards per referrer when the environment is silent.
//
// Attribution is a visitor-controlled cookie, so someone with real cards could keep
// referring themselves; the ceiling ends that.
const DefaultRewardCeiling = 12

// PendingReward is an attributed signup whose invitee holds a provider customer.
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

// Payments answers whether an invitee paid at least what the reward is worth.
//
// It is about money collected, not an active subscription (a trial or a total discount
// collects nothing), and about enough money: a 90% code pays a tenth of list price, so a
// half-price reward would cost far more than the sale brought in.
type Payments interface {
	HasCollectedAtLeast(ctx context.Context, customerID string, minCents int64) (bool, error)
}

// Credits places a reward on a referrer's balance. The implementation may create a
// provider customer for a referrer who never bought; this package does not see that.
type Credits interface {
	CreditAccount(ctx context.Context, userID, cents int64, idempotencyKey string) error
}

// GrantEarned moves rewards from attributed to earned and reports how many moved.
//
// It runs in a worker, not the webhook, which must answer inside the provider's window. It
// is guarded on row status, so a crash half-way only costs a no-op repeat.
//
// priceCents is the list price now; the amount is fixed into the row, so a later price
// change cannot revalue earned credit.
func (s *Service) GrantEarned(ctx context.Context, max int32, priceCents int64, payments Payments) (int, error) {
	rewards, err := s.repo.PendingRewards(ctx, max)
	if err != nil {
		return 0, fmt.Errorf("promo: reading pending rewards: %w", err)
	}

	amount := priceCents * int64(InvitePercent) / 100
	granted := 0
	ceiling := int64(s.ceiling())
	for _, reward := range rewards {
		// A cheap skip that saves a provider call; the real bound is inside the UPDATE.
		count, err := s.repo.CountGranted(ctx, reward.ReferrerID)
		if err != nil {
			return granted, fmt.Errorf("promo: counting rewards of user %d: %w", reward.ReferrerID, err)
		}
		if count >= ceiling {
			continue
		}

		// The threshold is the reward itself: a referral never pays out more than it brought in.
		collected, err := payments.HasCollectedAtLeast(ctx, reward.RefereeCustomer, amount)
		if err != nil {
			// One unreachable customer must not end the pass; the row stays pending.
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

// DeliverEarned places earned rewards on referrers' balances and reports how many were
// delivered.
//
// The stamp follows the credit, never precedes it: stamping first would record money that
// never moved, while a failure after the credit costs a repeat the provider's idempotency
// key absorbs.
//
// One row's failure never ends the pass: the query has no attempt counter, so a single
// undeliverable reward would sit at the head of every run and block the rest. Failures are
// joined and returned after the loop so the run still exits non-zero.
func (s *Service) DeliverEarned(ctx context.Context, max int32, credits Credits) (int, error) {
	rewards, err := s.repo.UndeliveredRewards(ctx, max)
	if err != nil {
		return 0, fmt.Errorf("promo: reading undelivered rewards: %w", err)
	}

	delivered := 0
	var failures []error
	for _, reward := range rewards {
		key := fmt.Sprintf("invite_reward_%d", reward.ID)
		if err := credits.CreditAccount(ctx, reward.ReferrerID, reward.AmountCents, key); err != nil {
			failures = append(failures, fmt.Errorf("promo: crediting user %d for reward %d: %w",
				reward.ReferrerID, reward.ID, err))
			continue
		}
		if _, err := s.repo.MarkDelivered(ctx, reward.ID); err != nil {
			// The credit went through; the repeat next run is absorbed by the idempotency key.
			failures = append(failures, fmt.Errorf("promo: stamping reward %d delivered: %w",
				reward.ID, err))
			continue
		}
		delivered++
	}
	return delivered, errors.Join(failures...)
}

// ceiling is the configured bound, or the default when none is set.
func (s *Service) ceiling() int {
	if s.cfg.RewardCeiling > 0 {
		return s.cfg.RewardCeiling
	}
	return DefaultRewardCeiling
}

// ConfigFromEnv reads what this package cannot derive. It cannot fail: an unparsable
// ceiling falls back and logs, because the pass that carries it also reconciles
// subscriptions and a typo must not stop that.
func ConfigFromEnv() Config {
	cfg := Config{
		// The same variable billing reads, so the two cannot drift.
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
