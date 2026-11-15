package billing

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// Discount is a percentage reduction to attach to one checkout session. The zero value
// means none. Who is owed one is decided in internal/identity/promo, not here.
//
// There is no amount-off variant: referral rewards are balance credits, since a coupon would
// be consumed when a session is created and sessions are mostly abandoned.
type Discount struct {
	// PercentOff is 1..100. Zero means no discount.
	PercentOff int32
	// Label is what the buyer sees named on the coupon. Cosmetic, and never a code.
	Label string
	// Key makes the coupon idempotent across retries. Empty mints a new coupon per call.
	Key string
}

// none reports whether this is the zero discount.
func (d Discount) none() bool { return d.PercentOff <= 0 }

// createCoupon mints a one-off, single-redemption percentage discount and returns its id.
//
// `duration=once` keeps the discount on the first invoice only. `max_redemptions=1` bounds
// subscriptions, not invoices: without it a buyer who cancels and resubscribes gets the same
// coupon back via the idempotency key and the discount applies again. The count moves when
// a subscription claims the coupon, so an abandoned checkout costs nothing.
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

// couponValid reports whether a coupon can still be applied. A replayed idempotent create
// returns the coupon as it was born, not its current state.
func (c *client) couponValid(ctx context.Context, id string) (bool, error) {
	var out struct {
		Valid bool `json:"valid"`
	}
	if err := c.do(ctx, http.MethodGet, "/coupons/"+url.PathEscape(id), nil, "", &out); err != nil {
		return false, err
	}
	return out.Valid, nil
}

// createCustomer registers an account that has not bought anything and returns the customer
// id. Used only to give a referral reward a balance to sit on; it carries the same metadata
// as checkout so the customer is attributable at once.
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

// creditCustomerBalance places credit on a customer, consumed on their next invoice.
//
// The amount is negated here, not by the caller: the provider's balance is money the
// customer owes, so callers pass a positive credit and this is the one place the sign flips.
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
// It is a threshold so a referral never pays out more than the sale brought in (a heavily
// discounted invitee must not trigger a full reward). It checks money collected, not an
// active subscription, which may have collected nothing (trial, total discount).
//
// Not built on `invoices` in overview.go: that returns empty on provider failure (here a
// network blip would deny an earned reward) and falls back to `amount_due` (here an unpaid
// invoice would count as paid).
func (c *client) hasCollectedAtLeast(ctx context.Context, customerID string, minCents int64) (bool, error) {
	// A zero threshold would make `amount_paid >= 0` true of an unpaid invoice.
	if minCents < 1 {
		minCents = 1
	}

	// One page, newest first, deliberately not paginated: a pending reward only needs the
	// window up to the invitee's first real payment, and paging would cost a request per page
	// for every account that never paid.
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
