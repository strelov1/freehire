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

func TestHasCollectedPaymentReadsAmountPaid(t *testing.T) {
	body := `{"data":[{"amount_paid":0,"amount_due":500,"status":"open"},
	                  {"amount_paid":500,"amount_due":500,"status":"paid"}]}`
	c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})

	collected, err := c.hasCollectedPayment(context.Background(), "cus_9")
	if err != nil {
		t.Fatalf("hasCollectedPayment: %v", err)
	}
	if !collected {
		t.Fatal("an invoice that collected 500 was not recognised")
	}
}

func TestHasCollectedPaymentIgnoresAnInvoiceThatOnlyOwes(t *testing.T) {
	body := `{"data":[{"amount_paid":0,"amount_due":500,"status":"open"}]}`
	c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})

	collected, err := c.hasCollectedPayment(context.Background(), "cus_9")
	if err != nil {
		t.Fatalf("hasCollectedPayment: %v", err)
	}
	if collected {
		t.Fatal("an unpaid invoice was read as a payment — an active subscription that " +
			"collected nothing is a trial or a total discount, and rewarding it turns the " +
			"discount into a way to mint credit")
	}
}

func TestHasCollectedPaymentSurfacesAFailure(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	if _, err := c.hasCollectedPayment(context.Background(), "cus_9"); err == nil {
		t.Fatal("a provider failure was reported as 'collected nothing' — the receipt list " +
			"may swallow its errors because a missing receipt costs nothing, but a reward " +
			"denied by a network blip is money somebody earned and never sees")
	}
}
