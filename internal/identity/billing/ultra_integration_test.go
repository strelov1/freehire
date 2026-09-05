//go:build integration

// The three-tier entitlement path end to end, against a real Postgres and a stubbed
// provider: which price confers which tier, that each provider writes only its own columns,
// and that an unset Ultra price list leaves everything behaving as it did before.
//
// These are properties of the SCHEMA and the seam together, so a fake of either would be
// asserting its own behaviour. Run with: go test -tags=integration ./internal/identity/billing/
package billing

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
)

const ultraPrice = "price_ultra_monthly"

// subscriptionsFor answers the provider's subscription listing with one active subscription
// for the given price.
func subscriptionsFor(price string, until time.Time) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"data":[{"status":"active","items":{"data":[{"current_period_end":%d,`+
			`"price":{"id":%q}}]}}]}`, until.Unix(), price)
	}
}

// newTieredService is newService with an Ultra price list configured too.
func newTieredService(t *testing.T, h http.HandlerFunc) (*Service, *pgxpool.Pool) {
	t.Helper()
	s, pool := newService(t, h)
	t.Setenv("STRIPE_ULTRA_PRICE_IDS", ultraPrice)
	return NewWithBase(ConfigFromEnv(), db.New(pool), s.client.baseURL), pool
}

// bindCustomer points an account at a provider customer, which is what makes it askable.
func bindCustomer(t *testing.T, pool *pgxpool.Pool, userID int64, customer string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`UPDATE users SET stripe_customer_id = $1 WHERE id = $2`, customer, userID); err != nil {
		t.Fatalf("binding the customer: %v", err)
	}
}

// untilsOf reads both derived columns.
func untilsOf(t *testing.T, pool *pgxpool.Pool, userID int64) (pro, ultra pgtype.Timestamptz) {
	t.Helper()
	err := pool.QueryRow(context.Background(),
		`SELECT pro_until, ultra_until FROM users WHERE id = $1`, userID).Scan(&pro, &ultra)
	if err != nil {
		t.Fatalf("reading the plan columns: %v", err)
	}
	return pro, ultra
}

func TestAnUltraPriceConfersUltraAndNotPro(t *testing.T) {
	until := time.Now().UTC().Add(30 * 24 * time.Hour).Truncate(time.Second)
	s, pool := newTieredService(t, subscriptionsFor(ultraPrice, until))
	ctx := context.Background()

	userID := insertUser(t, pool, "ultra-buyer@example.com")
	bindCustomer(t, pool, userID, "cus_ultra")

	if err := s.SyncCaller(ctx, userID); err != nil {
		t.Fatalf("SyncCaller: %v", err)
	}

	pro, ultra := untilsOf(t, pool, userID)
	if !ultra.Valid || !ultra.Time.UTC().Equal(until) {
		t.Fatalf("ultra_until = %v (valid=%v), want %v", ultra.Time, ultra.Valid, until)
	}
	if pro.Valid {
		t.Fatalf("pro_until = %v, want NULL — a subscription for an Ultra price confers Ultra "+
			"and nothing else, or the tiers would be indistinguishable downstream", pro.Time)
	}
}

func TestSyncingBothTiersIsOneWriteAndClearsWhatLapsed(t *testing.T) {
	until := time.Now().UTC().Add(30 * 24 * time.Hour).Truncate(time.Second)
	price := proPrice
	s, pool := newTieredService(t, func(w http.ResponseWriter, r *http.Request) {
		subscriptionsFor(price, until)(w, r)
	})
	ctx := context.Background()

	userID := insertUser(t, pool, "upgrader@example.com")
	bindCustomer(t, pool, userID, "cus_upgrade")

	if err := s.SyncCaller(ctx, userID); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if pro, ultra := untilsOf(t, pool, userID); !pro.Valid || ultra.Valid {
		t.Fatalf("after buying Pro: pro=%v ultra=%v, want pro set and ultra NULL", pro.Valid, ultra.Valid)
	}

	// The same customer now holds an Ultra subscription instead. One re-read has to move
	// both columns: were they written separately, there would be an instant with neither.
	price = ultraPrice
	if err := s.SyncCaller(ctx, userID); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	pro, ultra := untilsOf(t, pool, userID)
	if pro.Valid {
		t.Fatalf("pro_until = %v after the Pro subscription went away, want NULL — a provider "+
			"that could not clear its own column could never record a cancellation", pro.Time)
	}
	if !ultra.Valid {
		t.Fatal("ultra_until is NULL after the account moved to Ultra")
	}
}

func TestAStoreSyncLeavesTheWebUltraEntitlementAlone(t *testing.T) {
	until := time.Now().UTC().Add(30 * 24 * time.Hour).Truncate(time.Second)
	_, pool := newTieredService(t, subscriptionsFor(ultraPrice, until))
	ctx := context.Background()
	q := db.New(pool)

	userID := insertUser(t, pool, "both-providers@example.com")
	if err := q.SetStripeEntitlement(ctx, db.SetStripeEntitlementParams{
		ID: userID, UltraUntil: pgtype.Timestamptz{Time: until, Valid: true},
	}); err != nil {
		t.Fatalf("seeding the web entitlement: %v", err)
	}

	// The store provider reports nothing, for either tier, and writes that.
	if err := q.SetRevenueCatEntitlement(ctx, db.SetRevenueCatEntitlementParams{ID: userID}); err != nil {
		t.Fatalf("store sync: %v", err)
	}

	_, ultra := untilsOf(t, pool, userID)
	if !ultra.Valid || !ultra.Time.UTC().Equal(until) {
		t.Fatalf("ultra_until = %v (valid=%v) after a store sync, want the web value intact — "+
			"a provider saying 'I confer nothing' must not revoke what another origin confers",
			ultra.Time, ultra.Valid)
	}
}

func TestWithNoUltraPricesNobodyIsEverUltra(t *testing.T) {
	until := time.Now().UTC().Add(30 * 24 * time.Hour).Truncate(time.Second)
	// newService, not newTieredService: the Ultra list is left unset, which is what every
	// deployment looks like until somebody configures one.
	s, pool := newService(t, subscriptionsFor(proPrice, until))
	ctx := context.Background()

	userID := insertUser(t, pool, "pro-only@example.com")
	bindCustomer(t, pool, userID, "cus_prolonly")

	if err := s.SyncCaller(ctx, userID); err != nil {
		t.Fatalf("SyncCaller: %v", err)
	}

	pro, ultra := untilsOf(t, pool, userID)
	if !pro.Valid {
		t.Fatal("pro_until is NULL — configuring no Ultra prices must leave Pro untouched")
	}
	if ultra.Valid {
		t.Fatalf("ultra_until = %v with no Ultra price configured, want NULL", ultra.Time)
	}
}
