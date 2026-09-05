//go:build integration

// Integration tests for the store provider against a real Postgres and a stub RevenueCat.
//
// Run with: go test -tags=integration ./internal/identity/billing/
package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

const rcSecret = "whsec_revenuecat"

// newRevenueCat builds a store provider pointed at a stub, and reports how many times the
// stub was asked anything — which is itself an assertion in one of the tests below.
func newRevenueCat(t *testing.T, h http.HandlerFunc) (*RevenueCat, *pgxpool.Pool, *atomic.Int64) {
	t.Helper()
	pool := testdb.Pool(t)
	t.Setenv("REVENUECAT_API_KEY", "sk_rc_test")
	t.Setenv("REVENUECAT_WEBHOOK_SECRET", rcSecret)
	t.Setenv("REVENUECAT_ENTITLEMENT", "pro")

	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		h(w, r)
	}))
	t.Cleanup(srv.Close)

	return NewRevenueCatWithBase(RevenueCatConfigFromEnv(), db.New(pool), srv.URL), pool, &calls
}

// entitledUntil answers every request with one subscriber holding the pro entitlement.
func entitledUntil(expires string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body := `{"subscriber":{"entitlements":{"pro":{"expires_date":` + expires + `}}}}`
		_, _ = w.Write([]byte(body))
	}
}

func signRevenueCat(t *testing.T, body []byte, at time.Time) string {
	t.Helper()
	ts := strconv.FormatInt(at.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(rcSecret))
	mac.Write([]byte(ts + "."))
	mac.Write(body)
	return "t=" + ts + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

func rcDelivery(eventID string, userID int64) []byte {
	return []byte(fmt.Sprintf(
		`{"api_version":"1.0","event":{"id":%q,"type":"INITIAL_PURCHASE","app_user_id":"%d"}}`,
		eventID, userID))
}

// TestAStorePurchaseConfersPro is the whole point of the change, end to end: a signed
// delivery arrives, and the account is Pro because RevenueCat says so.
func TestAStorePurchaseConfersPro(t *testing.T) {
	until := time.Now().UTC().Add(30 * 24 * time.Hour).Truncate(time.Second)
	s, pool, _ := newRevenueCat(t, entitledUntil(`"`+until.Format(time.RFC3339)+`"`))
	ctx := context.Background()

	userID := insertUser(t, pool, "store-buyer@example.com")
	body := rcDelivery("evt_purchase", userID)

	ev, err := s.Accept(body, signRevenueCat(t, body, time.Now()), time.Now())
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	rowID, recorded, err := s.Record(ctx, ev)
	if err != nil || !recorded {
		t.Fatalf("record: recorded=%v err=%v", recorded, err)
	}
	if err := s.Apply(ctx, rowID, ev); err != nil {
		t.Fatalf("apply: %v", err)
	}

	if got := readSource(t, pool, userID, "pro_until_revenuecat"); got == nil || !got.Equal(until) {
		t.Fatalf("pro_until_revenuecat = %v, want %v", got, until)
	}
	if got := proUntil(t, pool, userID); got == nil || !got.Equal(until) {
		t.Fatalf("pro_until = %v, want the store purchase's %v", got, until)
	}
}

// TestReadingASubscriberNeedsAFootprint guards a read that WRITES. RevenueCat's subscribers
// endpoint creates the subscriber when the id is unknown, so asking about an account that
// never bought anything enrols it with the provider — permanently, and for every account we
// have if a reconciler pass ever walks the user table.
func TestReadingASubscriberNeedsAFootprint(t *testing.T) {
	s, pool, calls := newRevenueCat(t, entitledUntil(`"2030-01-01T00:00:00Z"`))
	ctx := context.Background()

	stranger := insertUser(t, pool, "never-bought@example.com")

	err := s.SyncUser(ctx, stranger)
	if !errors.Is(err, ErrNoSubscription) {
		t.Fatalf("sync of an account with no footprint returned %v, want ErrNoSubscription", err)
	}
	if n := calls.Load(); n != 0 {
		t.Fatalf("the provider was called %d times for an account it has never seen; that call would have created the subscriber", n)
	}
}

// TestARecordedDeliveryIsAFootprint is the other half: once RevenueCat has told us about an
// account, asking about it creates nothing that does not already exist.
func TestARecordedDeliveryIsAFootprint(t *testing.T) {
	s, pool, calls := newRevenueCat(t, entitledUntil(`"2030-01-01T00:00:00Z"`))
	ctx := context.Background()

	userID := insertUser(t, pool, "has-footprint@example.com")
	body := rcDelivery("evt_footprint", userID)
	ev, err := s.Accept(body, signRevenueCat(t, body, time.Now()), time.Now())
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if _, _, err := s.Record(ctx, ev); err != nil {
		t.Fatalf("record: %v", err)
	}

	if err := s.SyncUser(ctx, userID); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if n := calls.Load(); n != 1 {
		t.Fatalf("the provider was called %d times, want exactly 1", n)
	}
}

// TestEventIdsDoNotCollideAcrossProviders is why billing_events is unique on
// (provider, event_id) rather than on the id alone. The two namespaces are unrelated opaque
// strings, and discovering that they overlap by silently dropping a payment is not a good way
// to find out.
func TestEventIdsDoNotCollideAcrossProviders(t *testing.T) {
	ctx := context.Background()
	stripeSvc, pool := newService(t, subscriptionsWith(""))

	t.Setenv("REVENUECAT_API_KEY", "sk_rc_test")
	t.Setenv("REVENUECAT_WEBHOOK_SECRET", rcSecret)
	t.Setenv("REVENUECAT_ENTITLEMENT", "pro")
	rcSvc := NewRevenueCatWithBase(RevenueCatConfigFromEnv(), db.New(pool), "http://127.0.0.1:1")

	userID := insertUser(t, pool, "same-id@example.com")
	const shared = "evt_collision"

	if _, recorded, err := stripeSvc.Record(ctx, event(shared, "cus_x", fmt.Sprint(userID))); err != nil || !recorded {
		t.Fatalf("stripe record: recorded=%v err=%v", recorded, err)
	}
	rcEvent, err := parseRevenueCatEvent(rcDelivery(shared, userID))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, recorded, err := rcSvc.Record(ctx, rcEvent); err != nil || !recorded {
		t.Fatalf("revenuecat record: recorded=%v err=%v; the same id from another provider must not read as a redelivery", recorded, err)
	}

	// And a genuine redelivery of either still records once.
	if _, recorded, err := rcSvc.Record(ctx, rcEvent); err != nil || recorded {
		t.Fatalf("revenuecat redelivery: recorded=%v err=%v, want it recognised as a duplicate", recorded, err)
	}
}

// TestTheStoreWindowAdmitsALateRetry pins a deliberate difference from Stripe rather than an
// accident. RevenueCat's last retry lands 80 minutes after the first and its documentation
// does not say whether retries are re-signed; a five-minute window would reject them all and
// leave the reconciler as the only path, silently.
func TestTheStoreWindowAdmitsALateRetry(t *testing.T) {
	s, pool, _ := newRevenueCat(t, entitledUntil(`"2030-01-01T00:00:00Z"`))
	userID := insertUser(t, pool, "late-retry@example.com")

	body := rcDelivery("evt_late", userID)
	signedAt := time.Now().Add(-80 * time.Minute)

	if _, err := s.Accept(body, signRevenueCat(t, body, signedAt), time.Now()); err != nil {
		t.Fatalf("an 80-minute-old delivery was refused: %v", err)
	}

	// Still bounded, though: a signature with no time limit is a bearer credential.
	tooOld := time.Now().Add(-2 * revenuecatSignatureWindow)
	if _, err := s.Accept(body, signRevenueCat(t, body, tooOld), time.Now()); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("a delivery signed %s ago was accepted (%v); the window must still bound it", revenuecatSignatureWindow*2, err)
	}
}

// TestARenewalWithNoStoreWebhookIsRepaired is the reconciler's whole reason to exist for this
// provider. RevenueCat gives up after five retries, roughly two and a half hours after the
// event; past that point nothing else will ever notice that a subscription renewed.
func TestARenewalWithNoStoreWebhookIsRepaired(t *testing.T) {
	renewedUntil := time.Now().UTC().Add(30 * 24 * time.Hour).Truncate(time.Second)
	s, pool, _ := newRevenueCat(t, entitledUntil(`"`+renewedUntil.Format(time.RFC3339)+`"`))
	ctx := context.Background()

	userID := insertUser(t, pool, "silent-renewal@example.com")

	// The state after a purchase whose renewal webhook never arrived: the account holds a
	// store entitlement, and it is about to lapse as far as we know.
	if _, err := pool.Exec(ctx,
		`UPDATE users SET pro_until_revenuecat = now() + interval '1 hour' WHERE id = $1`, userID); err != nil {
		t.Fatalf("seed the lapsing entitlement: %v", err)
	}

	ids, err := s.SubscribersNearExpiry(ctx, 24*time.Hour, 100)
	if err != nil {
		t.Fatalf("near expiry: %v", err)
	}
	if len(ids) != 1 || ids[0] != userID {
		t.Fatalf("near expiry returned %v, want just the lapsing subscriber %d", ids, userID)
	}

	if err := s.SyncUser(ctx, userID); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if got := readSource(t, pool, userID, "pro_until_revenuecat"); got == nil || !got.Equal(renewedUntil) {
		t.Fatalf("pro_until_revenuecat = %v, want the renewal's %v", got, renewedUntil)
	}
}

// TestANonExpiringEntitlementIsRenewedRatherThanSentinelled. The column cannot say "forever",
// and a sentinel that means it is the hazard entitlement.go documents. It says "for as long
// as we keep confirming" instead, and this is the confirming.
func TestANonExpiringEntitlementIsRenewedRatherThanSentinelled(t *testing.T) {
	s, pool, _ := newRevenueCat(t, entitledUntil(`null`))
	ctx := context.Background()

	userID := insertUser(t, pool, "lifetime@example.com")
	if _, err := pool.Exec(ctx,
		`UPDATE users SET pro_until_revenuecat = now() + interval '1 hour' WHERE id = $1`, userID); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := s.SyncUser(ctx, userID); err != nil {
		t.Fatalf("sync: %v", err)
	}

	got := readSource(t, pool, userID, "pro_until_revenuecat")
	if got == nil {
		t.Fatal("a non-expiring entitlement cleared the column; that is the silent downgrade this rule exists to prevent")
	}
	// Far enough out to be Pro, near enough that a revoked grant lapses rather than lasting
	// forever — and inside the horizon, so the next pass can push it out again.
	if !got.After(time.Now().Add(revenuecatLifetimeHorizon - 24*time.Hour)) {
		t.Fatalf("pro_until_revenuecat = %v, want about %s out", got, revenuecatLifetimeHorizon)
	}
	if got.After(time.Now().Add(revenuecatLifetimeHorizon + time.Hour)) {
		t.Fatalf("pro_until_revenuecat = %v, further than the horizon — a sentinel by another name", got)
	}
}

// TestAFirstPurchaseWhoseWebhookWasLostIsRecoverable is the case the sync route exists for,
// and the one an earlier draft could not serve.
//
// A first-time buyer has no recorded event and a NULL source column — that is what "first"
// means. With the stranger check inside reach(), all three recovery paths refused them:
// applyPending saw no event, dueSoon's predicate skips NULL, and the route answered
// "no_subscription" without asking anybody. The account would never have become Pro.
func TestAFirstPurchaseWhoseWebhookWasLostIsRecoverable(t *testing.T) {
	until := time.Now().UTC().Add(30 * 24 * time.Hour).Truncate(time.Second)
	s, pool, calls := newRevenueCat(t, entitledUntil(`"`+until.Format(time.RFC3339)+`"`))
	ctx := context.Background()

	// Exactly the state after a purchase whose INITIAL_PURCHASE delivery never arrived.
	buyer := insertUser(t, pool, "first-purchase-lost@example.com")

	if err := s.SyncCaller(ctx, buyer); err != nil {
		t.Fatalf("the caller's own sync failed: %v", err)
	}
	if n := calls.Load(); n != 1 {
		t.Fatalf("the provider was asked %d times, want exactly 1 — a caller asking about themselves must reach it", n)
	}
	if got := readSource(t, pool, buyer, "pro_until_revenuecat"); got == nil || !got.Equal(until) {
		t.Fatalf("pro_until_revenuecat = %v, want %v", got, until)
	}

	// And the guard still binds the path that can reach accounts in bulk.
	stranger := insertUser(t, pool, "still-a-stranger@example.com")
	before := calls.Load()
	if err := s.SyncUser(ctx, stranger); !errors.Is(err, ErrNoSubscription) {
		t.Fatalf("the bulk path synced a stranger (%v); that read would have enrolled them with the provider", err)
	}
	if calls.Load() != before {
		t.Fatal("the bulk path reached the provider for an account it has never seen")
	}
}

// TestADeliveryNamingAVanishedAccountIsRecordedNotRejected. billing_events has a foreign key
// on user_id, so trusting the id straight from the envelope turns a deleted account into a
// 500 — and after five retries RevenueCat may disable the endpoint, taking every other
// subscriber's deliveries with it.
func TestADeliveryNamingAVanishedAccountIsRecordedNotRejected(t *testing.T) {
	s, pool, _ := newRevenueCat(t, entitledUntil(`"2030-01-01T00:00:00Z"`))
	ctx := context.Background()

	gone := insertUser(t, pool, "deleted-between@example.com")
	if _, err := pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, gone); err != nil {
		t.Fatalf("delete the account: %v", err)
	}

	body := rcDelivery("evt_orphan", gone)
	ev, err := s.Accept(body, signRevenueCat(t, body, time.Now()), time.Now())
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if _, recorded, err := s.Record(ctx, ev); err != nil || !recorded {
		t.Fatalf("record: recorded=%v err=%v — an unattributable delivery must still be stored", recorded, err)
	}
}

// TestAReplayWithNoStoredUserStillResolves is the path cmd/billing-sync takes when it retries
// an event the webhook recorded but could not apply.
//
// applyPending rebuilds the event from the stored row, and it can only fill UserRef from a
// non-NULL user_id — but a NULL user_id is exactly the row that still needs attributing. With
// account() reading UserRef alone, the replay resolved nobody, the worker read that as
// "nothing to apply" and STAMPED the row processed: a purchase marked done having conferred
// nothing, and never retried.
func TestAReplayWithNoStoredUserStillResolves(t *testing.T) {
	until := time.Now().UTC().Add(30 * 24 * time.Hour).Truncate(time.Second)
	s, pool, _ := newRevenueCat(t, entitledUntil(`"`+until.Format(time.RFC3339)+`"`))
	ctx := context.Background()

	userID := insertUser(t, pool, "replay-no-stored-user@example.com")

	body := rcDelivery("evt_replay", userID)
	ev, err := s.Accept(body, signRevenueCat(t, body, time.Now()), time.Now())
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	rowID, recorded, err := s.Record(ctx, ev)
	if err != nil || !recorded {
		t.Fatalf("record: recorded=%v err=%v", recorded, err)
	}

	// The row the reconciler finds when recording could not attribute it — the subject survives
	// in app_user_id and nothing else does.
	if _, err := pool.Exec(ctx, `UPDATE billing_events SET user_id = NULL WHERE id = $1`, rowID); err != nil {
		t.Fatalf("orphan the row: %v", err)
	}

	replay := Event{ID: ev.ID, CustomerID: ev.CustomerID, Type: ev.Type}
	if err := s.Apply(ctx, rowID, replay); err != nil {
		t.Fatalf("apply: %v — a replay carrying only app_user_id must still find the account", err)
	}
	if got := readSource(t, pool, userID, "pro_until_revenuecat"); got == nil || !got.Equal(until) {
		t.Fatalf("pro_until_revenuecat = %v, want %v", got, until)
	}
}
