package billing

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Overview is what a subscriber's own billing section shows: what they are paying, when it
// is next taken, and what has been taken so far.
//
// It is READ THROUGH to the provider every time rather than mirrored into our database.
// Money is the one thing a stale copy is least forgivable about — a receipt list that has
// silently missed a refund is worse than no receipt list — and the provider is the only
// party that knows what actually happened to a card.
type Overview struct {
	// Status is the provider's own word: active, trialing, past_due, canceled…
	Status string `json:"status"`
	// AmountCents and Interval describe what is being charged, read from the subscription's
	// own price rather than from our configuration — a subscriber on an older price is
	// paying that price, not today's.
	AmountCents int64  `json:"amount_cents"`
	Currency    string `json:"currency"`
	Interval    string `json:"interval"`
	// RenewsAt is when the next charge is due. Zero when the subscription is ending.
	RenewsAt *time.Time `json:"renews_at,omitempty"`
	// EndsAt is set when the subscriber has cancelled: access runs to here and stops. It is
	// shown INSTEAD of a renewal date, because "renews on the 4th" beside "you cancelled" is
	// the kind of contradiction that generates support mail.
	EndsAt   *time.Time `json:"ends_at,omitempty"`
	Invoices []Invoice  `json:"invoices"`
}

// Invoice is one charge as a receipt list shows it.
type Invoice struct {
	Date        time.Time `json:"date"`
	AmountCents int64     `json:"amount_cents"`
	Currency    string    `json:"currency"`
	// Status is the provider's: paid, open, void, uncollectible. A failed charge stays
	// visible — hiding it would leave a subscriber wondering why their card was declined.
	Status string `json:"status"`
	// ReceiptURL is the provider's hosted invoice. Absent for a draft.
	ReceiptURL string `json:"receipt_url,omitempty"`
}

// invoiceHistory bounds how far back the receipt list goes. A year of monthly charges fits,
// and anyone needing more has the provider's own portal, which this page links to.
const invoiceHistory = 12

// SubscriptionOverview reads what this account is paying and what it has paid.
//
// ErrNoSubscription when the account has never transacted — the ordinary state for a free
// account, and the caller renders nothing rather than an error.
func (s *Service) SubscriptionOverview(ctx context.Context, userID int64) (Overview, error) {
	if !s.Enabled() {
		return Overview{}, ErrDisabled
	}
	customer, err := s.customerOf(ctx, userID)
	if err != nil {
		return Overview{}, err
	}

	sub, err := s.client.subscriberState(ctx, customer)
	if err != nil {
		return Overview{}, err
	}

	out := Overview{Invoices: []Invoice{}}

	// The subscription that decides the plan is the one that reaches furthest — the same
	// rule proUntilFrom applies, so the section cannot describe a different subscription
	// from the one the plan came from.
	var best subscription
	var bestUntil time.Time
	for _, candidate := range sub.Subscriptions {
		if !entitlingStatuses[candidate.Status] || !candidate.coversAny(s.cfg.Prices) {
			continue
		}
		if until := candidate.until(); until.After(bestUntil) {
			best, bestUntil = candidate, until
		}
	}

	if best.Status != "" {
		out.Status = best.Status
		out.AmountCents, out.Currency, out.Interval = s.priceOf(ctx, best)
		when := bestUntil
		if !best.CancelAt.IsZero() {
			out.EndsAt = &when
		} else if !when.IsZero() {
			out.RenewsAt = &when
		}
	}

	out.Invoices = s.client.invoices(ctx, customer)
	return out, nil
}

// priceOf reads what this subscription actually charges. A subscriber on a price we no
// longer sell is paying THAT price, and showing today's would be a lie about their bill.
func (s *Service) priceOf(ctx context.Context, sub subscription) (int64, string, string) {
	for _, id := range sub.PriceIDs {
		if p, err := s.client.price(ctx, id); err == nil {
			return p.AmountCents, p.Currency, p.Interval
		}
	}
	return 0, "", ""
}

// invoices lists this customer's charges, newest first.
//
// A failure returns an empty list rather than an error: the subscription details are the
// answer to "what am I paying", and losing the receipt list should not take that down with
// it.
func (c *client) invoices(ctx context.Context, customerID string) []Invoice {
	form := url.Values{}
	form.Set("customer", customerID)
	form.Set("limit", strconv.Itoa(invoiceHistory))

	var raw struct {
		Data []struct {
			Created          int64  `json:"created"`
			AmountPaid       int64  `json:"amount_paid"`
			AmountDue        int64  `json:"amount_due"`
			Currency         string `json:"currency"`
			Status           string `json:"status"`
			HostedInvoiceURL string `json:"hosted_invoice_url"`
		} `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, "/invoices?"+form.Encode(), nil, &raw); err != nil {
		return []Invoice{}
	}

	out := make([]Invoice, 0, len(raw.Data))
	for _, in := range raw.Data {
		amount := in.AmountPaid
		if amount == 0 {
			// An unpaid or failed invoice still has an amount, and showing it as zero would
			// make a declined charge look like a free month.
			amount = in.AmountDue
		}
		out = append(out, Invoice{
			Date:        time.Unix(in.Created, 0).UTC(),
			AmountCents: amount,
			Currency:    in.Currency,
			Status:      in.Status,
			ReceiptURL:  in.HostedInvoiceURL,
		})
	}
	return out
}
