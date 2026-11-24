package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// One-off payments that fund a sponsored job campaign (openspec add-sponsored-jobs-cpc).
//
// They ride the same webhook and reconciler as the subscription, but they confer nothing on the
// account: the credit lands in internal/engage/sponsored, which this package cannot import
// (engage is above identity). The seam is TopupCrediter, injected by whoever wires the services.

const (
	// topupKind is metadata.kind on a checkout session (and its payment intent) that funds a campaign.
	topupKind = "sponsored_topup"

	// defaultMinTopupCents is the smallest top-up sold when SPONSORED_MIN_TOPUP_CENTS is unset.
	defaultMinTopupCents = 2900
)

// ErrTopupInvalid is what a TopupCrediter returns for a payment that can never be credited
// (no such campaign, or it belongs to someone else). The event is stamped processed rather than
// retried forever; the money is visible in Stripe for a human.
var ErrTopupInvalid = errors.New("billing: sponsored top-up names no campaign of the payer")

// BelowMinimumError is returned by SponsoredTopupURL for an amount under the configured minimum.
type BelowMinimumError struct{ MinCents, GotCents int64 }

func (e *BelowMinimumError) Error() string {
	return fmt.Sprintf("billing: a sponsored top-up is at least %d cents, got %d", e.MinCents, e.GotCents)
}

// Topup is one paid checkout, already re-read from the provider, ready to be credited.
type Topup struct {
	SessionID, PaymentIntentID string
	CampaignID, UserID         int64
	AmountCents                int64
}

// TopupCrediter credits a verified payment to its campaign. It MUST be idempotent by
// SessionID: the webhook and the reconciler both call it, and either may repeat.
type TopupCrediter interface {
	CreditTopup(ctx context.Context, t Topup) error
}

// SetTopupCrediter wires the sponsored side. Without it a top-up event stays unprocessed (and
// the reconciler retries it) instead of being stamped done with nothing credited.
func (s *Service) SetTopupCrediter(c TopupCrediter) { s.crediter = c }

// minTopupCents reads SPONSORED_MIN_TOPUP_CENTS (default 2900).
func (c Config) minTopupCents() int64 {
	if c.SponsoredMinTopupCents > 0 {
		return c.SponsoredMinTopupCents
	}
	return defaultMinTopupCents
}

func sponsoredMinTopupFromEnv() int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(os.Getenv("SPONSORED_MIN_TOPUP_CENTS")), 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// SponsoredTopupURL is where this account pays cents into one campaign. The caller has already
// checked the campaign is the user's; the webhook checks it again before crediting.
func (s *Service) SponsoredTopupURL(ctx context.Context, userID, campaignID, cents int64) (string, error) {
	if !s.cfg.Enabled() {
		return "", ErrDisabled
	}
	if !s.cfg.CanCheckout() {
		return "", ErrNoCheckout
	}
	if minCents := s.cfg.minTopupCents(); cents < minCents {
		return "", &BelowMinimumError{MinCents: minCents, GotCents: cents}
	}

	// Reuse a known customer, but never require one: a read failure only costs the linkage.
	var customer string
	if c, err := s.q.GetStripeCustomerID(ctx, userID); err == nil && c.Valid {
		customer = c.String
	}

	base := s.cfg.SiteURL + "/my/sponsored?topup="
	return s.client.createPaymentCheckoutSession(ctx, paymentCheckout{
		UserID: userID, CampaignID: campaignID, Cents: cents, Customer: customer,
		SuccessURL: base + "success", CancelURL: base + "cancel",
	}, uuid.NewString())
}

// RefundPaymentIntent returns a payment in full. The key makes a retry after a lost answer a no-op.
func (s *Service) RefundPaymentIntent(ctx context.Context, paymentIntentID, idempotencyKey string) error {
	if !s.cfg.Enabled() {
		return ErrDisabled
	}
	return s.client.refundPaymentIntent(ctx, paymentIntentID, idempotencyKey)
}

// paymentCheckout is one top-up checkout.
type paymentCheckout struct {
	UserID, CampaignID, Cents int64
	Customer                  string
	SuccessURL, CancelURL     string
}

// createPaymentCheckoutSession opens a one-off hosted payment page (mode=payment) with an inline
// price, and returns where to send the browser. The metadata is on the session AND the payment
// intent so a refund or dispute can be traced to the campaign from either.
func (c *client) createPaymentCheckoutSession(ctx context.Context, p paymentCheckout, idempotencyKey string) (string, error) {
	user, campaign := strconv.FormatInt(p.UserID, 10), strconv.FormatInt(p.CampaignID, 10)

	form := url.Values{}
	form.Set("mode", "payment")
	form.Set("line_items[0][quantity]", "1")
	form.Set("line_items[0][price_data][currency]", "usd")
	form.Set("line_items[0][price_data][unit_amount]", strconv.FormatInt(p.Cents, 10))
	form.Set("line_items[0][price_data][product_data][name]", "Sponsored job budget")
	form.Set("client_reference_id", user)
	form.Set("success_url", p.SuccessURL)
	form.Set("cancel_url", p.CancelURL)
	for _, prefix := range []string{"metadata", "payment_intent_data[metadata]"} {
		form.Set(prefix+"[kind]", topupKind)
		form.Set(prefix+"[campaign_id]", campaign)
		form.Set(prefix+"[user_id]", user)
	}
	if p.Customer != "" {
		form.Set("customer", p.Customer)
	}

	var out struct {
		URL string `json:"url"`
	}
	if err := c.do(ctx, http.MethodPost, "/checkout/sessions", form, idempotencyKey, &out); err != nil {
		return "", err
	}
	if out.URL == "" {
		return "", fmt.Errorf("billing: provider returned a checkout session with no URL")
	}
	return out.URL, nil
}

// paymentSession is the provider's current record of one checkout session.
type paymentSession struct {
	ID            string            `json:"id"`
	Mode          string            `json:"mode"`
	PaymentStatus string            `json:"payment_status"`
	Currency      string            `json:"currency"`
	AmountTotal   int64             `json:"amount_total"`
	PaymentIntent string            `json:"payment_intent"`
	Metadata      map[string]string `json:"metadata"`
}

func (c *client) checkoutSession(ctx context.Context, id string) (paymentSession, error) {
	var out paymentSession
	err := c.do(ctx, http.MethodGet, "/checkout/sessions/"+url.PathEscape(id), nil, "", &out)
	return out, err
}

// refundPaymentIntent refunds a payment in full. A refund the provider reports failed is an error;
// one it reports already done is not.
func (c *client) refundPaymentIntent(ctx context.Context, paymentIntentID, idempotencyKey string) error {
	form := url.Values{}
	form.Set("payment_intent", paymentIntentID)
	var out struct {
		Status string `json:"status"`
	}
	if err := c.do(ctx, http.MethodPost, "/refunds", form, idempotencyKey, &out); err != nil {
		// Already refunded (by hand in the dashboard, or under another key) is the outcome wanted;
		// as an error it would be retried by every reconciler pass forever.
		var pe *providerError
		if errors.As(err, &pe) && pe.Code() == "charge_already_refunded" {
			return nil
		}
		return err
	}
	if out.Status == "failed" || out.Status == "canceled" {
		return fmt.Errorf("billing: refund of %s ended %s", paymentIntentID, out.Status)
	}
	return nil
}

// topupSignal reads what a delivery says about a sponsored top-up: ok is false for every event
// that is not about one. It is only a pointer to the session; the facts are re-read.
func topupSignal(ev Event) (sessionID string, ok bool) {
	var obj struct {
		ID       string            `json:"id"`
		Object   string            `json:"object"`
		Metadata map[string]string `json:"metadata"`
	}
	if json.Unmarshal(ev.Payload, &obj) != nil || obj.Metadata["kind"] != topupKind {
		return "", false
	}
	if obj.Object != "checkout.session" || (ev.Type != "checkout.session.completed" && ev.Type != "checkout.session.async_payment_succeeded") {
		// A payment_intent or charge event of the same payment: nothing to credit, and no
		// subscription to sync either.
		return "", true
	}
	return obj.ID, true
}

// Apply is the engine's Apply, except for a sponsored top-up: that is credited from a RE-READ
// session (a webhook is a signal, not a fact) and the event is stamped processed only after the
// credit succeeded, so a failed credit stays for cmd/billing-sync. No subscription sync runs:
// a payment-mode session creates none, and a sync that found no customer would get the event
// stamped as unattributable.
func (s *Service) Apply(ctx context.Context, rowID int64, ev Event) error {
	session, isTopup := topupSignal(ev)
	if !isTopup {
		return s.engine.Apply(ctx, rowID, ev)
	}
	if session != "" {
		if err := s.creditTopup(ctx, session); err != nil {
			if !errors.Is(err, ErrTopupInvalid) {
				return err
			}
			log.Printf("billing: sponsored top-up %s cannot be credited, stamping it: %v", session, err)
		}
	}
	return s.MarkProcessed(ctx, rowID)
}

// creditTopup re-reads the session and credits it only if it is a paid USD payment-mode session.
func (s *Service) creditTopup(ctx context.Context, sessionID string) error {
	if s.crediter == nil {
		return errors.New("billing: no sponsored top-up crediter is wired")
	}
	sess, err := s.client.checkoutSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if sess.Metadata["kind"] != topupKind || sess.Mode != "payment" || sess.Currency != "usd" {
		return fmt.Errorf("%w: session %s is not a USD sponsored payment", ErrTopupInvalid, sessionID)
	}
	if sess.PaymentStatus != "paid" {
		// Async methods complete unpaid first; async_payment_succeeded comes later and credits.
		return nil
	}
	campaignID, cErr := strconv.ParseInt(sess.Metadata["campaign_id"], 10, 64)
	userID, uErr := strconv.ParseInt(sess.Metadata["user_id"], 10, 64)
	if cErr != nil || uErr != nil || campaignID <= 0 || userID <= 0 || sess.AmountTotal <= 0 || sess.PaymentIntent == "" {
		return fmt.Errorf("%w: session %s metadata is incomplete", ErrTopupInvalid, sessionID)
	}
	return s.crediter.CreditTopup(ctx, Topup{
		SessionID: sess.ID, PaymentIntentID: sess.PaymentIntent,
		CampaignID: campaignID, UserID: userID, AmountCents: sess.AmountTotal,
	})
}
