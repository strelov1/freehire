//go:build integration

package billing

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

type fakeCrediter struct {
	got []Topup
	err error
}

func (f *fakeCrediter) CreditTopup(_ context.Context, t Topup) error {
	f.got = append(f.got, t)
	return f.err
}

const paidSession = `{"id":"cs_1","mode":"payment","payment_status":"paid","currency":"usd","amount_total":4000,
 "payment_intent":"pi_1","metadata":{"kind":"sponsored_topup","campaign_id":"42","user_id":"7"}}`

// topupEvent is the signal only: the amount in it is deliberately a lie, to prove it is ignored.
func topupEvent(id, typ string) Event {
	return Event{ID: id, Type: typ, Payload: []byte(
		`{"id":"cs_1","object":"checkout.session","amount_total":1,"metadata":{"kind":"sponsored_topup","campaign_id":"1","user_id":"1"}}`)}
}

func applyTopup(t *testing.T, session string, c *fakeCrediter, typ string) (error, bool) {
	t.Helper()
	s, pool := newService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/checkout/sessions/cs_1" {
			t.Errorf("unexpected call %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(session))
	})
	s.SetTopupCrediter(c)
	ev := topupEvent("evt_top", typ)
	rowID, recorded, rerr := s.Record(context.Background(), ev)
	if rerr != nil || !recorded {
		t.Fatalf("record: %v %v", recorded, rerr)
	}
	err := s.Apply(context.Background(), rowID, ev)
	var n int
	if qerr := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM billing_events WHERE id = $1 AND processed_at IS NOT NULL`, rowID).Scan(&n); qerr != nil {
		t.Fatal(qerr)
	}
	return err, n == 1
}

func TestApplyCreditsTopupFromTheReReadSession(t *testing.T) {
	c := &fakeCrediter{}
	err, done := applyTopup(t, paidSession, c, "checkout.session.completed")
	if err != nil || !done {
		t.Fatalf("err=%v processed=%v", err, done)
	}
	want := Topup{SessionID: "cs_1", PaymentIntentID: "pi_1", CampaignID: 42, UserID: 7, AmountCents: 4000}
	if len(c.got) != 1 || c.got[0] != want {
		t.Fatalf("credited %+v, want %+v (the event's own numbers must be ignored)", c.got, want)
	}
}

func TestApplyLeavesAFailedCreditUnprocessed(t *testing.T) {
	c := &fakeCrediter{err: errors.New("db down")}
	err, done := applyTopup(t, paidSession, c, "checkout.session.completed")
	if err == nil || done {
		t.Fatalf("a failed credit must stay for the reconciler: err=%v processed=%v", err, done)
	}
}

func TestApplyDoesNotCreditWhatIsNotAPaidUSDPayment(t *testing.T) {
	for name, body := range map[string]string{
		"unpaid":       strings.Replace(paidSession, `"paid"`, `"unpaid"`, 1),
		"subscription": strings.Replace(paidSession, `"payment"`, `"subscription"`, 1),
		"eur":          strings.Replace(paidSession, `"usd"`, `"eur"`, 1),
	} {
		c := &fakeCrediter{}
		err, done := applyTopup(t, body, c, "checkout.session.completed")
		if err != nil || !done || len(c.got) != 0 {
			t.Errorf("%s: err=%v processed=%v credited=%v", name, err, done, c.got)
		}
	}
}

func TestApplyStampsAnInvalidTopupInsteadOfRetryingForever(t *testing.T) {
	c := &fakeCrediter{err: ErrTopupInvalid}
	if err, done := applyTopup(t, paidSession, c, "checkout.session.completed"); err != nil || !done {
		t.Fatalf("err=%v processed=%v", err, done)
	}
}

func TestApplyIgnoresOtherEventsOfATopupPayment(t *testing.T) {
	c := &fakeCrediter{}
	if err, done := applyTopup(t, paidSession, c, "payment_intent.succeeded"); err != nil || !done || len(c.got) != 0 {
		t.Fatalf("err=%v processed=%v credited=%v", err, done, c.got)
	}
}

func TestApplyWithoutACrediterLeavesTheEventForLater(t *testing.T) {
	s, pool := newService(t, func(http.ResponseWriter, *http.Request) {})
	ev := topupEvent("evt_x", "checkout.session.completed")
	rowID, _, err := s.Record(context.Background(), ev)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Apply(context.Background(), rowID, ev); err == nil {
		t.Fatal("want an error")
	}
	var n int
	_ = pool.QueryRow(context.Background(), `SELECT count(*) FROM billing_events WHERE processed_at IS NOT NULL`).Scan(&n)
	if n != 0 {
		t.Fatal("must stay unprocessed")
	}
}
