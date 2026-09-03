//go:build integration

// Integration tests for billing against a real Postgres: a redelivered event is recorded
// once, an identifier that was never ours is recorded rather than dropped, a sync derives
// users.pro_until whole and is therefore idempotent, and deleting an account takes its
// billing events with it. Run with: go test -tags=integration ./internal/identity/billing/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

// newService builds a Service whose provider is a stub returning whatever the test hands
// it. The client field is replaced directly rather than through a constructor seam: these
// are same-package tests, and inventing an exported injection point for them would put a
// hole in the API for the benefit of nobody outside it.
func newService(t *testing.T, h http.HandlerFunc) (*Service, *pgxpool.Pool) {
	t.Helper()
	pool := testdb.Pool(t)
	setEnv(t, "sk_test", testSecret, "", "")

	s := New(ConfigFromEnv(), db.New(pool))
	if h != nil {
		srv := httptest.NewServer(h)
		t.Cleanup(srv.Close)
		s.client = newClient("sk_test", testProjectID, srv.URL, srv.Client())
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

func countEvents(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM billing_events`).Scan(&n); err != nil {
		t.Fatalf("count events: %v", err)
	}
	return n
}

func event(id, appUserID string) Event {
	return Event{
		ID:        id,
		AppUserID: appUserID,
		Type:      "RENEWAL",
		Payload:   json.RawMessage(fmt.Sprintf(`{"id":%q,"app_user_id":%q,"type":"RENEWAL"}`, id, appUserID)),
	}
}

// subscriberWith serves a v2 customer carrying one pro entitlement expiring at expires, or
// no active entitlement at all when expires is empty.
func subscriberWith(expires string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if expires == "" {
			_, _ = w.Write([]byte(`{"active_entitlements":{"items":[]}}`))
			return
		}
		at, err := time.Parse(time.RFC3339, expires)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = fmt.Fprintf(w,
			`{"active_entitlements":{"items":[{"entitlement_id":"pro","expires_at":%d}]}}`, at.UnixMilli())
	}
}

// TestRecordIsIdempotent is the property the provider's retry behaviour demands: it
// redelivers anything it did not get a 200 for, reusing the event id.
func TestRecordIsIdempotent(t *testing.T) {
	s, pool := newService(t, nil)
	ctx := context.Background()
	userID := insertUser(t, pool, "sub@example.com")

	ev := event("evt_1", fmt.Sprint(userID))

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

	// The first insert's id is what the caller applies against; a redelivery has nothing
	// left to do, which is why it returns no id.
	if rowID == 0 {
		t.Fatal("want a row id from the first delivery")
	}
}

// TestRecordKeepsAnUnattributableEvent covers the dashboard TEST event and the anonymous
// identifier. A row we cannot attribute is evidence; a row we refused to write is nothing.
func TestRecordKeepsAnUnattributableEvent(t *testing.T) {
	s, pool := newService(t, nil)
	ctx := context.Background()

	if _, recorded, err := s.Record(ctx, event("evt_test", "$RCAnonymousID:9f8c")); err != nil || !recorded {
		t.Fatalf("recorded=%v err=%v", recorded, err)
	}

	var userID *int64
	if err := pool.QueryRow(ctx,
		`SELECT user_id FROM billing_events WHERE event_id = 'evt_test'`).Scan(&userID); err != nil {
		t.Fatalf("read event: %v", err)
	}
	if userID != nil {
		t.Fatalf("want a NULL user, got %d", *userID)
	}
}

func TestSyncDerivesProUntil(t *testing.T) {
	s, pool := newService(t, subscriberWith("2026-10-01T00:00:00Z"))
	ctx := context.Background()
	userID := insertUser(t, pool, "pro@example.com")

	if err := s.Sync(ctx, fmt.Sprint(userID)); err != nil {
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
	if err := s.Sync(ctx, fmt.Sprint(userID)); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if again := proUntil(t, pool, userID); again == nil || !again.Equal(*got) {
		t.Fatalf("not idempotent: %v then %v", got, again)
	}
}

// TestSyncClearsALapsedSubscription is the refund, the transfer and the cancellation all at
// once: the entitlement is simply no longer there, and no code branch was needed to notice.
func TestSyncClearsALapsedSubscription(t *testing.T) {
	s, pool := newService(t, subscriberWith(""))
	ctx := context.Background()
	userID := insertUser(t, pool, "lapsed@example.com")

	if _, err := pool.Exec(ctx,
		`UPDATE users SET pro_until = now() + interval '30 days' WHERE id = $1`, userID); err != nil {
		t.Fatalf("seed pro_until: %v", err)
	}

	if err := s.Sync(ctx, fmt.Sprint(userID)); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if got := proUntil(t, pool, userID); got != nil {
		t.Fatalf("want pro_until cleared, got %s", got)
	}
}

// TestApplyStampsTheEvent walks the whole webhook path after the response has gone out.
func TestApplyStampsTheEvent(t *testing.T) {
	s, pool := newService(t, subscriberWith("2026-10-01T00:00:00Z"))
	ctx := context.Background()
	userID := insertUser(t, pool, "apply@example.com")

	rowID, _, err := s.Record(ctx, event("evt_apply", fmt.Sprint(userID)))
	if err != nil {
		t.Fatalf("record: %v", err)
	}

	pending, err := s.PendingEvents(ctx, 10)
	if err != nil || len(pending) != 1 {
		t.Fatalf("want 1 unprocessed event, got %d (err %v)", len(pending), err)
	}

	if err := s.Apply(ctx, rowID, fmt.Sprint(userID)); err != nil {
		t.Fatalf("apply: %v", err)
	}

	if pending, err = s.PendingEvents(ctx, 10); err != nil || len(pending) != 0 {
		t.Fatalf("want no unprocessed events, got %d (err %v)", len(pending), err)
	}
	if got := proUntil(t, pool, userID); got == nil {
		t.Fatal("apply did not sync the plan")
	}
}

// TestListSubscribersNearProExpiry is the reconciler's second pass. It must find a
// subscriber whose renewal webhook never arrived, and must not walk users who never
// transacted — asking the provider about one of those would CREATE them.
func TestListSubscribersNearProExpiry(t *testing.T) {
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
	if _, _, err := s.Record(ctx, event("evt_near", fmt.Sprint(subscriber))); err != nil {
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
		t.Fatalf("want user %d, got %d — a user who never transacted must not be a candidate", subscriber, ids[0])
	}
}

// TestDeletingAnAccountErasesItsBillingEvents asserts the cascade the deletion surface
// promises.
func TestDeletingAnAccountErasesItsBillingEvents(t *testing.T) {
	s, pool := newService(t, nil)
	ctx := context.Background()
	userID := insertUser(t, pool, "gone@example.com")

	if _, _, err := s.Record(ctx, event("evt_gone", fmt.Sprint(userID))); err != nil {
		t.Fatalf("record: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	if n := countEvents(t, pool); n != 0 {
		t.Fatalf("want the events gone with the account, got %d", n)
	}
}

// TestARenewalWithNoWebhookIsRepaired is the reconciler's reason to exist, end to end. The
// provider stops retrying a delivery after five attempts over about two and a half hours;
// past that, this path is the only one left.
func TestARenewalWithNoWebhookIsRepaired(t *testing.T) {
	s, pool := newService(t, subscriberWith("2027-01-01T00:00:00Z"))
	ctx := context.Background()
	userID := insertUser(t, pool, "renewed@example.com")

	// A subscriber whose recorded plan is about to lapse: the renewal happened at the
	// provider and we never heard about it.
	if _, _, err := s.Record(ctx, event("evt_old", fmt.Sprint(userID))); err != nil {
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
		if err := s.Sync(ctx, fmt.Sprint(id)); err != nil {
			t.Fatalf("sync %d: %v", id, err)
		}
	}

	got := proUntil(t, pool, userID)
	if got == nil || got.UTC().Format(time.RFC3339) != "2027-01-01T00:00:00Z" {
		t.Fatalf("want the renewal picked up, got %v", got)
	}
}
