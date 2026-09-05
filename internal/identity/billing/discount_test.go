package billing

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestCreateCouponSendsAOnceOffPercentage(t *testing.T) {
	var gotPath, gotBody, gotKey string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 2048)
		n, _ := r.Body.Read(buf)
		gotPath, gotBody, gotKey = r.URL.Path, string(buf[:n]), r.Header.Get("Idempotency-Key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"co_abc"}`))
	})

	id, err := c.createCoupon(context.Background(), 50, "Invite", "key-1")
	if err != nil {
		t.Fatalf("createCoupon: %v", err)
	}
	if id != "co_abc" {
		t.Fatalf("coupon id = %q, want co_abc", id)
	}
	if gotPath != "/coupons" {
		t.Fatalf("path = %q, want /coupons", gotPath)
	}
	if !strings.Contains(gotBody, "duration=once") {
		t.Fatalf("body = %q, want duration=once — every discount here applies to the first "+
			"invoice and must not recur", gotBody)
	}
	if !strings.Contains(gotBody, "percent_off=50") {
		t.Fatalf("body = %q, want percent_off=50", gotBody)
	}
	if !strings.Contains(gotBody, "max_redemptions=1") {
		t.Fatalf("body = %q, want max_redemptions=1 — `duration` bounds how many invoices of "+
			"ONE subscription a coupon touches, not how many subscriptions may claim it, so "+
			"without this a buyer who cancels and resubscribes is discounted again", gotBody)
	}
	if gotKey != "key-1" {
		t.Fatalf("Idempotency-Key = %q, want key-1 — without it a retried checkout mints a "+
			"second coupon for one purchase", gotKey)
	}
}

func TestCheckoutSessionCarriesNoDiscountWhenThereIsNone(t *testing.T) {
	var gotBody string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"url":"https://pay.test/s"}`))
	})

	if _, err := c.createCheckoutSession(context.Background(), 7, "a@b.test",
		"price_1", "https://x.test", "https://x.test", "", ""); err != nil {
		t.Fatalf("createCheckoutSession: %v", err)
	}
	if strings.Contains(gotBody, "discounts") {
		t.Fatalf("body = %q, want no discounts field — an account with no code and no invite "+
			"must produce exactly the request checkout makes today", gotBody)
	}
}

func TestCheckoutSessionAttachesTheCoupon(t *testing.T) {
	var gotBody string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"url":"https://pay.test/s"}`))
	})

	if _, err := c.createCheckoutSession(context.Background(), 7, "a@b.test",
		"price_1", "https://x.test", "https://x.test", "", "co_abc"); err != nil {
		t.Fatalf("createCheckoutSession: %v", err)
	}
	if !strings.Contains(gotBody, "discounts%5B0%5D%5Bcoupon%5D=co_abc") {
		t.Fatalf("body = %q, want discounts[0][coupon]=co_abc", gotBody)
	}
}

func TestCreditCustomerBalanceSendsANegativeAmount(t *testing.T) {
	var gotPath, gotBody, gotKey string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 2048)
		n, _ := r.Body.Read(buf)
		gotPath, gotBody, gotKey = r.URL.Path, string(buf[:n]), r.Header.Get("Idempotency-Key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"cbtxn_1"}`))
	})

	err := c.creditCustomerBalance(context.Background(), "cus_9", 250, "usd", "Invite reward", "invite_reward_7")
	if err != nil {
		t.Fatalf("creditCustomerBalance: %v", err)
	}
	if gotPath != "/customers/cus_9/balance_transactions" {
		t.Fatalf("path = %q", gotPath)
	}
	if !strings.Contains(gotBody, "amount=-250") {
		t.Fatalf("body = %q, want amount=-250 — a positive amount is a DEBT the customer "+
			"owes us, which is the opposite of a reward", gotBody)
	}
	if gotKey != "invite_reward_7" {
		t.Fatalf("Idempotency-Key = %q, want invite_reward_7", gotKey)
	}
}

func TestHasCollectedAtLeastReadsAmountPaid(t *testing.T) {
	body := `{"data":[{"amount_paid":0,"amount_due":500,"status":"open"},
	                  {"amount_paid":500,"amount_due":500,"status":"paid"}]}`
	c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})

	collected, err := c.hasCollectedAtLeast(context.Background(), "cus_9", 250)
	if err != nil {
		t.Fatalf("hasCollectedAtLeast: %v", err)
	}
	if !collected {
		t.Fatal("an invoice that collected 500 was not recognised")
	}
}

func TestHasCollectedAtLeastIgnoresAnInvoiceThatOnlyOwes(t *testing.T) {
	body := `{"data":[{"amount_paid":0,"amount_due":500,"status":"open"}]}`
	c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})

	collected, err := c.hasCollectedAtLeast(context.Background(), "cus_9", 250)
	if err != nil {
		t.Fatalf("hasCollectedAtLeast: %v", err)
	}
	if collected {
		t.Fatal("an unpaid invoice was read as a payment — an active subscription that " +
			"collected nothing is a trial or a total discount, and rewarding it turns the " +
			"discount into a way to mint credit")
	}
}

func TestHasCollectedAtLeastIsAThresholdAndNotATestForAnyMoney(t *testing.T) {
	// A 90% code against a 500-cent price. Money moved, and it is not enough.
	body := `{"data":[{"amount_paid":50,"amount_due":50,"status":"paid"}]}`
	c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})

	collected, err := c.hasCollectedAtLeast(context.Background(), "cus_9", 250)
	if err != nil {
		t.Fatalf("hasCollectedAtLeast: %v", err)
	}
	if collected {
		t.Fatal("a 50-cent sale satisfied a 250-cent threshold — a referral that pays out " +
			"more than it brought in is a hole, not a growth channel")
	}
}

func TestHasCollectedAtLeastRefusesToTreatZeroAsNoThreshold(t *testing.T) {
	body := `{"data":[{"amount_paid":0,"amount_due":500,"status":"open"}]}`
	c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})

	collected, err := c.hasCollectedAtLeast(context.Background(), "cus_9", 0)
	if err != nil {
		t.Fatalf("hasCollectedAtLeast: %v", err)
	}
	if collected {
		t.Fatal("a threshold of zero matched an unpaid invoice — `amount_paid >= 0` is true " +
			"of every invoice ever issued, which is the opposite of the question")
	}
}

func TestHasCollectedAtLeastSurfacesAFailure(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	if _, err := c.hasCollectedAtLeast(context.Background(), "cus_9", 250); err == nil {
		t.Fatal("a provider failure was reported as 'collected nothing' — the receipt list " +
			"may swallow its errors because a missing receipt costs nothing, but a reward " +
			"denied by a network blip is money somebody earned and never sees")
	}
}
