package main

import (
	"context"
	"errors"
	"testing"
)

// stubProvider stands in for Stripe in the referral pass.
type stubProvider struct {
	priceCents int64
	priceErr   error
	collected  map[string]int64
	credited   map[int64]int64
}

func (s *stubProvider) CheckoutPriceCents(context.Context) (int64, error) {
	return s.priceCents, s.priceErr
}

func (s *stubProvider) HasCollectedAtLeast(_ context.Context, customerID string, minCents int64) (bool, error) {
	return s.collected[customerID] >= minCents, nil
}

func (s *stubProvider) CreditAccount(_ context.Context, userID, cents int64, _ string) error {
	s.credited[userID] += cents
	return nil
}

func TestSettleRewardsStopsWhenThePriceCannotBeRead(t *testing.T) {
	provider := &stubProvider{
		priceErr:  errors.New("provider unreachable"),
		collected: map[string]int64{},
		credited:  map[int64]int64{},
	}

	// A nil *db.Queries is safe precisely because the pass must not reach the database
	// after failing to read the price. If it ever does, this panics and says so loudly —
	// which is the point: granting at a guessed price would misprice credit permanently,
	// since the amount is fixed into the row and never recomputed.
	if failures := settleRewards(context.Background(), provider, nil, 100); failures != 1 {
		t.Fatalf("failures = %d, want 1", failures)
	}
}
