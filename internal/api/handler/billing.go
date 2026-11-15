package handler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/identity/billing"
	"github.com/strelov1/freehire/internal/identity/promo"
)

// billingHandlers serve the Pro subscription: where a candidate buys it and where the provider reports back.
// Neither route exists when billing is unconfigured (see register), so a deployment without credentials cannot be probed.
type billingHandlers struct {
	billing *billing.Service
	store   *billing.RevenueCat
	// promo decides the buyer's discount. Held here, not in billing: a discount is a reason to buy, not part of the subscription.
	promo *promo.Service
}

func newBillingHandlers(svc *billing.Service, store *billing.RevenueCat, discounts *promo.Service) *billingHandlers {
	return &billingHandlers{billing: svc, store: store, promo: discounts}
}

// webhookProvider is what a webhook route needs from a provider: verify, record, apply, or give up on a delivery.
// Both providers share one route so the rule that must not drift lives once: acknowledge what is STORED, never what is applied.
type webhookProvider interface {
	Enabled() bool
	SignatureHeader() string
	Accept(raw []byte, signature string, now time.Time) (billing.Event, error)
	Record(ctx context.Context, ev billing.Event) (int64, bool, error)
	Apply(ctx context.Context, rowID int64, ev billing.Event) error
	MarkProcessed(ctx context.Context, rowID int64) error
}

// register mounts the billing routes, or nothing at all. A route never mounted is indistinguishable from a build
// without it; a handler answering 404 can still be probed for timing and shape.
func (h *billingHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	// The two providers mount independently: a deployment may sell only on the web or only in the apps.
	h.registerStore(api, mw, contract)

	if !h.billing.Enabled() {
		return
	}
	// The only unauthenticated POST here: authenticated by the provider's HMAC (billing.verifySignature),
	// verified before anything is read out of the body.
	api.Post("/billing/stripe/webhook", webhookFor(h.billing))
	// Cookie only: a checkout link decides who gets charged, so it is never minted for an API key.
	api.Get("/billing/checkout", mw.cookie, contract)
	// Cookie only, like checkout. Its own route, not a field on /me/plan, so the plan surface keeps answering when the provider is down.
	api.Get("/billing/manage", mw.cookie, contract)
	// Read through to the provider; its own route for the same reason as manage.
	api.Get("/billing/subscription", mw.cookie, contract)
	api.Get("/billing/attempts", mw.cookie, contract)
}

// GetBillingAttempts reads only the session account's history, including failed first purchases.
func (h *billingHandlers) GetBillingAttempts(ctx context.Context) (*apigen.BillingAttemptsHeaders, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, applyTimeout)
	defer cancel()
	attempts, err := h.billing.PaymentAttempts(ctx, userID)
	if err != nil {
		log.Printf("billing: could not read payment attempts of user %d: %v", userID, err)
		return nil, fiber.NewError(fiber.StatusServiceUnavailable, "payment history is temporarily unavailable")
	}
	data := make([]apigen.BillingPaymentAttempt, len(attempts))
	for i, attempt := range attempts {
		data[i] = apigen.BillingPaymentAttempt{
			ID: attempt.ID, Date: attempt.Date,
			AmountCents: attempt.AmountCents, AmountRefundedCents: attempt.AmountRefundedCents,
			Currency: attempt.Currency, Status: attempt.Status, Reason: optString(attempt.Reason),
			CardBrand: optString(attempt.CardBrand), CardLast4: optString(attempt.CardLast4),
			CardFunding: optString(attempt.CardFunding), Wallet: optString(attempt.Wallet),
		}
	}
	return &apigen.BillingAttemptsHeaders{CacheControl: "no-store", Response: apigen.BillingAttempts{Data: data}}, nil
}

// applyTimeout bounds the inline attempt to bring the plan up to date, so a just-paid candidate sees Pro at once.
// Well inside the provider's 60s budget; failing is free: the event is recorded and cmd/billing-sync finishes it.
const applyTimeout = 10 * time.Second

// Webhook receives a provider event. The order is the contract: verify, record, acknowledge, then apply.
//   - a delivery that does not verify is refused and nothing is written;
//   - one we cannot RECORD is not acknowledged: a 200 claims the event is stored;
//   - one we cannot APPLY is still acknowledged: it IS stored and the reconciler owns it.
func webhookFor(p webhookProvider) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// c.Request().Body(), not c.Ctx.Body(): Fiber's Body() decompresses (gzip/deflate/brotli) before authentication, so the
		// HMAC would cover the wrong bytes and a small compressed body could expand unbounded on this unauthenticated POST
		// (BodyLimit bounds only the wire body).
		event, err := p.Accept(c.Request().Body(), c.Get(p.SignatureHeader()), time.Now())
		if err != nil {
			log.Printf("billing: refusing a webhook delivery: %v", err)
			// Unverified: 401, nothing proves it came from the provider. Unparseable: 400, a retry yields the same bytes,
			// and a retryable status for a permanent failure is how a provider decides to disable the endpoint.
			if !errors.Is(err, billing.ErrBadSignature) {
				return fiber.NewError(fiber.StatusBadRequest, "malformed webhook payload")
			}
			return fiber.NewError(fiber.StatusUnauthorized, "invalid webhook signature")
		}

		rowID, recorded, err := p.Record(c.Context(), event)
		if err != nil {
			log.Printf("billing: could not record event %s: %v", event.ID, err)
			return fiber.NewError(fiber.StatusInternalServerError, "could not record the event")
		}
		if !recorded {
			// A redelivery: the provider reuses the event id on retry, so this is the normal case.
			return c.JSON(fiber.Map{"data": fiber.Map{"status": "duplicate"}})
		}

		applyNow(c, p, rowID, event)
		return c.JSON(fiber.Map{"data": fiber.Map{"status": "recorded"}})
	}
}

// applyNow makes the best-effort inline attempt to bring the plan up to date. It never
// returns an error, because nothing it can discover changes the response.
func applyNow(c *fiber.Ctx, p webhookProvider, rowID int64, event billing.Event) {
	ctx, cancel := context.WithTimeout(c.Context(), applyTimeout)
	defer cancel()

	err := p.Apply(ctx, rowID, event)
	if err == nil {
		return
	}

	// An event nobody can attribute (unmetered, or created outside this integration): stamp it processed
	// so it never sits in the reconciler's queue.
	if errors.Is(err, billing.ErrUnknownSubscriber) || errors.Is(err, billing.ErrNoSubscription) {
		log.Printf("billing: event %s (%s) names no account we meter — recorded, not applied", event.ID, event.Type)
		if markErr := p.MarkProcessed(ctx, rowID); markErr != nil {
			log.Printf("billing: could not stamp unattributable event %s: %v", event.ID, markErr)
		}
		return
	}

	log.Printf("billing: event %s recorded but not applied, leaving it for the reconciler: %v", event.ID, err)
}

// Checkout returns where this caller buys Pro. The identifier comes from the session, never the request:
// it decides who gets charged.
func (h *billingHandlers) GetBillingCheckout(ctx context.Context, params apigen.GetBillingCheckoutParams) (*apigen.GetBillingCheckoutOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, applyTimeout)
	defer cancel()

	discount := h.discountFor(ctx, userID)

	// The price comes from the pricing page's monthly/annual choice. It is validated
	// against the configured list inside the service — never trusted as sent.
	url, applied, err := h.billing.CheckoutURL(ctx, userID, params.Price.Value, discount)
	if billingAbsent(err) {
		// Checkout is unconfigured or the price is not offered: the surface omits the offer.
		return nil, fiber.NewError(fiber.StatusNotFound, "checkout is not available")
	}
	if err != nil {
		return nil, err
	}
	return &apigen.GetBillingCheckoutOK{Data: apigen.BillingCheckout{
		URL: url,
		// Which offer was APPLIED, not merely offered. Empty when none, including a tier change that modified
		// a subscription in place.
		DiscountPercent: int64(applied.PercentOff),
		DiscountSource:  applied.Label,
	}}, nil
}

// discountFor reads the ONE discount this checkout may carry, and writes nothing.
// This GET must stay side-effect free: SameSite=Lax sends the session cookie on cross-site navigation, so a link here
// could burn the visitor's one lifetime redemption. Redeeming is the POST /me/promo/redeem.
// A read failure is logged and the sale goes ahead at list price.
func (h *billingHandlers) discountFor(ctx context.Context, userID int64) billing.Discount {
	if h.promo == nil {
		return billing.Discount{}
	}

	resolved, err := h.promo.Discount(ctx, userID)
	if err != nil {
		log.Printf("promo: resolving the discount of user %d: %v", userID, err)
		return billing.Discount{}
	}
	if resolved.Percent <= 0 {
		return billing.Discount{}
	}
	return billing.Discount{
		PercentOff: int32(resolved.Percent),
		Label:      resolved.Source,
		Key:        fmt.Sprintf("%s_%d_%d", resolved.Source, userID, resolved.Percent),
	}
}

// Subscription returns what the caller is paying and has been charged. 404 when there is none (the free-account norm).
// Money is read from the provider each time, not mirrored: a receipt list that missed a refund is worse than none.
func (h *billingHandlers) GetBillingSubscription(ctx context.Context) (*apigen.GetBillingSubscriptionOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, applyTimeout)
	defer cancel()

	out, err := h.billing.SubscriptionOverview(ctx, userID)
	if billingAbsent(err) {
		return nil, fiber.NewError(fiber.StatusNotFound, "no subscription")
	}
	if err != nil {
		return nil, err
	}
	data := apigen.BillingOverview{
		Status: out.Status, AmountCents: out.AmountCents, Currency: out.Currency, Interval: out.Interval,
		Invoices:              make([]apigen.BillingInvoice, len(out.Invoices)),
		MultipleSubscriptions: apigen.OptBool{Value: true, Set: out.MultipleSubscriptions},
	}
	if out.RenewsAt != nil {
		data.RenewsAt = apigen.NewOptDateTime(*out.RenewsAt)
	}
	if out.EndsAt != nil {
		data.EndsAt = apigen.NewOptDateTime(*out.EndsAt)
	}
	for i, in := range out.Invoices {
		data.Invoices[i] = apigen.BillingInvoice{ID: in.ID, Date: in.Date, AmountCents: in.AmountCents,
			Currency: in.Currency, Status: in.Status, ReceiptURL: optString(in.ReceiptURL)}
	}
	return &apigen.GetBillingSubscriptionOK{Data: data}, nil
}

// Manage returns the provider's page where the subscriber changes their card or cancels.
// Deleting a freehire account does NOT cancel a subscription; the delete surface links here.
// The URL is generated by the provider per visit, never composed here.
func (h *billingHandlers) GetBillingManage(ctx context.Context) (*apigen.GetBillingManageOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, applyTimeout)
	defer cancel()

	url, err := h.billing.ManagementURL(ctx, userID)
	if billingAbsent(err) {
		return nil, fiber.NewError(fiber.StatusNotFound, "no subscription to manage")
	}
	if err != nil {
		return nil, err
	}
	return &apigen.GetBillingManageOK{Data: apigen.BillingUrl{URL: url}}, nil
}

// billingAbsent is the state with nothing to show: billing unconfigured or no subscription. Anything else,
// such as the provider down, is a fault: 500 and reported, not passed off as "no subscription".
func billingAbsent(err error) bool {
	return errors.Is(err, billing.ErrNoSubscription) || errors.Is(err, billing.ErrDisabled) || errors.Is(err, billing.ErrNoCheckout)
}
