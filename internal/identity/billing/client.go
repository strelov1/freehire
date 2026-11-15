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

// requestTimeout bounds one call to the provider. Short because the webhook handler calls out
// inline and the provider retries a slow delivery; the reconciler picks up what fails.
const requestTimeout = 8 * time.Second

// errorBodyLimit caps how much of a failure response is read into an error message.
const errorBodyLimit = 512

// client talks to the payment provider; the only network access in this package.
type client struct {
	http    *http.Client
	baseURL string
	apiKey  string
}

// newProviderClient is the production constructor: an SSRF-guarded client that refuses
// redirects (see refuseRedirect).
func newProviderClient(apiKey string) *client {
	httpc := safehttp.NewClient(requestTimeout)
	httpc.CheckRedirect = refuseRedirect
	return newClient(apiKey, apiBaseURL, httpc)
}

// newClient builds a client against an arbitrary base URL with an arbitrary HTTP client.
// The seam exists for tests: safehttp refuses private addresses, so a loopback test server
// is unreachable through the production client.
func newClient(apiKey, baseURL string, httpc *http.Client) *client {
	return &client{http: httpc, baseURL: baseURL, apiKey: apiKey}
}

// refuseRedirect keeps the secret key from travelling to any host it was not addressed to.
// Go's default policy follows redirects with the Authorization header (even HTTPS to HTTP),
// and the SSRF guard does not stop a public host from reading it. The APIs called here do
// not redirect, so a redirect is a misconfiguration or an attack.
func refuseRedirect(req *http.Request, _ []*http.Request) error {
	return fmt.Errorf("billing: refusing a redirect to %s — the provider key travels to one host only", req.URL.Host)
}

// do performs one form-encoded call and decodes the JSON reply into out (the API is
// form-encoded in, JSON out). idempotencyKey is empty for reads and repeatable calls.
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
		// The provider replays the first response for 24 hours per key, so a retry after a
		// lost answer does not act twice.
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
// CurrentPeriodEnd appears on both the subscription (older API versions) and each item (the
// live one); read both or a subscription ends up with no end.
type stripeSubscriptionList struct {
	Data []struct {
		ID                string `json:"id"`
		Status            string `json:"status"`
		CurrentPeriodEnd  int64  `json:"current_period_end"`
		CancelAt          int64  `json:"cancel_at"`
		CancelAtPeriodEnd bool   `json:"cancel_at_period_end"`
		Items             struct {
			Data []struct {
				ID               string `json:"id"`
				CurrentPeriodEnd int64  `json:"current_period_end"`
				Price            struct {
					ID string `json:"id"`
				} `json:"price"`
			} `json:"data"`
		} `json:"items"`
	} `json:"data"`
}

// subscriberState reads the provider's current record for one customer.
//
// It lists `status=all` so the entitling rule stays in one place (entitlingStatuses). An
// unknown customer id is an error, not an empty answer: it means our binding is wrong.
func (c *client) subscriberState(ctx context.Context, customerID string) (subscriber, error) {
	form := url.Values{}
	form.Set("customer", customerID)
	form.Set("status", "all")
	form.Set("limit", "100")
	// Without this the price is a bare id string.
	form.Set("expand[]", "data.items.data.price")

	var raw stripeSubscriptionList
	if err := c.do(ctx, http.MethodGet, "/subscriptions?"+form.Encode(), nil, "", &raw); err != nil {
		return subscriber{}, err
	}

	out := subscriber{Subscriptions: make([]subscription, 0, len(raw.Data))}
	for _, s := range raw.Data {
		sub := subscription{ID: s.ID, Status: s.Status}
		if s.CancelAt > 0 {
			sub.CancelAt = time.Unix(s.CancelAt, 0).UTC()
		}

		// The furthest item decides how long access lasts.
		periodEnd := s.CurrentPeriodEnd
		for i, item := range s.Items.Data {
			if item.CurrentPeriodEnd > periodEnd {
				periodEnd = item.CurrentPeriodEnd
			}
			if item.Price.ID != "" {
				sub.PriceIDs = append(sub.PriceIDs, item.Price.ID)
			}
			if i == 0 {
				// Every subscription created here has exactly one item (see
				// createCheckoutSession); plan changes replace its price.
				sub.ItemID = item.ID
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
// The account id travels in two places: client_reference_id attributes the first purchase
// (no customer binding yet), and the customer metadata attributes later renewals.
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
		// Reuse the known customer so a second purchase does not split one person in two.
		form.Set("customer", existingCustomer)
	} else {
		// Do not set customer_creation: the provider refuses it in subscription mode, the
		// session is never created and the upgrade button silently hides. A subscription
		// creates a customer anyway.
		form.Set("metadata[freehire_user_id]", id)
		if email != "" {
			form.Set("customer_email", email)
		}
	}

	// The provider admits one coupon per session, so the caller resolves a single discount.
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

// updateSubscriptionPrice swaps an existing subscription's item to another price with
// proration (upgrade/downgrade), never creating a second subscription.
//
// proration_behavior is explicit so an upstream default change cannot alter mid-cycle charges.
// idempotencyKey is required: Go's transport can replay a POST with a seekable body, which
// would bill the proration twice.
func (c *client) updateSubscriptionPrice(ctx context.Context, subscriptionID, itemID, priceID, idempotencyKey string) error {
	form := url.Values{}
	form.Set("items[0][id]", itemID)
	form.Set("items[0][price]", priceID)
	form.Set("proration_behavior", "create_prorations")

	return c.do(ctx, http.MethodPost, "/subscriptions/"+subscriptionID, form, idempotencyKey, nil)
}

// createPortalSession opens the provider's own subscription-management page for one
// customer and returns where to send the browser.
//
// This is where a subscriber changes their card or cancels; it is the provider's page so it
// cannot disagree with what happened to the money.
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
