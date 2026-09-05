package handler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/billing"
	"github.com/strelov1/freehire/internal/identity/promo"
)

// billingHandlers serve the two ends of the Pro subscription: where a candidate goes to
// buy it, and where the provider tells us what happened.
//
// Neither route exists when billing is unconfigured — see register. That is deliberate and
// it is what makes this code safe to ship in a public repository: a deployment that never
// sets the credentials cannot tell the endpoints are there.
type billingHandlers struct {
	billing *billing.Service
	store   *billing.RevenueCat
	// promo decides what discount the buyer is owed. Held here rather than imported by
	// billing, because a discount is a reason to buy and billing's scope is the
	// subscription itself — the two packages meet in this handler and nowhere else.
	promo *promo.Service
}

func newBillingHandlers(svc *billing.Service, store *billing.RevenueCat, discounts *promo.Service) *billingHandlers {
	return &billingHandlers{billing: svc, store: store, promo: discounts}
}

// webhookProvider is the part of a billing provider a webhook route needs, and it is all the
// route needs: verify a delivery, record it, apply it, or give up on it.
//
// The route is identical for both providers because everything that differs between them —
// the header name, the signature window, the envelope, how an account is addressed — is
// already answered behind billing's own seam. Writing it twice would give two copies of the
// one rule that must not drift: acknowledge what is STORED, never what is applied.
type webhookProvider interface {
	Enabled() bool
	SignatureHeader() string
	Accept(raw []byte, signature string, now time.Time) (billing.Event, error)
	Record(ctx context.Context, ev billing.Event) (int64, bool, error)
	Apply(ctx context.Context, rowID int64, ev billing.Event) error
	MarkProcessed(ctx context.Context, rowID int64) error
}

// register mounts the billing routes, or mounts nothing at all.
//
// Not registering is the whole of the "disabled" behaviour. A route that exists and
// answers 404 from inside the handler is a route an attacker can still probe for timing
// and shape; a route that was never mounted is indistinguishable from a build without
// this file. It also means no handler here has to remember the check.
func (h *billingHandlers) register(api fiber.Router, mw middleware) {
	// The two providers mount independently. A deployment that sells only on the web is a
	// legitimate one, and so is one that sells only in the apps — an `if !stripe { return }`
	// covering both would make the second impossible and would say so nowhere.
	h.registerStore(api, mw)

	if !h.billing.Enabled() {
		return
	}
	// The only unauthenticated POST here. It is authenticated by the provider's HMAC
	// signature instead of by a session — see billing.verifySignature — which is why the
	// verification happens before anything is read out of the body.
	api.Post("/billing/stripe/webhook", webhookFor(h.billing))
	// Cookie only. A checkout link decides who gets charged, so it is minted for a browser
	// session and never for an API key, in the same spirit as key management itself.
	api.Get("/billing/checkout", mw.cookie, h.Checkout)
	// Where the caller cancels or changes their card. Cookie-only for the same reason as
	// checkout, and its own route rather than a field on /me/plan because it is a call to
	// the provider — the plan surface must stay readable when the provider is not.
	api.Get("/billing/manage", mw.cookie, h.Manage)
	// What the caller is paying and what has been taken. Read through to the provider, so
	// it is its own route rather than a field on /me/plan — that surface must keep answering
	// when the provider does not.
	api.Get("/billing/subscription", mw.cookie, h.Subscription)
}

// applyTimeout bounds the inline attempt to bring the user's plan up to date.
//
// The provider allows 60 seconds before it disconnects and schedules a retry, and it
// advises acknowledging first and deferring the work. We do the work inline anyway,
// bounded well inside that budget, because a candidate who has just paid should see Pro on
// the next page rather than at the top of the hour — and failing this is free: the event
// is already recorded and cmd/billing-sync will finish the job.
const applyTimeout = 10 * time.Second

// Webhook receives a provider event.
//
// The order is the contract. Verify, record, acknowledge, then apply:
//
//   - a delivery that does not verify is refused and nothing is written;
//   - a delivery we cannot RECORD is not acknowledged, because a 200 is a claim that the
//     event is stored and claiming that falsely is the one way to lose it for good;
//   - a delivery we cannot APPLY is still acknowledged, because it IS stored, and the
//     reconciler owns what happens next.
func webhookFor(p webhookProvider) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// c.Request().Body() and NOT c.Ctx.Body(), which is neither raw nor safe here.
		//
		// Fiber's Body() reads Content-Encoding and DECOMPRESSES, chaining up to three layers
		// (gzip, deflate, brotli). Two consequences, and the second is the reason this route
		// cannot use it. The HMAC covers the bytes the provider sent, so a body that arrived
		// encoded would be verified against something else entirely. And the decompression
		// happens BEFORE any authentication — the server's 8MB BodyLimit bounds the wire body,
		// not what it expands to, so on the one unauthenticated POST in the app a few compressed
		// megabytes become an unbounded allocation. fasthttp's Body() is the bytes as received.
		event, err := p.Accept(c.Request().Body(), c.Get(p.SignatureHeader()), time.Now())
		if err != nil {
			log.Printf("billing: refusing a webhook delivery: %v", err)
			// A delivery that does not VERIFY is refused with 401 and the provider gives up on
			// it, which is right: nothing proves it came from them. A delivery that verifies but
			// does not PARSE is a 400 — retrying it can only produce the same bytes, and an
			// endpoint that answers a permanent failure with a retryable status is how the
			// provider decides the endpoint is broken and disables it.
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
			// A redelivery. The provider retries anything it did not get a 200 for and reuses
			// the event id, so this is the normal case rather than an anomaly.
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

	// An event nobody can ever attribute — one about something we do not meter, or an
	// object created outside this integration. Stamping it processed keeps it out of the
	// reconciler's queue forever rather than leaving a row that fails every hour until
	// someone reads the logs.
	if errors.Is(err, billing.ErrUnknownSubscriber) || errors.Is(err, billing.ErrNoSubscription) {
		log.Printf("billing: event %s (%s) names no account we meter — recorded, not applied", event.ID, event.Type)
		if markErr := p.MarkProcessed(ctx, rowID); markErr != nil {
			log.Printf("billing: could not stamp unattributable event %s: %v", event.ID, markErr)
		}
		return
	}

	log.Printf("billing: event %s recorded but not applied, leaving it for the reconciler: %v", event.ID, err)
}

// Checkout returns where this caller buys Pro.
//
// The identifier in the URL is taken from the session, never from the request. It decides
// who gets charged and who becomes Pro, and a value the browser composes is a value the
// browser can change.
func (h *billingHandlers) Checkout(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(c.Context(), applyTimeout)
	defer cancel()

	discount := h.discountFor(ctx, userID)

	// The price comes from the pricing page's monthly/annual choice. It is validated
	// against the configured list inside the service — never trusted as sent.
	url, err := h.billing.CheckoutURL(ctx, userID, c.Query("price"), discount)
	if err != nil {
		// Either checkout is unconfigured, or the provider refused. Neither is something a
		// candidate can act on, and a 404 lets the surface omit the offer rather than render
		// a broken one.
		log.Printf("billing: no checkout for user %d: %v", userID, err)
		return fiber.NewError(fiber.StatusNotFound, "checkout is not available")
	}
	return c.JSON(fiber.Map{"data": fiber.Map{
		"url": url,
		// Which offer was applied, so the page can say so rather than leaving the buyer to
		// work it out from the amount on the provider's page. Empty when there was none.
		"discount_percent": discount.PercentOff,
		"discount_source":  discount.Label,
	}})
}

// discountFor reads the ONE discount this checkout may carry, and writes nothing.
//
// This route is a GET, so it must have no side effects — and `SameSite=Lax` is exactly why
// that is a security rule here rather than a stylistic one: the session cookie IS sent on a
// cross-site top-level navigation, so any page linking here would act as this visitor. An
// earlier draft redeemed a code from the query string on this path, which meant a link
// could burn somebody's one lifetime redemption. Redeeming now lives on its own POST
// (`/me/promo/redeem`) and is durable, so what reaches here is only what the account
// already holds.
//
// It cannot fail the checkout either. A discount somebody was offered but that we could not
// read is not a reason to refuse them the purchase, so a failure is logged and the sale
// goes ahead at list price.
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

// Subscription returns what the caller is paying and what has been charged.
//
// 404 when there is no subscription — the ordinary state for a free account, and the
// surface renders nothing rather than an error. The money is read from the provider on
// every request rather than mirrored here: a receipt list that has quietly missed a refund
// is worse than no receipt list.
func (h *billingHandlers) Subscription(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(c.Context(), applyTimeout)
	defer cancel()

	out, err := h.billing.SubscriptionOverview(ctx, userID)
	if err != nil {
		if !errors.Is(err, billing.ErrNoSubscription) {
			log.Printf("billing: could not read the subscription of user %d: %v", userID, err)
		}
		return fiber.NewError(fiber.StatusNotFound, "no subscription")
	}
	return c.JSON(fiber.Map{"data": out})
}

// Manage returns the provider's own page where this subscriber changes their card or
// cancels.
//
// Deleting a freehire account does NOT cancel a subscription, and the delete surface says
// so and links here. The destination is generated by the provider per visit — we never
// compose it, because a page we wrote would be one more thing that can disagree with what
// actually happened to the money.
func (h *billingHandlers) Manage(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(c.Context(), applyTimeout)
	defer cancel()

	url, err := h.billing.ManagementURL(ctx, userID)
	if err != nil {
		// No subscription, or the provider is unreachable. Either way there is nothing to
		// manage right now, and the surface omits the link.
		if !errors.Is(err, billing.ErrNoSubscription) {
			log.Printf("billing: no management URL for user %d: %v", userID, err)
		}
		return fiber.NewError(fiber.StatusNotFound, "no subscription to manage")
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"url": url}})
}
