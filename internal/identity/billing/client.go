package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/platform/safehttp"
)

// apiBaseURL is the provider's REST API. Overridden only by tests.
const apiBaseURL = "https://api.stripe.com/v1"

// requestTimeout bounds one call to the provider.
//
// Short on purpose. The webhook handler calls out inline so that a candidate who has just
// paid sees Pro immediately, and the provider gives that handler a limited window before it
// disconnects and schedules a retry. Failing quickly and leaving the event for the
// reconciler is strictly better than risking the delivery.
const requestTimeout = 8 * time.Second

// errorBodyLimit caps how much of a failure response is read into an error message.
const errorBodyLimit = 512

// client talks to the payment provider. It is the only thing in this package that touches
// the network.
type client struct {
	http    *http.Client
	baseURL string
	apiKey  string
}

// newProviderClient is the production constructor: an SSRF-guarded client against the real
// API.
func newProviderClient(apiKey string) *client {
	return newClient(apiKey, apiBaseURL, safehttp.NewClient(requestTimeout))
}

// newClient builds a client against an arbitrary base URL with an arbitrary HTTP client.
// The seam exists for tests: safehttp refuses private addresses, which is correct in
// production and makes a loopback test server unreachable.
func newClient(apiKey, baseURL string, httpc *http.Client) *client {
	return &client{http: httpc, baseURL: baseURL, apiKey: apiKey}
}

// do performs one form-encoded call and decodes the JSON reply into out.
//
// The provider's API is form-encoded on the way in and JSON on the way out — an asymmetry
// worth stating once here rather than rediscovering at each call site.
//
// idempotencyKey is empty for every read and for anything the provider treats as repeatable.
// It is a named parameter rather than a general header map because there is exactly one
// header we ever add, and a map would invite a second, unreviewed one.
func (c *client) do(ctx context.Context, method, path string, form url.Values, idempotencyKey string, out any) error {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if idempotencyKey != "" {
		// The provider replays the first response for 24 hours against this key, which is
		// what makes a retried credit or a double-clicked checkout cost nothing. Without it
		// a retry after a timeout we never saw the answer to credits twice.
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("billing: %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, errorBodyLimit))
		return fmt.Errorf("billing: %s %s: provider returned %d: %s", method, path, resp.StatusCode, snippet)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("billing: decoding %s %s: %w", method, path, err)
	}
	return nil
}

// stripeSubscriptionList is the provider's reply shape for a subscription listing.
//
// CurrentPeriodEnd appears in TWO places and the item's is the live one. The provider moved
// it from the subscription onto each item — a subscription can hold items on different
// billing cycles, so the field stopped having one answer at the top. The outer field is kept
// here because an account pinned to an older API version still sends it there, and reading
// only one place would silently produce a subscription with no end.
type stripeSubscriptionList struct {
	Data []struct {
		Status            string `json:"status"`
		CurrentPeriodEnd  int64  `json:"current_period_end"`
		CancelAt          int64  `json:"cancel_at"`
		CancelAtPeriodEnd bool   `json:"cancel_at_period_end"`
		Items             struct {
			Data []struct {
				CurrentPeriodEnd int64 `json:"current_period_end"`
				Price            struct {
					ID string `json:"id"`
				} `json:"price"`
			} `json:"data"`
		} `json:"items"`
	} `json:"data"`
}

// subscriberState reads the provider's current record for one customer.
//
// `status=all` on purpose: a listing of only the active ones would make "cancelled" and
// "we asked wrongly" indistinguishable, and the entitling rule lives in one place
// (entitlingStatuses) rather than being split between a query parameter and a map.
//
// A customer the provider has never seen is not reachable here at all — callers hold a
// stored customer id or they do not call. Asking about an unknown id is an error, not an
// empty answer, because it means our binding is wrong.
func (c *client) subscriberState(ctx context.Context, customerID string) (subscriber, error) {
	form := url.Values{}
	form.Set("customer", customerID)
	form.Set("status", "all")
	form.Set("limit", "100")
	// Without this the price is a bare id string and the items carry nothing to match on.
	form.Set("expand[]", "data.items.data.price")

	var raw stripeSubscriptionList
	if err := c.do(ctx, http.MethodGet, "/subscriptions?"+form.Encode(), nil, "", &raw); err != nil {
		return subscriber{}, err
	}

	out := subscriber{Subscriptions: make([]subscription, 0, len(raw.Data))}
	for _, s := range raw.Data {
		sub := subscription{Status: s.Status}
		if s.CancelAt > 0 {
			sub.CancelAt = time.Unix(s.CancelAt, 0).UTC()
		}

		// The furthest item decides how far the subscription reaches: with items on
		// different cycles, access should last as long as the longest of them.
		periodEnd := s.CurrentPeriodEnd
		for _, item := range s.Items.Data {
			if item.CurrentPeriodEnd > periodEnd {
				periodEnd = item.CurrentPeriodEnd
			}
			if item.Price.ID != "" {
				sub.PriceIDs = append(sub.PriceIDs, item.Price.ID)
			}
		}
		if periodEnd > 0 {
			sub.CurrentPeriodEnd = time.Unix(periodEnd, 0).UTC()
		}

		out.Subscriptions = append(out.Subscriptions, sub)
	}
	return out, nil
}

// createCheckoutSession opens a hosted payment page for one account and returns where to
// send the browser.
//
// The account's own id travels in TWO places, and both earn their keep. client_reference_id
// comes back on the completion event, which is how a first purchase is attributed before
// any customer binding exists. The customer's metadata survives that event, which is how
// every later renewal is attributed once the binding does exist.
func (c *client) createCheckoutSession(ctx context.Context, userID int64, email, priceID, successURL, cancelURL, existingCustomer, couponID string) (string, error) {
	id := strconv.FormatInt(userID, 10)

	form := url.Values{}
	form.Set("mode", "subscription")
	form.Set("line_items[0][price]", priceID)
	form.Set("line_items[0][quantity]", "1")
	form.Set("client_reference_id", id)
	form.Set("success_url", successURL)
	form.Set("cancel_url", cancelURL)
	form.Set("subscription_data[metadata][freehire_user_id]", id)

	if existingCustomer != "" {
		// Reuse the customer we already know about, so a second purchase does not create a
		// second customer for one person — which would leave two subscriptions nobody sums.
		form.Set("customer", existingCustomer)
	} else {
		// No customer_creation here, and its absence is load-bearing: the provider refuses
		// it outright in subscription mode ("can only be used in `payment` mode"), and the
		// refusal is silent from a candidate's side — the session is never created, the
		// handler answers 404, and the upgrade button hides itself as though billing were
		// switched off. A subscription always creates a customer anyway.
		form.Set("metadata[freehire_user_id]", id)
		if email != "" {
			form.Set("customer_email", email)
		}
	}

	// Set only when there is one, so an ordinary purchase sends exactly the request it sent
	// before discounts existed. The provider admits ONE coupon per session — which is why
	// the caller resolves a single discount rather than handing over everything an account
	// might be owed.
	if couponID != "" {
		form.Set("discounts[0][coupon]", couponID)
	}

	var out struct {
		URL string `json:"url"`
	}
	if err := c.do(ctx, http.MethodPost, "/checkout/sessions", form, "", &out); err != nil {
		return "", err
	}
	if out.URL == "" {
		return "", fmt.Errorf("billing: provider returned a checkout session with no URL")
	}
	return out.URL, nil
}

// createPortalSession opens the provider's own subscription-management page for one
// customer and returns where to send the browser.
//
// This is where a subscriber changes their card or cancels. It is the provider's page, not
// ours, which is the whole point: a cancellation flow we wrote would be one more thing that
// can disagree with what actually happened to the money.
func (c *client) createPortalSession(ctx context.Context, customerID, returnURL string) (string, error) {
	form := url.Values{}
	form.Set("customer", customerID)
	form.Set("return_url", returnURL)

	var out struct {
		URL string `json:"url"`
	}
	if err := c.do(ctx, http.MethodPost, "/billing_portal/sessions", form, "", &out); err != nil {
		return "", err
	}
	if out.URL == "" {
		return "", fmt.Errorf("billing: provider returned a portal session with no URL")
	}
	return out.URL, nil
}
