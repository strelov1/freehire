package promo

import (
	"context"
	"errors"
	"testing"
)

// fakeLedger stands in for the reward rows the worker walks.
type fakeLedger struct {
	pending     []PendingReward
	undelivered []EarnedReward
	granted     map[int64]int64 // reward id → amount
	delivered   map[int64]bool
	countByRef  map[int64]int64
}

func newFakeLedger() *fakeLedger {
	return &fakeLedger{
		granted:    map[int64]int64{},
		delivered:  map[int64]bool{},
		countByRef: map[int64]int64{},
	}
}

func (f *fakeLedger) PendingRewards(_ context.Context, _ int32) ([]PendingReward, error) {
	return f.pending, nil
}

func (f *fakeLedger) CountGranted(_ context.Context, referrerID int64) (int64, error) {
	return f.countByRef[referrerID], nil
}

func (f *fakeLedger) Grant(_ context.Context, id, amountCents int64) (bool, error) {
	if _, done := f.granted[id]; done {
		return false, nil
	}
	f.granted[id] = amountCents
	return true, nil
}

// UndeliveredRewards filters on the stamp, as the query it stands in for does. Without
// that the fake would hand the same row back on a second pass and the test would be
// asserting something production can never do.
func (f *fakeLedger) UndeliveredRewards(_ context.Context, _ int32) ([]EarnedReward, error) {
	out := make([]EarnedReward, 0, len(f.undelivered))
	for _, reward := range f.undelivered {
		if !f.delivered[reward.ID] {
			out = append(out, reward)
		}
	}
	return out, nil
}

func (f *fakeLedger) MarkDelivered(_ context.Context, id int64) (bool, error) {
	if f.delivered[id] {
		return false, nil
	}
	f.delivered[id] = true
	return true, nil
}

// fakePayments answers whether an invitee's money actually moved.
type fakePayments struct {
	collected map[string]bool
	asked     []string
}

func (f *fakePayments) HasCollectedPayment(_ context.Context, customerID string) (bool, error) {
	f.asked = append(f.asked, customerID)
	return f.collected[customerID], nil
}

// fakeCredits records what was placed on a referrer's balance.
type fakeCredits struct {
	placed map[int64]int64
	keys   []string
	fail   error
}

func (f *fakeCredits) CreditAccount(_ context.Context, userID, cents int64, key string) error {
	if f.fail != nil {
		return f.fail
	}
	f.placed[userID] += cents
	f.keys = append(f.keys, key)
	return nil
}

func newRewardService(ledger *fakeLedger, cfg Config) *Service {
	repo := newFakeRepo()
	repo.fakeLedger = ledger
	return New(repo, cfg)
}

func TestGrantPaysOnlyForAnInvoiceThatCollected(t *testing.T) {
	ledger := newFakeLedger()
	ledger.pending = []PendingReward{
		{ID: 1, ReferrerID: 10, RefereeCustomer: "cus_paid"},
		{ID: 2, ReferrerID: 11, RefereeCustomer: "cus_free"},
	}
	payments := &fakePayments{collected: map[string]bool{"cus_paid": true}}
	svc := newRewardService(ledger, Config{RewardCeiling: 12})

	if _, err := svc.GrantEarned(context.Background(), 100, 500, payments); err != nil {
		t.Fatalf("GrantEarned: %v", err)
	}
	if _, ok := ledger.granted[1]; !ok {
		t.Fatal("the reward for a paying invitee was not granted")
	}
	if _, ok := ledger.granted[2]; ok {
		t.Fatal("a reward was granted for an invitee whose invoices collected nothing — an " +
			"active subscription that collected zero is a trial or a total discount, and " +
			"paying for it turns the discount into a way to mint credit")
	}
}

func TestGrantFixesTheAmountAtHalfTheListPrice(t *testing.T) {
	ledger := newFakeLedger()
	ledger.pending = []PendingReward{{ID: 1, ReferrerID: 10, RefereeCustomer: "cus_paid"}}
	payments := &fakePayments{collected: map[string]bool{"cus_paid": true}}
	svc := newRewardService(ledger, Config{RewardCeiling: 12})

	if _, err := svc.GrantEarned(context.Background(), 100, 500, payments); err != nil {
		t.Fatalf("GrantEarned: %v", err)
	}
	if got := ledger.granted[1]; got != 250 {
		t.Fatalf("amount = %d, want 250 — half of a 500-cent price, fixed now so a later "+
			"price change cannot revalue credit somebody has already earned", got)
	}
}

func TestGrantStopsAtTheCeiling(t *testing.T) {
	ledger := newFakeLedger()
	ledger.pending = []PendingReward{{ID: 1, ReferrerID: 10, RefereeCustomer: "cus_paid"}}
	ledger.countByRef[10] = 12
	payments := &fakePayments{collected: map[string]bool{"cus_paid": true}}
	svc := newRewardService(ledger, Config{RewardCeiling: 12})

	if _, err := svc.GrantEarned(context.Background(), 100, 500, payments); err != nil {
		t.Fatalf("GrantEarned: %v", err)
	}
	if _, ok := ledger.granted[1]; ok {
		t.Fatal("a reward was granted past the per-referrer ceiling")
	}
}

func TestGrantIsANoOpOnASecondRun(t *testing.T) {
	ledger := newFakeLedger()
	ledger.pending = []PendingReward{{ID: 1, ReferrerID: 10, RefereeCustomer: "cus_paid"}}
	payments := &fakePayments{collected: map[string]bool{"cus_paid": true}}
	svc := newRewardService(ledger, Config{RewardCeiling: 12})

	first, err := svc.GrantEarned(context.Background(), 100, 500, payments)
	if err != nil {
		t.Fatalf("first GrantEarned: %v", err)
	}
	second, err := svc.GrantEarned(context.Background(), 100, 500, payments)
	if err != nil {
		t.Fatalf("second GrantEarned: %v", err)
	}
	if first != 1 || second != 0 {
		t.Fatalf("granted %d then %d, want 1 then 0", first, second)
	}
}

func TestDeliverCreditsOnceAndCarriesAStableKey(t *testing.T) {
	ledger := newFakeLedger()
	ledger.undelivered = []EarnedReward{{ID: 7, ReferrerID: 10, AmountCents: 250}}
	credits := &fakeCredits{placed: map[int64]int64{}}
	svc := newRewardService(ledger, Config{RewardCeiling: 12})

	if _, err := svc.DeliverEarned(context.Background(), 100, credits); err != nil {
		t.Fatalf("DeliverEarned: %v", err)
	}
	if _, err := svc.DeliverEarned(context.Background(), 100, credits); err != nil {
		t.Fatalf("second DeliverEarned: %v", err)
	}
	if credits.placed[10] != 250 {
		t.Fatalf("credited %d, want 250 exactly once", credits.placed[10])
	}
	if len(credits.keys) == 0 || credits.keys[0] == "" {
		t.Fatal("the credit carried no idempotency key — the provider is the other half of " +
			"the never-twice guarantee, and a retried request without one credits again")
	}
}

func TestDeliverLeavesTheRowAloneWhenTheCreditFails(t *testing.T) {
	ledger := newFakeLedger()
	ledger.undelivered = []EarnedReward{{ID: 7, ReferrerID: 10, AmountCents: 250}}
	credits := &fakeCredits{placed: map[int64]int64{}, fail: errors.New("provider down")}
	svc := newRewardService(ledger, Config{RewardCeiling: 12})

	if _, err := svc.DeliverEarned(context.Background(), 100, credits); err == nil {
		t.Fatal("DeliverEarned reported success while the credit failed")
	}
	if ledger.delivered[7] {
		t.Fatal("the reward was stamped delivered although nothing was credited — the stamp " +
			"is the only record that the money moved, so it must follow the credit")
	}
}

func TestConfigFromEnvFallsBackRatherThanFailing(t *testing.T) {
	t.Setenv("INVITE_REWARD_MAX_PER_USER", "not a number")
	t.Setenv("FRONTEND_ORIGIN", "https://example.test/")

	cfg := ConfigFromEnv()
	if cfg.RewardCeiling != DefaultRewardCeiling {
		t.Fatalf("ceiling = %d, want %d — a typo here must not stop the pass that also "+
			"reconciles subscriptions", cfg.RewardCeiling, DefaultRewardCeiling)
	}
	if cfg.SiteURL != "https://example.test" {
		t.Fatalf("SiteURL = %q, want the origin without its trailing slash", cfg.SiteURL)
	}
}

func TestConfigFromEnvReadsTheCeiling(t *testing.T) {
	t.Setenv("INVITE_REWARD_MAX_PER_USER", "3")
	t.Setenv("FRONTEND_ORIGIN", "")

	if got := ConfigFromEnv().RewardCeiling; got != 3 {
		t.Fatalf("ceiling = %d, want 3", got)
	}
}
