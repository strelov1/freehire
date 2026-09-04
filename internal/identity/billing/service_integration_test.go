//go:build integration

// Integration tests for billing against a real Postgres: a redelivered event is recorded
// once, the account is bound to the provider's customer on first sight, a sync derives
// users.pro_until whole and is therefore idempotent, and deleting an account takes its
// billing events with it. Run with: go test -tags=integration ./internal/identity/billing/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

// newService builds a Service whose provider is a stub returning whatever the test hands it.
// The client field is replaced directly rather than through a constructor seam: these are
// same-package tests, and inventing an exported injection point for them would put a hole in
// the API for the benefit of nobody outside it.
func newService(t *testing.T, h http.HandlerFunc) (*Service, *pgxpool.Pool) {
	t.Helper()
	pool := testdb.Pool(t)
	setEnv(t, "sk_test", testSecret, proPrice, "https://freehire.me")

	s := New(ConfigFromEnv(), db.New(pool))
	if h != nil {
		srv := httptest.NewServer(h)
		t.Cleanup(srv.Close)
		s.client = newClient("sk_test", srv.URL, srv.Client())
	}
	return s, pool
}

func insertUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("insert user %s: %v", email, err)
	}
	return id
}

func proUntil(t *testing.T, pool *pgxpool.Pool, userID int64) *time.Time {
	t.Helper()
	var out *time.Time
	if err := pool.QueryRow(context.Background(),
		`SELECT pro_until FROM users WHERE id = $1`, userID).Scan(&out); err != nil {
		t.Fatalf("read pro_until: %v", err)
	}
	return out
}

func customerOf(t *testing.T, pool *pgxpool.Pool, userID int64) *string {
	t.Helper()
	var out *string
	if err := pool.QueryRow(context.Background(),
		`SELECT stripe_customer_id FROM users WHERE id = $1`, userID).Scan(&out); err != nil {
		t.Fatalf("read stripe_customer_id: %v", err)
	}
	return out
}

func countEvents(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM billing_events`).Scan(&n); err != nil {
		t.Fatalf("count events: %v", err)
	}
	return n
}

// event builds a delivery naming a customer, and optionally carrying our account reference
// the way a completed checkout does.
func event(id, customer, userRef string) Event {
	return Event{
		ID:         id,
		CustomerID: customer,
		UserRef:    userRef,
		Type:       "invoice.paid",
		Payload:    json.RawMessage(fmt.Sprintf(`{"customer":%q,"client_reference_id":%q}`, customer, userRef)),
	}
}

// subscriptionsWith serves a customer holding one entitling subscription ending at ends, or
// none at all when ends is empty.
func subscriptionsWith(ends string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if ends == "" {
			_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
			return
		}
		at, err := time.Parse(time.RFC3339, ends)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = fmt.Fprintf(w,
			`{"object":"list","data":[{"status":"active","current_period_end":%d,"items":{"data":[{"price":{"id":%q}}]}}]}`,
			at.Unix(), proPrice)
	}
}

// TestRecordIsIdempotent is the property the provider's retry behaviour demands: it
// redelivers anything it did not get a 2xx for, reusing the event id.
func TestRecordIsIdempotent(t *testing.T) {
	s, pool := newService(t, nil)
	ctx := context.Background()
	userID := insertUser(t, pool, "sub@example.com")

	ev := event("evt_1", "cus_1", fmt.Sprint(userID))

	rowID, recorded, err := s.Record(ctx, ev)
	if err != nil || !recorded {
		t.Fatalf("first delivery: recorded=%v err=%v", recorded, err)
	}

	_, recorded, err = s.Record(ctx, ev)
	if err != nil {
		t.Fatalf("redelivery: %v", err)
	}
	if recorded {
		t.Fatal("a redelivery must report itself as already recorded")
	}
	if n := countEvents(t, pool); n != 1 {
		t.Fatalf("want 1 stored event, got %d", n)
	}
	if rowID == 0 {
		t.Fatal("want a row id from the first delivery")
	}
}

// TestRecordBindsTheCustomer is what makes the reconciler possible at all: a webhook names a
// customer, but a scheduled re-check starts from a user and has to name one.
func TestRecordBindsTheCustomer(t *testing.T) {
	s, pool := newService(t, nil)
	ctx := context.Background()
	userID := insertUser(t, pool, "bind@example.com")

	if _, _, err := s.Record(ctx, event("evt_bind", "cus_bind", fmt.Sprint(userID))); err != nil {
		t.Fatalf("record: %v", err)
	}

	got := customerOf(t, pool, userID)
	if got == nil || *got != "cus_bind" {
		t.Fatalf("want the account bound to cus_bind, got %v", got)
	}

	// And a LATER event that carries only the customer must still resolve to the account —
	// that is the whole point of storing the binding.
	if _, _, err := s.Record(ctx, event("evt_later", "cus_bind", "")); err != nil {
		t.Fatalf("later event: %v", err)
	}
	var boundUser *int64
	if err := pool.QueryRow(ctx,
		`SELECT user_id FROM billing_events WHERE event_id = 'evt_later'`).Scan(&boundUser); err != nil {
		t.Fatalf("read event: %v", err)
	}
	if boundUser == nil || *boundUser != userID {
		t.Fatalf("a later event did not resolve through the binding: %v", boundUser)
	}
}

// TestRecordKeepsAnUnattributableEvent covers the events that are about nothing we meter.
// A row we cannot attribute is evidence; a row we refused to write is nothing.
func TestRecordKeepsAnUnattributableEvent(t *testing.T) {
	s, pool := newService(t, nil)
	ctx := context.Background()

	if _, recorded, err := s.Record(ctx, event("evt_orphan", "cus_unknown", "")); err != nil || !recorded {
		t.Fatalf("recorded=%v err=%v", recorded, err)
	}

	var userID *int64
	if err := pool.QueryRow(ctx,
		`SELECT user_id FROM billing_events WHERE event_id = 'evt_orphan'`).Scan(&userID); err != nil {
		t.Fatalf("read event: %v", err)
	}
	if userID != nil {
		t.Fatalf("want a NULL user, got %d", *userID)
	}
}

func TestSyncDerivesProUntil(t *testing.T) {
	s, pool := newService(t, subscriptionsWith("2026-10-01T00:00:00Z"))
	ctx := context.Background()
	userID := insertUser(t, pool, "pro@example.com")

	if _, _, err := s.Record(ctx, event("evt_pro", "cus_pro", fmt.Sprint(userID))); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := s.SyncUser(ctx, userID); err != nil {
		t.Fatalf("sync: %v", err)
	}

	got := proUntil(t, pool, userID)
	if got == nil {
		t.Fatal("want pro_until set")
	}
	if want := "2026-10-01T00:00:00Z"; got.UTC().Format(time.RFC3339) != want {
		t.Fatalf("want %s, got %s", want, got.UTC().Format(time.RFC3339))
	}

	// Applying the same provider state again must change nothing. This is what makes a
	// redelivered event, a reconciler pass and a retry all free.
	if err := s.SyncUser(ctx, userID); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if again := proUntil(t, pool, userID); again == nil || !again.Equal(*got) {
		t.Fatalf("not idempotent: %v then %v", got, again)
	}
}

// TestSyncClearsALapsedSubscription is the cancellation, the refund and the failed card all
// at once: the subscription is simply no longer entitling, and no code branch was needed.
func TestSyncClearsALapsedSubscription(t *testing.T) {
	s, pool := newService(t, subscriptionsWith(""))
	ctx := context.Background()
	userID := insertUser(t, pool, "lapsed@example.com")

	if _, _, err := s.Record(ctx, event("evt_lapsed", "cus_lapsed", fmt.Sprint(userID))); err != nil {
		t.Fatalf("record: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE users SET pro_until = now() + interval '30 days' WHERE id = $1`, userID); err != nil {
		t.Fatalf("seed pro_until: %v", err)
	}

	if err := s.SyncUser(ctx, userID); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if got := proUntil(t, pool, userID); got != nil {
		t.Fatalf("want pro_until cleared, got %s", got)
	}
}

// TestSyncRefusesAnAccountThatNeverPaid guards the read: without a binding there is no
// customer to ask about, and inventing one would be worse than saying so.
func TestSyncRefusesAnAccountThatNeverPaid(t *testing.T) {
	s, pool := newService(t, subscriptionsWith("2026-10-01T00:00:00Z"))
	userID := insertUser(t, pool, "never@example.com")

	if err := s.SyncUser(context.Background(), userID); err == nil {
		t.Fatal("want an error for an account with no customer")
	}
}

// TestApplyStampsTheEvent walks the whole webhook path after the response has gone out.
func TestApplyStampsTheEvent(t *testing.T) {
	s, pool := newService(t, subscriptionsWith("2026-10-01T00:00:00Z"))
	ctx := context.Background()
	userID := insertUser(t, pool, "apply@example.com")

	ev := event("evt_apply", "cus_apply", fmt.Sprint(userID))
	rowID, _, err := s.Record(ctx, ev)
	if err != nil {
		t.Fatalf("record: %v", err)
	}

	pending, err := s.PendingEvents(ctx, 10)
	if err != nil || len(pending) != 1 {
		t.Fatalf("want 1 unprocessed event, got %d (err %v)", len(pending), err)
	}

	if err := s.Apply(ctx, rowID, ev); err != nil {
		t.Fatalf("apply: %v", err)
	}

	if pending, err = s.PendingEvents(ctx, 10); err != nil || len(pending) != 0 {
		t.Fatalf("want no unprocessed events, got %d (err %v)", len(pending), err)
	}
	if got := proUntil(t, pool, userID); got == nil {
		t.Fatal("apply did not sync the plan")
	}
}

// TestSubscribersNearExpiry is the reconciler's second pass. It must find a subscriber whose
// renewal webhook never arrived, and must not walk accounts that never transacted.
func TestSubscribersNearExpiry(t *testing.T) {
	s, pool := newService(t, nil)
	ctx := context.Background()

	subscriber := insertUser(t, pool, "near@example.com")
	stranger := insertUser(t, pool, "stranger@example.com")

	for _, id := range []int64{subscriber, stranger} {
		if _, err := pool.Exec(ctx,
			`UPDATE users SET pro_until = now() + interval '1 hour' WHERE id = $1`, id); err != nil {
			t.Fatalf("seed pro_until: %v", err)
		}
	}
	if _, _, err := s.Record(ctx, event("evt_near", "cus_near", fmt.Sprint(subscriber))); err != nil {
		t.Fatalf("record: %v", err)
	}

	ids, err := s.SubscribersNearExpiry(ctx, 24*time.Hour, 100)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("want exactly the subscriber, got %d candidates", len(ids))
	}
	if ids[0] != subscriber {
		t.Fatalf("want user %d, got %d — an account that never transacted must not be a candidate", subscriber, ids[0])
	}
}

// TestARenewalWithNoWebhookIsRepaired is the reconciler's reason to exist, end to end.
func TestARenewalWithNoWebhookIsRepaired(t *testing.T) {
	s, pool := newService(t, subscriptionsWith("2027-01-01T00:00:00Z"))
	ctx := context.Background()
	userID := insertUser(t, pool, "renewed@example.com")

	if _, _, err := s.Record(ctx, event("evt_old", "cus_renewed", fmt.Sprint(userID))); err != nil {
		t.Fatalf("record: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE users SET pro_until = now() + interval '1 hour' WHERE id = $1`, userID); err != nil {
		t.Fatalf("seed pro_until: %v", err)
	}

	ids, err := s.SubscribersNearExpiry(ctx, 24*time.Hour, 100)
	if err != nil {
		t.Fatalf("candidates: %v", err)
	}
	for _, id := range ids {
		if err := s.SyncUser(ctx, id); err != nil {
			t.Fatalf("sync %d: %v", id, err)
		}
	}

	got := proUntil(t, pool, userID)
	if got == nil || got.UTC().Format(time.RFC3339) != "2027-01-01T00:00:00Z" {
		t.Fatalf("want the renewal picked up, got %v", got)
	}
}

// TestApplyResolvesFromTheStoredUserWhenNoBindingExists is the reconciler's replay path,
// and the bug it guards is the most expensive one this package can have.
//
// A first purchase arrives as a checkout completion carrying our account id but no binding
// yet. If the binding write then fails, the stored row still knows whose event it is — but a
// replay rebuilt from the customer id alone would resolve to nobody, and the worker treats
// "nobody" as unattributable and stamps the row processed. A real, paid subscription, marked
// done forever, silently.
func TestApplyResolvesFromTheStoredUserWhenNoBindingExists(t *testing.T) {
	s, pool := newService(t, subscriptionsWith("2026-10-01T00:00:00Z"))
	ctx := context.Background()
	userID := insertUser(t, pool, "unbound@example.com")

	ev := event("evt_unbound", "cus_unbound", fmt.Sprint(userID))
	rowID, _, err := s.Record(ctx, ev)
	if err != nil {
		t.Fatalf("record: %v", err)
	}

	// Simulate the binding never having been written.
	if _, err := pool.Exec(ctx,
		`UPDATE users SET stripe_customer_id = NULL WHERE id = $1`, userID); err != nil {
		t.Fatalf("clear binding: %v", err)
	}

	// FIRST, the failure the fix removes: a replay built from the customer alone resolves to
	// nobody, and "nobody" is exactly what the worker stamps as unattributable. Asserted
	// before the repair, because the repair is what makes it stop happening.
	bare := Event{ID: ev.ID, CustomerID: ev.CustomerID, Type: ev.Type}
	if err := s.Apply(ctx, rowID, bare); !errors.Is(err, ErrUnknownSubscriber) {
		t.Fatalf("want ErrUnknownSubscriber without the stored user, got %v", err)
	}

	// THEN the replay the worker actually builds: the customer from the row, plus the user
	// the row already stored. It repairs the binding on the way through, so the sync it needs
	// can happen at all.
	replay := Event{ID: ev.ID, CustomerID: ev.CustomerID, Type: ev.Type, UserRef: fmt.Sprint(userID)}
	if err := s.Apply(ctx, rowID, replay); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if got := proUntil(t, pool, userID); got == nil {
		t.Fatal("a paid subscription was not applied on replay")
	}
	if bound := customerOf(t, pool, userID); bound == nil || *bound != ev.CustomerID {
		t.Fatalf("the replay did not repair the binding: %v", bound)
	}
}

// TestDeletingAnAccountErasesItsBillingEvents asserts the cascade the deletion surface
// promises.
func TestDeletingAnAccountErasesItsBillingEvents(t *testing.T) {
	s, pool := newService(t, nil)
	ctx := context.Background()
	userID := insertUser(t, pool, "gone@example.com")

	if _, _, err := s.Record(ctx, event("evt_gone", "cus_gone", fmt.Sprint(userID))); err != nil {
		t.Fatalf("record: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	if n := countEvents(t, pool); n != 0 {
		t.Fatalf("want the events gone with the account, got %d", n)
	}
}
