//go:build integration

package changehook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

type received struct {
	delivery string
	event    Event
}

// endpoint is a receiver that verifies every signature and answers with a status the test sets.
type endpoint struct {
	t      *testing.T
	secret string
	mu     sync.Mutex
	status int
	got    []received
}

func (e *endpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	if err := Verify(e.secret, r.Header.Get(HeaderTimestamp), r.Header.Get(HeaderSignature), body, time.Now(), DefaultTolerance); err != nil {
		e.t.Errorf("receiver: %v", err)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	var ev Event
	if err := json.Unmarshal(body, &ev); err != nil {
		e.t.Errorf("receiver: body: %v", err)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.got = append(e.got, received{r.Header.Get(HeaderDelivery), ev})
	w.WriteHeader(e.status)
}

func (e *endpoint) take() []received {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := e.got
	e.got = nil
	return out
}

func exec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func seedJob(t *testing.T, pool *pgxpool.Pool, slug, company string) {
	t.Helper()
	exec(t, pool, `INSERT INTO jobs (source, external_id, url, title, public_slug, company_slug)
		VALUES ('test', $1, 'https://e.test/'||$1, 'Engineer', $1, $2)`, slug, company)
}

func TestDeliveryEndToEnd(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)

	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, email_verified, api_until_stripe)
		VALUES ('hook@example.test', true, now() + interval '30 days') RETURNING id`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	ep := &endpoint{t: t, secret: "whsec_test", status: http.StatusOK}
	srv := httptest.NewServer(ep)
	defer srv.Close()
	w := NewWorker(q, newSender(srv.Client()))

	// A change made just before the subscription can be delivered once: the start is the
	// oldest open transaction of the cluster, which may predate it. Receivers dedupe on seq.
	seedJob(t, pool, "before", "acme")

	sub, err := q.CreateChangeWebhook(ctx, db.CreateChangeWebhookParams{
		UserID: userID, URL: srv.URL, Secret: ep.secret, CompanySlugs: []string{"acme"}, Kinds: []string{"opened", "closed", "reopened"},
	})
	if err != nil {
		t.Fatal(err)
	}
	state := func() db.ChangeWebhook {
		row, err := q.GetChangeWebhook(ctx, db.GetChangeWebhookParams{ID: sub.ID, UserID: userID})
		if err != nil {
			t.Fatal(err)
		}
		return row
	}
	run := func() Stats {
		// A failing customer endpoint must not surface as an error (it would fail the unit):
		// the "failure retries" case below runs through here with the endpoint answering 500.
		st, err := w.Run(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return st
	}
	// The feed trails the oldest open transaction of the whole Postgres cluster, which on a
	// shared development database is another test's. Run again until the wanted thing shows.
	var total Stats
	until := func(done func() bool) {
		t.Helper()
		for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
			st := run()
			total.Changes += st.Changes
			total.Failed += st.Failed
			total.Disabled += st.Disabled
			if done() {
				return
			}
		}
		t.Fatal("timed out waiting for the feed")
	}
	due := func() { exec(t, pool, `UPDATE change_webhooks SET next_attempt_at = now() WHERE id = $1`, sub.ID) }
	start := state()

	seedJob(t, pool, "job-a", "acme")
	seedJob(t, pool, "job-x", "other") // filtered out, but the position still moves past it
	exec(t, pool, `UPDATE jobs SET closed_at = now(), closed_reason = 'expired' WHERE public_slug = 'job-a'`)

	var got []received
	collect := func(n int) func() bool {
		return func() bool { got = append(got, ep.take()...); return len(got) >= n }
	}

	t.Run("a batch of the wanted changes, signed, in order, and the cursor advances", func(t *testing.T) {
		until(collect(1))
		if len(got) != 1 || total.Failed != 0 {
			t.Fatalf("deliveries = %+v totals = %+v, want one POST", got, total)
		}
		ev := got[0].event
		if c := ev.Changes; len(c) == 3 && c[0].PublicSlug == "before" {
			ev.Changes = c[1:]
		}
		if ev.Type != EventChanges || ev.ID != got[0].delivery || len(ev.Changes) != 2 ||
			ev.Changes[0].Kind != "opened" || ev.Changes[1].Kind != "closed" || ev.Changes[1].ClosedReason != "expired" ||
			ev.Changes[0].Seq >= ev.Changes[1].Seq || ev.Changes[0].PublicSlug != "job-a" {
			t.Fatalf("event = %+v", ev)
		}
		after := state()
		if after.CursorID <= start.CursorID || after.ConsecutiveFailures != 0 || !after.LastSuccessAt.Valid {
			t.Fatalf("state after = %+v (start %+v)", after, start)
		}
	})
	t.Run("nothing new is nothing sent", func(t *testing.T) {
		run()
		if n := len(ep.take()); n != 0 {
			t.Fatalf("a caught-up subscription was sent %d delivery(ies)", n)
		}
	})
	t.Run("failure retries the same batch with the same id, then the cursor advances", func(t *testing.T) {
		seedJob(t, pool, "job-b", "acme")
		ep.status = http.StatusInternalServerError
		before := state()
		got = nil
		total = Stats{}
		until(func() bool { return total.Failed == 1 })
		got = append(got, ep.take()...)
		s := state()
		if len(got) != 1 || s.ConsecutiveFailures != 1 || s.CursorID != before.CursorID || !s.NextAttemptAt.Time.After(time.Now().Add(10*time.Second)) || s.LastError == "" {
			t.Fatalf("after failure: deliveries %+v state %+v", got, s)
		}
		// Not due: the backoff holds it back.
		run()
		if n := len(ep.take()); n != 0 {
			t.Fatalf("retried before the backoff elapsed (%d)", n)
		}
		failed := got[0]
		ep.status = http.StatusOK
		due()
		got = nil
		until(collect(1))
		if len(got) != 1 || got[0].delivery != failed.delivery || got[0].event.Changes[0].PublicSlug != "job-b" {
			t.Fatalf("retry = %+v, first = %+v", got, failed)
		}
		if s := state(); s.ConsecutiveFailures != 0 || s.CursorID <= before.CursorID {
			t.Fatalf("after retry = %+v", s)
		}
	})
	t.Run("a lapsed API plan pauses delivery", func(t *testing.T) {
		seedJob(t, pool, "job-c", "acme")
		exec(t, pool, `UPDATE users SET api_until_stripe = now() - interval '1 day' WHERE id = $1`, userID)
		due()
		for range 3 {
			run()
			time.Sleep(100 * time.Millisecond)
		}
		if n := len(ep.take()); n != 0 {
			t.Fatalf("delivered %d time(s) for an account without an API plan", n)
		}
		exec(t, pool, `UPDATE users SET api_until_stripe = now() + interval '1 day' WHERE id = $1`, userID)
		got = nil
		until(collect(1))
		if len(got) != 1 || got[0].event.Changes[0].PublicSlug != "job-c" {
			t.Fatalf("after renewal = %+v, want job-c", got)
		}
	})
	t.Run("410 disables at once", func(t *testing.T) {
		seedJob(t, pool, "job-d", "acme")
		ep.status = http.StatusGone
		total = Stats{}
		until(func() bool { return total.Disabled == 1 })
		ep.take()
		if s := state(); !s.DisabledAt.Valid || s.DisabledReason == "" {
			t.Fatalf("not disabled: %+v", s)
		}
		due()
		run()
		if n := len(ep.take()); n != 0 {
			t.Fatalf("a disabled subscription was delivered (%d)", n)
		}
	})
	t.Run("sustained failure disables", func(t *testing.T) {
		exec(t, pool, `UPDATE change_webhooks SET disabled_at = NULL, disabled_reason = '', consecutive_failures = $2 WHERE id = $1`, sub.ID, maxFailures-1)
		ep.status = http.StatusBadGateway
		due()
		total = Stats{}
		until(func() bool { return total.Disabled == 1 })
	})
}
