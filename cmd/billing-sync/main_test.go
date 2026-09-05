package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/identity/billing"
	"github.com/strelov1/freehire/internal/platform/db"
)

// clearProviders unsets every provider's credentials, so a test says what it configures and
// a developer's own environment cannot decide the outcome.
func clearProviders(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"STRIPE_SECRET_KEY", "STRIPE_WEBHOOK_SECRET",
		"REVENUECAT_API_KEY", "REVENUECAT_WEBHOOK_SECRET",
	} {
		t.Setenv(k, "")
	}
	t.Setenv("DATABASE_URL", "")
}

// TestRunIsANoOpWithoutCredentials asserts the gate sits BEFORE Bootstrap.
//
// DATABASE_URL is deliberately cleared: if run() reached Bootstrap it would fail to open a
// pool and return 1, so a zero here is proof that it never tried. That is the property the
// spec asks for — an unconfigured deployment must run without touching the database — and
// it is also what makes this binary harmless in a checkout that is not freehire.me.
func TestRunIsANoOpWithoutCredentials(t *testing.T) {
	clearProviders(t)

	if code := run(); code != 0 {
		t.Fatalf("want a clean no-op exit, got %d", code)
	}
}

// TestRunIsANoOpWithHalfCredentials covers the misconfiguration that is worse than none: a
// key without a signing secret would accept unverifiable webhooks, and a secret without a
// key would record events it can never apply.
func TestRunIsANoOpWithHalfCredentials(t *testing.T) {
	for _, tc := range []struct{ name, apiKey, secret string }{
		{name: "api key only", apiKey: "sk_test"},
		{name: "webhook secret only", secret: "whsec_test"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearProviders(t)
			t.Setenv("STRIPE_SECRET_KEY", tc.apiKey)
			t.Setenv("STRIPE_WEBHOOK_SECRET", tc.secret)

			if code := run(); code != 0 {
				t.Fatalf("want a clean no-op exit, got %d", code)
			}
		})
	}
}

func TestMaxPerRun(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want int32
	}{
		{name: "unset keeps the default", raw: "", want: maxPerRunDefault},
		{name: "a positive number is honoured", raw: "50", want: 50},
		// A typo resolving to zero would make every run a silent no-op, which looks exactly
		// like a backlog that has already been drained.
		{name: "zero keeps the default", raw: "0", want: maxPerRunDefault},
		{name: "negative keeps the default", raw: "-5", want: maxPerRunDefault},
		{name: "nonsense keeps the default", raw: "lots", want: maxPerRunDefault},
		// Parsed at the width it is used at. Read as a platform int and converted, this
		// would WRAP to a negative limit — a query error, not a big batch — and the
		// "not a positive number" log line would be a lie.
		{name: "beyond a batch size keeps the default", raw: "3000000000", want: maxPerRunDefault},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("BILLING_SYNC_MAX_PER_RUN", tc.raw)
			if got := maxPerRun(); got != tc.want {
				t.Fatalf("want %d, got %d", tc.want, got)
			}
		})
	}
}

// TestTheStoreProviderAloneKeepsTheWorkerRunning is the other side of the gate. A deployment
// that sells only in the apps is a legitimate one, and a gate written as "no Stripe, nothing
// to do" would silently stop reconciling its store subscriptions.
//
// DATABASE_URL is empty, so reaching Bootstrap fails and returns 1. That failure is the
// assertion: it is proof the run did NOT stop at the gate.
func TestTheStoreProviderAloneKeepsTheWorkerRunning(t *testing.T) {
	clearProviders(t)
	t.Setenv("REVENUECAT_API_KEY", "sk_rc")
	t.Setenv("REVENUECAT_WEBHOOK_SECRET", "whsec_rc")

	if code := run(); code == 0 {
		t.Fatal("the run stopped at the gate with a configured store provider; its subscriptions would never be reconciled")
	}
}

// brokenProvider is a provider whose every call fails, standing in for one company's outage.
type brokenProvider struct{}

func (brokenProvider) Enabled() bool { return true }
func (brokenProvider) PendingEvents(context.Context, int32) ([]db.ListUnprocessedBillingEventsRow, error) {
	return nil, errors.New("provider unreachable")
}
func (brokenProvider) Apply(context.Context, int64, billing.Event) error { return nil }
func (brokenProvider) MarkProcessed(context.Context, int64) error        { return nil }
func (brokenProvider) SubscribersNearExpiry(context.Context, time.Duration, int32) ([]int64, error) {
	return nil, errors.New("provider unreachable")
}
func (brokenProvider) SyncUser(context.Context, int64) error { return nil }

// workingProvider has one account to refresh and refreshes it.
type workingProvider struct{ refreshed int }

func (*workingProvider) Enabled() bool { return true }
func (*workingProvider) PendingEvents(context.Context, int32) ([]db.ListUnprocessedBillingEventsRow, error) {
	return nil, nil
}
func (*workingProvider) Apply(context.Context, int64, billing.Event) error { return nil }
func (*workingProvider) MarkProcessed(context.Context, int64) error        { return nil }
func (*workingProvider) SubscribersNearExpiry(context.Context, time.Duration, int32) ([]int64, error) {
	return []int64{7}, nil
}
func (w *workingProvider) SyncUser(context.Context, int64) error { w.refreshed++; return nil }

// TestOneProviderOutageDoesNotStallTheOther. They are different companies with different
// outages; a Stripe incident that also stalled App Store renewals would be an outage we
// invented ourselves.
func TestOneProviderOutageDoesNotStallTheOther(t *testing.T) {
	ctx := context.Background()

	if _, failed := applyPending(ctx, "broken", brokenProvider{}, 10); failed == 0 {
		t.Fatal("a provider that cannot be read reported no failure; the run would exit 0 having done nothing")
	}
	if _, failed := refreshNearExpiry(ctx, "broken", brokenProvider{}, 10); failed == 0 {
		t.Fatal("a failed near-expiry read reported no failure")
	}

	working := &workingProvider{}
	refreshed, failed := refreshNearExpiry(ctx, "working", working, 10)
	if failed != 0 || refreshed != 1 || working.refreshed != 1 {
		t.Fatalf("the healthy provider refreshed=%d failed=%d (calls=%d), want one clean refresh", refreshed, failed, working.refreshed)
	}
}
