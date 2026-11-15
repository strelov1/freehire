package billing

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Overview is what a subscriber's billing section shows: what they pay, when it is next
// taken, and what has been taken. It is read through to the provider every time, never
// mirrored, since a stale copy of money (a missed refund) is worse than none.
type Overview struct {
	// Status is the provider's own word: active, trialing, past_due, canceled…
	Status string `json:"status"`
	// AmountCents and Interval are read from the subscription's own price, not our
	// configuration: a subscriber on an older price pays that price.
	AmountCents int64  `json:"amount_cents"`
	Currency    string `json:"currency"`
	Interval    string `json:"interval"`
	// RenewsAt is when the next charge is due. Zero when the subscription is ending.
	RenewsAt *time.Time `json:"renews_at,omitempty"`
	// EndsAt is set when the subscriber has cancelled: access stops here. Shown instead of a
	// renewal date so the two never contradict.
	EndsAt   *time.Time `json:"ends_at,omitempty"`
	Invoices []Invoice  `json:"invoices"`
	// MultipleSubscriptions is true when the customer has more than one active entitling
	// subscription (a duplicate). The section still describes the single best one.
	MultipleSubscriptions bool `json:"multiple_subscriptions,omitempty"`
}

// Invoice is one charge as a receipt list shows it.
type Invoice struct {
	// ID is the provider's invoice identifier, carried as the SPA list key. A date is not a
	// key: two invoices can share a second (proration beside a cycle charge), and a duplicate
	// key in a Svelte {#each} takes the block down.
	ID          string    `json:"id"`
	Date        time.Time `json:"date"`
	AmountCents int64     `json:"amount_cents"`
	Currency    string    `json:"currency"`
	// Status is the provider's: paid, open, void, uncollectible. A failed charge stays visible.
	Status string `json:"status"`
	// ReceiptURL is the provider's hosted invoice. Absent for a draft.
	ReceiptURL string `json:"receipt_url,omitempty"`
}

// invoiceHistory bounds the receipt list; older ones are in the provider's portal.
const invoiceHistory = 12

// SubscriptionOverview reads what this account is paying and what it has paid.
//
// It returns ErrNoSubscription for an account that never transacted (a free account); the
// caller renders nothing.
func (s *Service) SubscriptionOverview(ctx context.Context, userID int64) (Overview, error) {
	// Stripe only: receipts and renewal dates exist only for web card purchases.
	if !s.cfg.Enabled() {
		return Overview{}, ErrDisabled
	}
	customer, err := s.stripe().customerOf(ctx, userID)
	if err != nil {
		return Overview{}, err
	}
	return s.overviewFor(ctx, customer)
}

// overviewFor is SubscriptionOverview once the customer is known, split out so it can be
// tested without a database.
func (s *Service) overviewFor(ctx context.Context, customer string) (Overview, error) {
	sub, err := s.client.subscriberState(ctx, customer)
	if err != nil {
		return Overview{}, err
	}

	out := Overview{Invoices: []Invoice{}}

	// The same selection the plan derivation makes, so this section describes the subscription
	// the plan came from. It needs both price lists: an Ultra account has no Pro price.
	best, tierPrices := billedSubscription(sub, s.cfg.Prices, s.cfg.UltraPrices, time.Now().UTC())

	// No eligible subscription (free, or ended) must read as "no section", not zeroes.
	if best.Status == "" {
		return Overview{}, ErrNoSubscription
	}

	amount, currency, interval, err := s.priceOf(ctx, best, tierPrices)
	if err != nil {
		// Subscribed but price unknown: return an error rather than show "$0".
		return Overview{}, err
	}
	out.Status = best.Status
	out.AmountCents, out.Currency, out.Interval = amount, currency, interval
	out.MultipleSubscriptions = moreThanOneEntitling(sub, s.cfg.Prices, s.cfg.UltraPrices)

	when := best.CurrentPeriodEnd
	if !best.CancelAt.IsZero() {
		out.EndsAt = &when
	} else if !when.IsZero() {
		out.RenewsAt = &when
	}

	out.Invoices = s.client.invoices(ctx, customer)
	return out, nil
}

// priceOf reads what this subscription actually charges, including a price we no longer
// sell. It errors rather than returning zeroes when no price resolves, since zero renders as
// "free". The tier list only sets the order tried: during an upgrade one subscription can
// carry an item from each tier, and the amount must match the tier shown.
func (s *Service) priceOf(ctx context.Context, sub subscription, tierPrices []string) (int64, string, string, error) {
	var lastErr error
	for _, id := range tierFirst(sub.PriceIDs, tierPrices) {
		p, err := s.client.price(ctx, id)
		if err == nil {
			return p.AmountCents, p.Currency, p.Interval, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("billing: subscription carries no price we could read")
	}
	return 0, "", "", lastErr
}

// invoices lists this customer's charges, newest first. A failure returns an empty list so
// losing receipts does not take down the subscription details.
func (c *client) invoices(ctx context.Context, customerID string) []Invoice {
	form := url.Values{}
	form.Set("customer", customerID)
	form.Set("limit", strconv.Itoa(invoiceHistory))

	var raw struct {
		Data []struct {
			ID               string `json:"id"`
			Created          int64  `json:"created"`
			AmountPaid       int64  `json:"amount_paid"`
			AmountDue        int64  `json:"amount_due"`
			Currency         string `json:"currency"`
			Status           string `json:"status"`
			HostedInvoiceURL string `json:"hosted_invoice_url"`
		} `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, "/invoices?"+form.Encode(), nil, "", &raw); err != nil {
		return []Invoice{}
	}

	out := make([]Invoice, 0, len(raw.Data))
	for _, in := range raw.Data {
		amount := in.AmountPaid
		if amount == 0 {
			// An unpaid or failed invoice has no amount_paid; zero would look like a free month.
			amount = in.AmountDue
		}
		out = append(out, Invoice{
			ID:          in.ID,
			Date:        time.Unix(in.Created, 0).UTC(),
			AmountCents: amount,
			Currency:    in.Currency,
			Status:      in.Status,
			ReceiptURL:  in.HostedInvoiceURL,
		})
	}
	return out
}
