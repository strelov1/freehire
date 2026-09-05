//go:build integration

// The whole referral flow, end to end, against a real Postgres and a stubbed provider: an
// invited signup earns nothing until the invitee's invoice actually collects, then the
// referrer is credited exactly once — and a second run of the same pass moves nothing.
//
// It lives here because this is where the two halves meet. internal/identity/promo decides
// what is owed and never talks to a provider; internal/identity/billing talks to the
// provider and never learns why. Only the worker holds both, so only the worker can be
// asked whether they agree.
//
// Run with: go test -tags=integration ./cmd/billing-sync/
package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/identity/billing"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

const testPrice = "price_pro_monthly"

// providerStub answers what the referral pass asks: the sale price, the invitee's invoices,
// a customer for a referrer who has none, and the credit itself.
type providerStub struct {
	// collected keys the customers whose invoices actually took money.
	collected map[string]bool
	credits   map[string]int
	customers int
}

func (p *providerStub) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/prices/"):
			_, _ = w.Write([]byte(`{"id":"` + testPrice + `","unit_amount":500,"currency":"usd",` +
				`"recurring":{"interval":"month"}}`))

		case r.URL.Path == "/invoices":
			// An invitee whose subscription is active but collected nothing looks exactly
			// like this: an invoice exists, amount_paid is zero.
			if p.collected[r.URL.Query().Get("customer")] {
				_, _ = w.Write([]byte(`{"data":[{"amount_paid":500}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":[{"amount_paid":0}]}`))

		case r.URL.Path == "/customers":
			p.customers++
			_, _ = w.Write([]byte(`{"id":"cus_minted"}`))

		case strings.HasSuffix(r.URL.Path, "/balance_transactions"):
			customer := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/customers/"), "/balance_transactions")
			p.credits[customer]++
			_, _ = w.Write([]byte(`{"id":"cbtxn_1"}`))

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func newUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("inserting %s: %v", email, err)
	}
	return id
}

func TestAReferralIsSettledOnceTheInviteeHasPaid(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	stub := &providerStub{collected: map[string]bool{}, credits: map[string]int{}}
	srv := httptest.NewServer(stub.handler())
	t.Cleanup(srv.Close)

	t.Setenv("STRIPE_SECRET_KEY", "sk_test")
	t.Setenv("STRIPE_WEBHOOK_SECRET", "whsec_test")
	t.Setenv("STRIPE_PRICE_IDS", testPrice)
	t.Setenv("FRONTEND_ORIGIN", "https://freehire.me")
	t.Setenv("INVITE_REWARD_MAX_PER_USER", "12")

	queries := db.New(pool)
	provider := billing.NewWithBase(billing.ConfigFromEnv(), queries, srv.URL)

	referrer := newUser(t, pool, "referrer@example.com")
	invitee := newUser(t, pool, "invitee@example.com")
	if _, err := pool.Exec(ctx,
		`UPDATE users SET stripe_customer_id = 'cus_invitee' WHERE id = $1`, invitee); err != nil {
		t.Fatalf("binding the invitee: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO invite_rewards (referrer_id, referee_id) VALUES ($1, $2)`,
		referrer, invitee); err != nil {
		t.Fatalf("attributing: %v", err)
	}

	// The invitee has a subscription but has paid nothing yet.
	if failures := settleRewards(ctx, provider, queries, 100); failures != 0 {
		t.Fatalf("failures = %d, want 0", failures)
	}
	var status string
	if err := pool.QueryRow(ctx,
		`SELECT status FROM invite_rewards WHERE referee_id = $1`, invitee).Scan(&status); err != nil {
		t.Fatalf("reading the reward: %v", err)
	}
	if status != "pending" {
		t.Fatalf("status = %q, want pending — an active subscription that collected nothing "+
			"is a trial or a total discount, and rewarding it mints credit out of a discount",
			status)
	}
	if len(stub.credits) != 0 {
		t.Fatalf("credits placed = %v, want none", stub.credits)
	}

	// Now the money moves.
	stub.collected["cus_invitee"] = true
	if failures := settleRewards(ctx, provider, queries, 100); failures != 0 {
		t.Fatalf("failures after payment = %d, want 0", failures)
	}

	var amount int64
	if err := pool.QueryRow(ctx,
		`SELECT status, amount_cents FROM invite_rewards WHERE referee_id = $1`, invitee).
		Scan(&status, &amount); err != nil {
		t.Fatalf("re-reading the reward: %v", err)
	}
	if status != "granted" || amount != 250 {
		t.Fatalf("status=%q amount=%d, want granted and 250 — half of a 500-cent price",
			status, amount)
	}
	if stub.customers != 1 {
		t.Fatalf("customers created = %d, want 1 — the referrer has never bought, so a "+
			"balance credit needs a customer minted for them", stub.customers)
	}
	if stub.credits["cus_minted"] != 1 {
		t.Fatalf("credits = %v, want one on cus_minted", stub.credits)
	}

	// And again, changing nothing. Both halves are guarded on the row's own state, which is
	// what makes stopping this pass mid-way free.
	if failures := settleRewards(ctx, provider, queries, 100); failures != 0 {
		t.Fatalf("failures on the repeat = %d, want 0", failures)
	}
	if stub.credits["cus_minted"] != 1 {
		t.Fatalf("credits after a repeat = %v, want still one", stub.credits)
	}
	if stub.customers != 1 {
		t.Fatalf("customers after a repeat = %d, want still 1", stub.customers)
	}
}

func TestTheCeilingStopsAReferrerEarningMore(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	stub := &providerStub{collected: map[string]bool{"cus_payer": true}, credits: map[string]int{}}
	srv := httptest.NewServer(stub.handler())
	t.Cleanup(srv.Close)

	t.Setenv("STRIPE_SECRET_KEY", "sk_test")
	t.Setenv("STRIPE_WEBHOOK_SECRET", "whsec_test")
	t.Setenv("STRIPE_PRICE_IDS", testPrice)
	t.Setenv("FRONTEND_ORIGIN", "https://freehire.me")
	t.Setenv("INVITE_REWARD_MAX_PER_USER", "1")

	queries := db.New(pool)
	provider := billing.NewWithBase(billing.ConfigFromEnv(), queries, srv.URL)

	referrer := newUser(t, pool, "popular@example.com")
	// One reward already earned, which at a ceiling of one is the ceiling.
	spent := newUser(t, pool, "spent@example.com")
	if _, err := pool.Exec(ctx,
		`INSERT INTO invite_rewards (referrer_id, referee_id, status, amount_cents, granted_at, delivered_at)
		 VALUES ($1, $2, 'granted', 250, now(), now())`, referrer, spent); err != nil {
		t.Fatalf("seeding the earned reward: %v", err)
	}

	payer := newUser(t, pool, "payer@example.com")
	if _, err := pool.Exec(ctx,
		`UPDATE users SET stripe_customer_id = 'cus_payer' WHERE id = $1`, payer); err != nil {
		t.Fatalf("binding the payer: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO invite_rewards (referrer_id, referee_id) VALUES ($1, $2)`,
		referrer, payer); err != nil {
		t.Fatalf("attributing: %v", err)
	}

	if failures := settleRewards(ctx, provider, queries, 100); failures != 0 {
		t.Fatalf("failures = %d, want 0", failures)
	}

	var status string
	if err := pool.QueryRow(ctx,
		`SELECT status FROM invite_rewards WHERE referee_id = $1`, payer).Scan(&status); err != nil {
		t.Fatalf("reading the reward: %v", err)
	}
	if status != "pending" {
		t.Fatalf("status = %q, want pending — past the ceiling the row stays put and nothing "+
			"is credited", status)
	}
	if len(stub.credits) != 0 {
		t.Fatalf("credits = %v, want none", stub.credits)
	}
}
