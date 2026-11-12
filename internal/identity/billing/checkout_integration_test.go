//go:build integration

// CheckoutURL end to end, against a real Postgres and a stubbed provider: the one piece of
// the duplicate-subscription fix that the pure decideCheckoutTarget/updateSubscriptionPrice
// unit tests cannot reach, because CheckoutURL itself needs *db.Queries (GetStripeCustomerID)
// to even decide whether a customer is known. Run with:
// go test -tags=integration ./internal/identity/billing/
package billing

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// checkoutRouter dispatches the provider stub paths CheckoutURL can reach, recording which
// ones were hit and what they carried — the assertion surface every test below reads.
type checkoutRouter struct {
	subscriptions http.HandlerFunc // GET /subscriptions

	checkoutCalled bool
	checkoutForm   string
	couponCalled   bool
	spent          map[string]bool   // coupon ids GET /coupons/{id} reports as no longer valid
	couponKeys     []string          // Idempotency-Key of every POST /coupons, in order
	couponByKey    map[string]string // the provider's idempotency: one coupon per key
	updateCalled   bool
	updatePath     string
	updateForm     string
}

func (r *checkoutRouter) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		switch {
		case req.URL.Path == "/subscriptions":
			r.subscriptions(w, req)
		case req.URL.Path == "/checkout/sessions":
			r.checkoutCalled = true
			_ = req.ParseForm()
			r.checkoutForm = req.Form.Encode()
			_, _ = w.Write([]byte(`{"url":"https://checkout.stripe.com/c/pay/cs_test_123"}`))
		case req.URL.Path == "/coupons":
			r.couponCalled = true
			key := req.Header.Get("Idempotency-Key")
			r.couponKeys = append(r.couponKeys, key)
			if r.couponByKey == nil {
				r.couponByKey = map[string]string{}
			}
			id, ok := r.couponByKey[key]
			if !ok || key == "" {
				id = fmt.Sprintf("co_test%d", len(r.couponByKey)+1)
				r.couponByKey[key] = id
			}
			_, _ = fmt.Fprintf(w, `{"id":%q}`, id)
		case strings.HasPrefix(req.URL.Path, "/coupons/"):
			_, _ = fmt.Fprintf(w, `{"valid":%t}`, !r.spent[strings.TrimPrefix(req.URL.Path, "/coupons/")])
		case strings.HasPrefix(req.URL.Path, "/subscriptions/"):
			r.updateCalled = true
			r.updatePath = req.URL.Path
			_ = req.ParseForm()
			r.updateForm = req.Form.Encode()
			_, _ = w.Write([]byte(`{"id":"sub_1"}`))
		default:
			_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
		}
	}
}

// TestCheckoutURLUpdatesAnExistingSubscriptionInPlace is the fix's central guarantee: a
// customer who already has an active Pro subscription and asks for Ultra gets that SAME
// subscription changed in place, never a second Checkout Session. Against the pre-fix code
// this assertion fails — createCheckoutSession is always called regardless of what the
// customer already has.
func TestCheckoutURLUpdatesAnExistingSubscriptionInPlace(t *testing.T) {
	router := &checkoutRouter{
		subscriptions: func(w http.ResponseWriter, _ *http.Request) {
			until := time.Now().UTC().Add(30 * 24 * time.Hour)
			_, _ = fmt.Fprintf(w, `{"data":[{"id":"sub_1","status":"active",`+
				`"items":{"data":[{"id":"si_1","current_period_end":%d,"price":{"id":%q}}]}}]}`,
				until.Unix(), proPrice)
		},
	}
	s, pool := newTieredService(t, router.handler())
	ctx := context.Background()

	userID := insertUser(t, pool, "upgrader-inplace@example.com")
	bindCustomer(t, pool, userID, "cus_inplace")

	// A discount is offered but must never be applied to an in-place update — design.md's
	// own stated Non-Goal — and the coupon endpoint must never even be called.
	offered := Discount{PercentOff: 20, Label: "invite", Key: "k1"}
	url, applied, err := s.CheckoutURL(ctx, userID, ultraPrice, offered)
	if err != nil {
		t.Fatalf("CheckoutURL: %v", err)
	}

	if router.checkoutCalled {
		t.Fatal("a new Checkout Session was opened for a customer who already has an entitling subscription")
	}
	if router.couponCalled {
		t.Fatal("a coupon was minted for an in-place update, which never applies a discount")
	}
	if !router.updateCalled {
		t.Fatal("the existing subscription was never updated")
	}
	if router.updatePath != "/subscriptions/sub_1" {
		t.Fatalf("updated path = %q, want /subscriptions/sub_1", router.updatePath)
	}
	if !strings.Contains(router.updateForm, "items%5B0%5D%5Bid%5D=si_1") ||
		!strings.Contains(router.updateForm, "items%5B0%5D%5Bprice%5D="+ultraPrice) {
		t.Fatalf("update form = %q, want item si_1 replaced with %s", router.updateForm, ultraPrice)
	}
	// SuccessURL, not ReturnURL: an upgrade applied in place IS a purchase — it simply
	// never shows a checkout page. Sending the browser back without the marker left every
	// Pro-to-Ultra sale looking abandoned in the funnel, since checkout_start had already
	// been recorded and nothing ever answered it.
	if url != s.cfg.SuccessURL() {
		t.Fatalf("url = %q, want the success URL %q", url, s.cfg.SuccessURL())
	}
	if applied != (Discount{}) {
		t.Fatalf("applied discount = %+v, want none — an in-place update never applies one", applied)
	}
}

// TestCheckoutURLOpensACheckoutSessionForAKnownCustomerWithNoSubscription covers the
// fallthrough: a bound Stripe customer (they have transacted before — a referral credit, a
// lapsed subscription) who currently holds nothing entitling must still reach an ordinary
// checkout, carrying whatever discount they were offered.
func TestCheckoutURLOpensACheckoutSessionForAKnownCustomerWithNoSubscription(t *testing.T) {
	router := &checkoutRouter{
		subscriptions: func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"data":[]}`))
		},
	}
	s, pool := newTieredService(t, router.handler())
	ctx := context.Background()

	userID := insertUser(t, pool, "lapsed@example.com")
	bindCustomer(t, pool, userID, "cus_lapsed")

	offered := Discount{PercentOff: 20, Label: "invite", Key: "k2"}
	url, applied, err := s.CheckoutURL(ctx, userID, proPrice, offered)
	if err != nil {
		t.Fatalf("CheckoutURL: %v", err)
	}

	if router.updateCalled {
		t.Fatal("nothing was updated in place — there is no entitling subscription to update")
	}
	if !router.checkoutCalled {
		t.Fatal("no Checkout Session was opened for a customer with no entitling subscription")
	}
	if !router.couponCalled {
		t.Fatal("the offered discount was never minted as a coupon")
	}
	if url != "https://checkout.stripe.com/c/pay/cs_test_123" {
		t.Fatalf("url = %q, want the Checkout Session URL", url)
	}
	if applied != offered {
		t.Fatalf("applied discount = %+v, want the offered one %+v", applied, offered)
	}
}

// TestCheckoutURLFailsRatherThanTreatAFailedBindingReadAsUnbound guards a sibling of the
// fix's central line (service.go's read of client.subscriberState): a failure reading
// GetStripeCustomerID itself must not be treated as "no customer", which would open a
// checkout that creates a second Stripe customer and a second subscription for someone who
// already has both — reproducing the exact bug this method exists to prevent. A nonexistent
// user id is what makes the underlying `:one` query fail deterministically, without needing
// to fail the database connection itself.
func TestCheckoutURLFailsRatherThanTreatAFailedBindingReadAsUnbound(t *testing.T) {
	router := &checkoutRouter{
		subscriptions: func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"data":[]}`))
		},
	}
	s, _ := newTieredService(t, router.handler())
	ctx := context.Background()

	const nonexistentUserID = -1
	if _, _, err := s.CheckoutURL(ctx, nonexistentUserID, proPrice, Discount{}); err == nil {
		t.Fatal("want an error when the customer binding cannot be read, got nil")
	}

	if router.checkoutCalled {
		t.Fatal("a checkout was opened despite the binding read failing — this is the bug this test guards")
	}
}

// TestCheckoutURLRefusesToGuessOnAnAmbiguousSubscription: a subscription already carrying an
// item of each tier (the shape billedSubscription's own comment describes an upgrade through
// the provider's portal leaving behind) must never have an item silently replaced by guess.
func TestCheckoutURLRefusesToGuessOnAnAmbiguousSubscription(t *testing.T) {
	router := &checkoutRouter{
		subscriptions: func(w http.ResponseWriter, _ *http.Request) {
			until := time.Now().UTC().Add(30 * 24 * time.Hour)
			_, _ = fmt.Fprintf(w, `{"data":[{"id":"sub_both","status":"active","items":{"data":[`+
				`{"id":"si_a","current_period_end":%d,"price":{"id":%q}},`+
				`{"id":"si_b","current_period_end":%d,"price":{"id":%q}}]}}]}`,
				until.Unix(), proPrice, until.Unix(), ultraPrice)
		},
	}
	s, pool := newTieredService(t, router.handler())
	ctx := context.Background()

	userID := insertUser(t, pool, "ambiguous@example.com")
	bindCustomer(t, pool, userID, "cus_ambiguous")

	if _, _, err := s.CheckoutURL(ctx, userID, "price_pro_annual", Discount{}); err == nil {
		t.Fatal("want an error refusing to guess which item to change, got nil")
	}

	if router.checkoutCalled {
		t.Fatal("a second Checkout Session was opened instead of refusing")
	}
	if router.updateCalled {
		t.Fatal("an item was updated despite the ambiguity")
	}
}

// TestCheckoutURLAppliesNoDiscountToTheAPIProduct: promo codes and invite discounts are a
// consumer offer for Pro and Ultra. The API product is sold to businesses at a flat price, so
// an account's held discount must not reach its checkout — no coupon is minted, and the
// applied discount reported back is none.
func TestCheckoutURLAppliesNoDiscountToTheAPIProduct(t *testing.T) {
	router := &checkoutRouter{
		subscriptions: func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"data":[]}`))
		},
	}
	s, pool := newTieredService(t, router.handler())
	s.cfg.APIPrices = []string{apiPrice}
	ctx := context.Background()

	userID := insertUser(t, pool, "api-with-promo@example.com")
	bindCustomer(t, pool, userID, "cus_api_promo")

	url, applied, err := s.CheckoutURL(ctx, userID, apiPrice, Discount{PercentOff: 90, Label: "promo", Key: "k9"})
	if err != nil {
		t.Fatalf("CheckoutURL: %v", err)
	}
	if !router.checkoutCalled || url == "" {
		t.Fatal("no Checkout Session was opened for the API price")
	}
	if router.couponCalled {
		t.Fatal("a coupon was minted for an API purchase — the account's discount reached the API product")
	}
	if applied != (Discount{}) {
		t.Fatalf("applied discount = %+v, want none", applied)
	}
}

// A declined card spends the coupon's one redemption at the provider. The checkout must carry
// a replacement, not the spent coupon the idempotent create replays — which made every later
// checkout fail outright.
func TestCheckoutURLReplacesACouponADeclinedAttemptSpent(t *testing.T) {
	router := &checkoutRouter{spent: map[string]bool{"co_test1": true}}
	s, pool := newTieredService(t, router.handler())
	userID := insertUser(t, pool, "declined-once@example.com")

	offer := Discount{PercentOff: 90, Label: "promo", Key: "promo_k"}
	_, applied, err := s.CheckoutURL(context.Background(), userID, "", offer)
	if err != nil {
		t.Fatalf("CheckoutURL: %v", err)
	}
	if want := []string{"promo_k", "promo_k_after_co_test1"}; strings.Join(router.couponKeys, ",") != strings.Join(want, ",") {
		t.Fatalf("coupon keys = %v, want %v", router.couponKeys, want)
	}
	if !strings.Contains(router.checkoutForm, "co_test2") {
		t.Fatalf("checkout carried %q, want the replacement co_test2", router.checkoutForm)
	}
	if applied != offer {
		t.Fatalf("applied = %+v, want %+v", applied, offer)
	}
}

// Two checkouts opened after a decline must share ONE replacement: its single redemption is
// what stops both from being paid at the discount.
func TestCheckoutURLSharesOneReplacementAcrossCheckouts(t *testing.T) {
	router := &checkoutRouter{spent: map[string]bool{"co_test1": true}}
	s, pool := newTieredService(t, router.handler())
	userID := insertUser(t, pool, "two-tabs@example.com")
	offer := Discount{PercentOff: 90, Label: "promo", Key: "promo_t"}

	var forms []string
	for range 2 {
		if _, _, err := s.CheckoutURL(context.Background(), userID, "", offer); err != nil {
			t.Fatalf("CheckoutURL: %v", err)
		}
		forms = append(forms, router.checkoutForm)
	}
	if len(router.couponByKey) != 2 {
		t.Fatalf("coupons minted = %v, want the spent one and one replacement", router.couponByKey)
	}
	for _, f := range forms {
		if !strings.Contains(f, "co_test2") {
			t.Fatalf("a checkout carried %q, want the shared replacement co_test2", f)
		}
	}
}

// A replacement spent by a second decline is itself replaced: the chain is walked to its
// first usable link.
func TestCheckoutURLWalksPastASpentReplacement(t *testing.T) {
	router := &checkoutRouter{spent: map[string]bool{"co_test1": true, "co_test2": true}}
	s, pool := newTieredService(t, router.handler())
	userID := insertUser(t, pool, "declined-twice@example.com")

	if _, _, err := s.CheckoutURL(context.Background(), userID, "", Discount{PercentOff: 90, Label: "promo", Key: "promo_w"}); err != nil {
		t.Fatalf("CheckoutURL: %v", err)
	}
	if !strings.Contains(router.checkoutForm, "co_test3") {
		t.Fatalf("checkout carried %q, want co_test3", router.checkoutForm)
	}
}
