package billing

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// Discount is a reduction to attach to one checkout session. The zero value means none,
// and must produce exactly the request this package made before discounts existed.
//
// A percentage and nothing else. What decides an account is owed one lives in
// internal/identity/promo, which this package does not import and does not know about: a
// discount is a number and a label here, and the reason it exists is somebody else's.
//
// There is deliberately no amount-off variant. A referral reward is delivered as a credit
// on the customer's balance rather than as a coupon, because a coupon would have to be
// marked consumed when a session is CREATED, and a checkout session is abandoned far more
// often than it is completed.
type Discount struct {
	// PercentOff is 1..100. Zero means no discount.
	PercentOff int32
	// Label is what the buyer sees named on the coupon. Cosmetic, and never a code.
	Label string
	// Key makes the coupon idempotent across retries and double-clicks. Empty mints a new
	// coupon on every call, which is wasteful rather than wrong.
	Key string
}

// none reports whether this is the zero discount.
func (d Discount) none() bool { return d.PercentOff <= 0 }

// createCoupon mints a one-off, single-redemption percentage discount and returns its id.
//
// `duration=once` on every coupon this package creates: each discount here applies to the
// first invoice of a subscription and must not follow it into renewals. A recurring coupon
// created by accident would be indistinguishable from a price change nobody approved.
//
// `max_redemptions=1` beside it, because the two bound different things and only together
// say what the offer means. `duration` bounds how many INVOICES of one subscription a
// coupon touches; redemptions bound how many SUBSCRIPTIONS may claim it. Without the
// second, a buyer who subscribes, cancels and subscribes again gets the discount each time
// — the idempotency key hands their checkout the same coupon back, and an unlimited coupon
// happily applies again. The offer is a first month, not a first month per attempt.
//
// The count moves when a subscription actually claims the coupon, not when a session is
// created, so an abandoned checkout costs nothing.
func (c *client) createCoupon(ctx context.Context, percentOff int32, name, idempotencyKey string) (string, error) {
	form := url.Values{}
	form.Set("duration", "once")
	form.Set("max_redemptions", "1")
	form.Set("percent_off", strconv.FormatInt(int64(percentOff), 10))
	if name != "" {
		form.Set("name", name)
	}

	var out struct {
		ID string `json:"id"`
	}
	if err := c.do(ctx, http.MethodPost, "/coupons", form, idempotencyKey, &out); err != nil {
		return "", err
	}
	if out.ID == "" {
		return "", fmt.Errorf("billing: provider returned a coupon with no id")
	}
	return out.ID, nil
}

// createCustomer registers an account with the provider before it has bought anything, and
// returns the customer id.
//
// Used only to deliver a referral reward to somebody who has never paid us: a balance
// credit needs a customer to sit on. It carries the same metadata a checkout would, so the
// customer is attributable the moment it exists rather than only after a first purchase.
func (c *client) createCustomer(ctx context.Context, userID int64, email, idempotencyKey string) (string, error) {
	id := strconv.FormatInt(userID, 10)

	form := url.Values{}
	form.Set("metadata[freehire_user_id]", id)
	if email != "" {
		form.Set("email", email)
	}

	var out struct {
		ID string `json:"id"`
	}
	if err := c.do(ctx, http.MethodPost, "/customers", form, idempotencyKey, &out); err != nil {
		return "", err
	}
	if out.ID == "" {
		return "", fmt.Errorf("billing: provider returned a customer with no id")
	}
	return out.ID, nil
}

// creditCustomerBalance places credit on a customer, which the provider consumes on that
// customer's next invoice.
//
// The amount is NEGATED here rather than by the caller. The provider's sign convention is
// the opposite of the intuitive one — a positive balance is money the customer OWES — so a
// caller passing a signed amount would eventually pass the wrong sign and bill somebody for
// a reward. Callers name a positive amount of credit; this is the one place the convention
// is applied.
func (c *client) creditCustomerBalance(ctx context.Context, customerID string, cents int64, currency, description, idempotencyKey string) error {
	form := url.Values{}
	form.Set("amount", strconv.FormatInt(-cents, 10))
	form.Set("currency", currency)
	if description != "" {
		form.Set("description", description)
	}

	path := "/customers/" + url.PathEscape(customerID) + "/balance_transactions"
	return c.do(ctx, http.MethodPost, path, form, idempotencyKey, nil)
}

// hasCollectedAtLeast reports whether any of this customer's invoices collected at least
// minCents.
//
// A THRESHOLD and not "more than nothing", because the two discounts stack in the customer's
// favour across different invoices: an invitee redeeming a 90% code pays a tenth of the list
// price, and a reward worth half of it would cost us four times what the sale brought in —
// repeatably, for as long as that code has seats. The rule the threshold states is that a
// referral never pays out more than it brought in.
//
// Deliberately NOT built on `invoices` in overview.go, which serves the receipt list. That
// one returns an empty slice when the provider fails, and falls back to `amount_due` when
// `amount_paid` is zero so a declined charge does not render as a free month. Both are
// right for a receipt list and both are wrong here: the first would read a network blip as
// "collected nothing" and silently deny a reward somebody earned, and the second would read
// an unpaid invoice as a payment.
//
// It asks about money COLLECTED and not about the subscription being active, because a
// subscription can be active having collected nothing — a trial, or a total discount.
func (c *client) hasCollectedAtLeast(ctx context.Context, customerID string, minCents int64) (bool, error) {
	// A threshold of zero would make `amount_paid >= 0` true of an UNPAID invoice, which is
	// the exact opposite of the question. Callers pass a reward amount and it is always
	// positive; the floor is here so that a future caller passing nothing cannot silently
	// turn this into "has any invoice at all".
	if minCents < 1 {
		minCents = 1
	}

	// One page, newest first, and deliberately not paginated. The only caller walks rewards
	// that are still PENDING, and a reward stops being pending at the first invoice meeting
	// this threshold — so the window this has to cover is from the invitee's signup to their
	// first real payment. Reaching past a hundred invoices would mean a hundred billing
	// periods of paying less than half the list price and then paying it, which is not a
	// case; paginating for it would spend a request per page on every account that has
	// simply never paid, which is almost all of them.
	form := url.Values{}
	form.Set("customer", customerID)
	form.Set("limit", "100")

	var raw struct {
		Data []struct {
			AmountPaid int64 `json:"amount_paid"`
		} `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, "/invoices?"+form.Encode(), nil, "", &raw); err != nil {
		return false, err
	}
	for _, invoice := range raw.Data {
		if invoice.AmountPaid >= minCents {
			return true, nil
		}
	}
	return false, nil
}
