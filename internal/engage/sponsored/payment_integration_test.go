//go:build integration

package sponsored

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/identity/billing"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

type fakeRefunder struct {
	mu   sync.Mutex
	keys []string // idempotency keys, in call order
	pis  []string
	fail bool
}

func (f *fakeRefunder) RefundPaymentIntent(_ context.Context, pi, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.keys, f.pis = append(f.keys, key), append(f.pis, pi)
	if f.fail {
		return errors.New("stripe down")
	}
	return nil
}

type campaignRow struct {
	Status   Status
	Budget   int64
	Reviewed bool
	Reason   string
}

func readCampaign(t *testing.T, pool *pgxpool.Pool, id int64) campaignRow {
	t.Helper()
	var r campaignRow
	var reason *string
	if err := pool.QueryRow(context.Background(),
		`SELECT status, budget_cents, reviewed_at IS NOT NULL AND reviewed_by IS NOT NULL, reject_reason FROM sponsored_campaigns WHERE id = $1`, id).
		Scan(&r.Status, &r.Budget, &r.Reviewed, &reason); err != nil {
		t.Fatal(err)
	}
	if reason != nil {
		r.Reason = *reason
	}
	return r
}

func ownerOf(t *testing.T, pool *pgxpool.Pool, campaignID int64) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(), `SELECT owner_user_id FROM sponsored_campaigns WHERE id = $1`, campaignID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func topup(t *testing.T, pool *pgxpool.Pool, id int64, session string, cents int64) billing.Topup {
	t.Helper()
	return billing.Topup{SessionID: session, PaymentIntentID: "pi_" + session, CampaignID: id, UserID: ownerOf(t, pool, id), AmountCents: cents}
}

func refundedCount(t *testing.T, pool *pgxpool.Pool, id int64) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM sponsored_topups WHERE campaign_id = $1 AND refunded_at IS NOT NULL`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestCreditTopupMovesStatusAndIsIdempotent(t *testing.T) {
	cases := []struct{ from, want Status }{
		{StatusDraft, StatusPendingReview},
		{StatusPendingPayment, StatusPendingReview},
		{StatusPendingReview, StatusPendingReview},
		{StatusActive, StatusActive},
		{StatusPaused, StatusPaused},
	}
	for _, tc := range cases {
		t.Run(string(tc.from), func(t *testing.T) {
			pool := testdb.Pool(t)
			id, jobID := seedCampaign(t, pool, 50, 0, tc.from)
			svc := New(pool)
			tp := topup(t, pool, id, "cs_a", 4000)
			for range 2 { // the replay credits nothing more
				if err := svc.CreditTopup(context.Background(), tp); err != nil {
					t.Fatal(err)
				}
			}
			got := readCampaign(t, pool, id)
			if got.Status != tc.want || got.Budget != 4000 {
				t.Fatalf("got %+v, want status %s budget 4000", got, tc.want)
			}
			if outboxHas(t, pool, jobID) != (tc.from != tc.want) {
				t.Errorf("outbox for %s -> %s: %v", tc.from, tc.want, outboxHas(t, pool, jobID))
			}
		})
	}
}

func TestCreditTopupReactivatesExhausted(t *testing.T) {
	pool := testdb.Pool(t)
	id, jobID := seedCampaign(t, pool, 50, 100, StatusActive)
	if _, err := pool.Exec(context.Background(), `UPDATE sponsored_campaigns SET status='exhausted', spent_cents=100, reviewed_at=now() WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if err := New(pool).CreditTopup(context.Background(), topup(t, pool, id, "cs_b", 3000)); err != nil {
		t.Fatal(err)
	}
	if got := readCampaign(t, pool, id); got.Status != StatusActive || got.Budget != 3100 {
		t.Fatalf("got %+v", got)
	}
	if !outboxHas(t, pool, jobID) {
		t.Error("an exhausted campaign coming back must be re-indexed")
	}
}

func TestCreditTopupRefusesAForeignOrMissingCampaign(t *testing.T) {
	pool := testdb.Pool(t)
	id, _ := seedCampaign(t, pool, 50, 0, StatusDraft)
	svc := New(pool)
	tp := topup(t, pool, id, "cs_c", 4000)
	tp.UserID += 999
	if err := svc.CreditTopup(context.Background(), tp); !errors.Is(err, billing.ErrTopupInvalid) {
		t.Errorf("foreign owner: %v", err)
	}
	tp = topup(t, pool, id, "cs_c", 4000)
	tp.CampaignID = id + 999
	if err := svc.CreditTopup(context.Background(), tp); !errors.Is(err, billing.ErrTopupInvalid) {
		t.Errorf("missing campaign: %v", err)
	}
	if got := readCampaign(t, pool, id); got.Budget != 0 {
		t.Errorf("nothing may be credited: %+v", got)
	}
}

// Money for a campaign that can never serve goes back: recorded, NOT added to the budget, refunded.
func TestCreditTopupOnADeadCampaignIsRefunded(t *testing.T) {
	for _, dead := range []Status{StatusRejected, StatusEnded} {
		t.Run(string(dead), func(t *testing.T) {
			pool := testdb.Pool(t)
			id, _ := seedCampaign(t, pool, 50, 0, dead)
			rf := &fakeRefunder{}
			svc := New(pool).WithRefunder(rf)
			if err := svc.CreditTopup(context.Background(), topup(t, pool, id, "cs_d", 4000)); err != nil {
				t.Fatal(err)
			}
			if got := readCampaign(t, pool, id); got.Status != dead || got.Budget != 0 {
				t.Errorf("a dead campaign keeps its status and budget: %+v", got)
			}
			if len(rf.pis) != 1 || rf.pis[0] != "pi_cs_d" || refundedCount(t, pool, id) != 1 {
				t.Errorf("refund calls %v, stamped %d", rf.pis, refundedCount(t, pool, id))
			}
		})
	}
}

func TestCreditTopupOnADeadCampaignRefundRetriesLater(t *testing.T) {
	pool := testdb.Pool(t)
	id, _ := seedCampaign(t, pool, 50, 0, StatusEnded)
	rf := &fakeRefunder{fail: true}
	svc := New(pool).WithRefunder(rf)
	// The credit itself succeeded (the event may be stamped): only the refund is outstanding.
	if err := svc.CreditTopup(context.Background(), topup(t, pool, id, "cs_e", 4000)); err != nil {
		t.Fatal(err)
	}
	if refundedCount(t, pool, id) != 0 {
		t.Fatal("not stamped while Stripe failed")
	}
	rf.fail = false
	if n, err := svc.RefundPending(context.Background(), 10); err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if refundedCount(t, pool, id) != 1 {
		t.Fatal("stamped after Stripe confirmed")
	}
}

func TestRefundPendingLeavesLegitTopupsOfAnEndedCampaign(t *testing.T) {
	pool := testdb.Pool(t)
	id, _ := seedCampaign(t, pool, 50, 0, StatusActive)
	rf := &fakeRefunder{}
	svc := New(pool).WithRefunder(rf)
	if err := svc.CreditTopup(context.Background(), topup(t, pool, id, "cs_f", 4000)); err != nil {
		t.Fatal(err)
	}
	if err := svc.Transition(context.Background(), id, StatusEnded); err != nil {
		t.Fatal(err)
	}
	if n, err := svc.RefundPending(context.Background(), 10); err != nil || n != 0 || len(rf.pis) != 0 {
		t.Fatalf("unused budget of an ended campaign stays recorded: n=%d err=%v calls=%v", n, err, rf.pis)
	}
}

func TestApprove(t *testing.T) {
	pool := testdb.Pool(t)
	id, jobID := seedCampaign(t, pool, 50, 4000, StatusPendingReview)
	svc := New(pool)
	mod := moderator(t, pool)
	if err := svc.Approve(context.Background(), id, mod); err != nil {
		t.Fatal(err)
	}
	if got := readCampaign(t, pool, id); got.Status != StatusActive || !got.Reviewed {
		t.Fatalf("%+v", got)
	}
	if !outboxHas(t, pool, jobID) {
		t.Error("approval must reach the index")
	}
	if err := svc.Approve(context.Background(), id, mod); !errors.Is(err, ErrBadTransition) {
		t.Errorf("approving twice: %v", err)
	}
}

func moderator(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(), `INSERT INTO users (email) VALUES ('mod@example.test') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestRejectRefundsEveryTopupAndSurvivesAFailedRefund(t *testing.T) {
	pool := testdb.Pool(t)
	id, jobID := seedCampaign(t, pool, 50, 0, StatusDraft)
	rf := &fakeRefunder{}
	svc := New(pool).WithRefunder(rf)
	ctx := context.Background()
	for i, s := range []string{"cs_1", "cs_2"} {
		if err := svc.CreditTopup(ctx, topup(t, pool, id, s, int64(3000+i))); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `DELETE FROM search_outbox`); err != nil {
		t.Fatal(err)
	}
	mod := moderator(t, pool)

	rf.fail = true
	err := svc.Reject(ctx, id, mod, "not a real job")
	if err == nil {
		t.Fatal("a failed refund must be reported")
	}
	got := readCampaign(t, pool, id)
	if got.Status != StatusRejected || !got.Reviewed || got.Reason != "not a real job" {
		t.Fatalf("the status change must commit even when a refund fails: %+v", got)
	}
	if !outboxHas(t, pool, jobID) || refundedCount(t, pool, id) != 0 {
		t.Fatalf("outbox %v, refunded %d", outboxHas(t, pool, jobID), refundedCount(t, pool, id))
	}

	rf.fail, rf.keys, rf.pis = false, nil, nil
	if n, err := svc.RefundPending(ctx, 10); err != nil || n != 2 {
		t.Fatalf("retry: n=%d err=%v", n, err)
	}
	if refundedCount(t, pool, id) != 2 {
		t.Fatal("both stamped")
	}
	for _, k := range rf.keys {
		if len(k) < len("sponsored-refund-") || k[:len("sponsored-refund-")] != "sponsored-refund-" {
			t.Errorf("idempotency key %q", k)
		}
	}
	if n, _ := svc.RefundPending(ctx, 10); n != 0 {
		t.Errorf("a refunded top-up is never refunded again: %d", n)
	}
}

func TestRejectOnlyFromPendingReview(t *testing.T) {
	pool := testdb.Pool(t)
	id, _ := seedCampaign(t, pool, 50, 0, StatusActive)
	if err := New(pool).WithRefunder(&fakeRefunder{}).Reject(context.Background(), id, moderator(t, pool), "x"); !errors.Is(err, ErrBadTransition) {
		t.Fatalf("got %v", err)
	}
}
