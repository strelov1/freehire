package billing

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

// ErrDisabled is returned by every operation when billing is not configured. Callers render
// it as 404, not 500: an unconfigured optional subsystem should look absent.
var ErrDisabled = errors.New("billing: not configured")

// ErrUnknownSubscriber is returned when an event names nobody we can resolve to an account.
// It is a reportable outcome, not a failure.
var ErrUnknownSubscriber = errors.New("billing: event names no account of ours")

// ErrNoSubscription is returned when an account has never transacted, so there is no
// customer to ask the provider about.
var ErrNoSubscription = errors.New("billing: account has no subscription")

// engine is billing's provider-generic half: accept a delivery, record it, and derive one
// provider's source column from that provider's current state.
//
// A webhook is a signal, never a fact, so applying one re-reads the provider. The source is
// derived whole rather than adjusted, so a repeat is free and out-of-order delivery is harmless.
// Record before applying, because the acknowledgement claims durability and nothing else.
// No transaction is needed: the recorded event IS the durable state and applying is idempotent,
// so a crash between the two costs a reconciler pass.
type engine struct {
	p provider
	q *db.Queries
}

// Enabled reports whether this provider is configured.
func (e *engine) Enabled() bool { return e.p.enabled() }

// SignatureHeader is the header this provider signs its deliveries with; the two providers
// use different names for the same scheme.
func (e *engine) SignatureHeader() string { return e.p.signatureHeader() }

// Accept verifies a delivery's signature and parses it. It is separate from Record so the
// handler cannot record bytes it has not authenticated.
func (e *engine) Accept(raw []byte, signature string, now time.Time) (Event, error) {
	if !e.Enabled() {
		return Event{}, ErrDisabled
	}
	return e.p.accept(raw, signature, now)
}

// Record stores an accepted event, once, and reports whether this delivery was the first.
// A false second return is a REDELIVERY, not an error: the provider retries anything without a
// 2xx and reuses the event id, so the caller answers 200.
//
// It also BINDS the account to what the provider needs to reach it later: the customer id for
// Stripe (a scheduled re-check cannot name anybody without it), nothing for RevenueCat.
func (e *engine) Record(ctx context.Context, ev Event) (rowID int64, recorded bool, err error) {
	if !e.Enabled() {
		return 0, false, ErrDisabled
	}

	userID, hasUser := e.p.account(ctx, ev)

	params := db.InsertBillingEventParams{
		Provider:  e.p.name(),
		EventID:   ev.ID,
		AppUserID: ev.CustomerID,
		EventType: ev.Type,
		Payload:   ev.Payload,
	}
	if hasUser {
		params.UserID = pgtype.Int8{Int64: userID, Valid: true}
	}

	rowID, err = e.q.InsertBillingEvent(ctx, params)
	if errors.Is(err, pgx.ErrNoRows) {
		// ON CONFLICT DO NOTHING returned nothing: we already hold this event.
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("billing: recording event %q: %w", ev.ID, err)
	}

	if hasUser {
		// A failed binding does not fail the delivery (the event IS stored), but it is logged: the
		// binding is how a scheduled re-check reaches this account. The recorded row still carries
		// the user, so the reconciler can work without it.
		if err := e.p.bind(ctx, userID, ev); err != nil {
			log.Printf("billing: %s event %s recorded but user %d not bound: %v",
				e.p.name(), ev.ID, userID, err)
		}
	}
	return rowID, true, nil
}

// Apply brings one recorded event's account up to date and marks the event processed. The order
// is load-bearing: a crash between sync and stamp leaves the event unprocessed and the
// reconciler repeats a harmless sync; stamping first would drop the work.
func (e *engine) Apply(ctx context.Context, rowID int64, ev Event) error {
	userID, ok := e.p.account(ctx, ev)
	if !ok {
		return fmt.Errorf("%w: customer %q, ref %q", ErrUnknownSubscriber, ev.CustomerID, ev.UserRef)
	}

	// Repair the binding BEFORE syncing: the sync needs it and the event has it. Otherwise the
	// sync finds no customer, gets ErrNoSubscription, and the worker stamps a paid subscription
	// as done. Unlike in Record, a failure here is fatal.
	if err := e.p.bind(ctx, userID, ev); err != nil {
		return fmt.Errorf("billing: binding user %d for %s event %s: %w", userID, e.p.name(), ev.ID, err)
	}

	// e.sync, not SyncUser: the stranger check stops a bulk pass from enrolling accounts WITHOUT
	// evidence, but a signed delivery naming this account is the evidence, and refusing it would
	// stamp a paid purchase processed having conferred nothing.
	if err := e.sync(ctx, userID); err != nil {
		return err
	}
	return e.q.MarkBillingEventProcessed(ctx, rowID)
}

// MarkProcessed stamps an event without applying it, for events nobody can ever resolve (not
// metered, or created outside this integration) where retrying forever is the alternative.
func (e *engine) MarkProcessed(ctx context.Context, rowID int64) error {
	return e.q.MarkBillingEventProcessed(ctx, rowID)
}

// PendingEvents returns THIS PROVIDER'S events recorded but never applied, oldest first: the
// reconciler's first pass. The scoping is a correctness requirement: the providers address
// accounts differently, so the other's row would write the wrong column and be stamped done.
func (e *engine) PendingEvents(ctx context.Context, max int32) ([]db.ListUnprocessedBillingEventsRow, error) {
	if !e.Enabled() {
		return nil, ErrDisabled
	}
	return e.q.ListUnprocessedBillingEvents(ctx, db.ListUnprocessedBillingEventsParams{
		Provider: e.p.name(),
		MaxRows:  max,
	})
}

// SubscribersNearExpiry returns accounts whose entitlement from this provider falls within
// window either side of now: the reconciler's pass for renewals whose webhook never arrived.
func (e *engine) SubscribersNearExpiry(ctx context.Context, window time.Duration, max int32) ([]int64, error) {
	if !e.Enabled() {
		return nil, ErrDisabled
	}
	now := time.Now().UTC()
	return e.p.dueSoon(ctx, now.Add(-window), now.Add(window), max)
}

// SyncUser reads the provider's current state for one account and writes THIS PROVIDER'S
// source column from it; users.pro_until follows, derived by the schema.
//
// It writes the provider's own column, never the derived one: "no subscription" means "I confer
// nothing", not "not Pro" (the account may hold a store subscription or a manual grant). The
// source is derived WHOLE every time, so it is idempotent and order-independent, and a refund,
// cancellation or failed card needs no code of its own.
func (e *engine) SyncUser(ctx context.Context, userID int64) error {
	if !e.Enabled() {
		return ErrDisabled
	}

	// A stranger is not asked about: the reconciler and event replay reach accounts in bulk (see
	// provider.knows for what asking costs).
	known, err := e.p.knows(ctx, userID)
	if err != nil {
		return err
	}
	if !known {
		return fmt.Errorf("%w: user %d", ErrNoSubscription, userID)
	}
	return e.sync(ctx, userID)
}

// SyncCaller is SyncUser for an account asking about ITSELF; it skips the stranger check. That
// check stops a bulk pass enrolling every account with RevenueCat, not one authenticated,
// rate-limited caller who just bought something. A first-time buyer has no recorded event and a
// NULL source column, so the guard would turn "your webhook was lost" into "you are not a
// subscriber".
func (e *engine) SyncCaller(ctx context.Context, userID int64) error {
	if !e.Enabled() {
		return ErrDisabled
	}
	return e.sync(ctx, userID)
}

// sync is the shared body: read the provider whole, write this provider's source columns. One
// read for every tier and one write, so there are no disagreeing reads and no instant where an
// upgrading subscriber holds neither tier.
func (e *engine) sync(ctx context.Context, userID int64) error {
	ent, err := e.p.reach(ctx, userID)
	if err != nil {
		return err
	}

	if err := e.p.store(ctx, userID, ent); err != nil {
		return fmt.Errorf("billing: writing the %s sources for user %d: %w", e.p.name(), userID, err)
	}
	return nil
}

// nullable converts a reach to the pgtype the setters take; a zero time is NULL (this provider
// confers nothing).
func nullable(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: !t.IsZero()}
}

// stamp converts a time to the pgtype the queries take.
func stamp(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// Service is Stripe: the shared engine plus the web-only surface (checkout, portal, prices,
// receipts). Those have no RevenueCat counterpart, so they live here, not behind the provider
// seam; store subscriptions are managed entirely inside the App Store or Google Play.
type Service struct {
	*engine
	cfg    Config
	q      *db.Queries
	client *client
	prices priceCache
}

// New constructs a Service. It never fails: an unconfigured Service is disabled and refuses
// every operation with ErrDisabled.
func New(cfg Config, q *db.Queries) *Service {
	return NewWithBase(cfg, q, apiBaseURL)
}

// NewWithBase is New pointed at a different provider base URL, for tests with a stub. It dials
// WITHOUT the SSRF guard, which would refuse the loopback address stubs listen on; that is safe
// because the guard defends against caller-supplied destinations and the production base URL
// is a constant. New still uses the guarded client.
func NewWithBase(cfg Config, q *db.Queries, baseURL string) *Service {
	s := &Service{cfg: cfg, q: q}
	if cfg.Enabled() {
		if baseURL == apiBaseURL {
			s.client = newProviderClient(cfg.APIKey)
		} else {
			s.client = newClient(cfg.APIKey, baseURL, &http.Client{Timeout: requestTimeout})
		}
	}
	s.engine = &engine{p: &stripeProvider{cfg: cfg, q: q, client: s.client}, q: q}
	return s
}

// There is deliberately no Config() accessor: Config carries the secret key and webhook secret,
// and a getter invites serialising them by accident. Enabled() is the answer outsiders need.

// CheckoutURL is where this account buys, upgrades or downgrades a subscription.
//
// The account id comes from the caller's session, never the request: it decides who gets
// charged and who becomes Pro. A customer with an entitling subscription is never sent to a
// new Checkout Session for a different price; their subscription is changed in place (see
// decideCheckoutTarget).
//
// The returned Discount is what was actually APPLIED: an in-place update never applies one, and
// echoing the input would tell a subscriber a discount was used when their bill never showed it.
func (s *Service) CheckoutURL(ctx context.Context, userID int64, priceID string, discount Discount) (string, Discount, error) {
	if !s.cfg.CanCheckout() {
		return "", Discount{}, ErrNoCheckout
	}

	// A price the browser named must be one we actually sell, or a caller could name a free one.
	if priceID == "" {
		priceID = s.cfg.CheckoutPrice()
	} else if !s.cfg.Sells(priceID) {
		return "", Discount{}, fmt.Errorf("%w: price %q is not offered", ErrNoCheckout, priceID)
	}

	// Promo and invite discounts are a consumer offer for Pro and Ultra; the API product is
	// sold at a flat price, so no discount reaches an API checkout.
	if s.cfg.isAPIPrice(priceID) {
		discount = Discount{}
	}

	// An existing customer is reused so a second purchase cannot create a second customer. A
	// failure to read the binding is refused, not treated as "no customer": a transient DB error
	// would otherwise open a checkout creating a second customer and subscription.
	existing, err := s.q.GetStripeCustomerID(ctx, userID)
	if err != nil {
		return "", Discount{}, fmt.Errorf("billing: reading the Stripe customer of user %d: %w", userID, err)
	}
	var customerID string
	if existing.Valid {
		customerID = existing.String
	}
	if customerID == "" {
		// Bind before checkout so a failed first payment is still reachable by its owner.
		// newCustomerFor re-reads the write-once binding if another checkout wins the race.
		customerID, err = s.newCustomerFor(ctx, userID, fmt.Sprintf("checkout_user_%d", userID))
		if err != nil {
			return "", Discount{}, err
		}
	}

	// Fail the whole call on a read error rather than fall back to a new checkout, which
	// would open a second subscription beside a live one.
	current, err := s.client.subscriberState(ctx, customerID)
	if err != nil {
		return "", Discount{}, fmt.Errorf("billing: reading the subscriptions of user %d: %w", userID, err)
	}
	// Within the requested price's own product: an API purchase must never change a Pro/Ultra
	// subscription in place, nor the reverse (see Config.checkoutFamily).
	familyA, familyB := s.cfg.checkoutFamily(priceID)
	target := decideCheckoutTarget(current, priceID, familyA, familyB)
	switch {
	case target.AlreadyOnPrice:
		// ReturnURL, not SuccessURL: nothing was bought, and SuccessURL would count a sale on
		// every re-open of the current price.
		return s.cfg.ReturnURL(), Discount{}, nil
	case target.Ambiguous:
		return "", Discount{}, fmt.Errorf(
			"billing: user %d's subscription carries more than one item; refusing to guess which one to change", userID)
	case target.SubscriptionID != "":
		// A fresh key per call: it guards one call's own low-level HTTP retries against
		// double-billing a proration, not repeated clicks (re-setting the price changes nothing).
		idempotencyKey := uuid.NewString()
		if err := s.client.updateSubscriptionPrice(ctx, target.SubscriptionID, target.ItemID, priceID, idempotencyKey); err != nil {
			return "", Discount{}, fmt.Errorf("billing: upgrading user %d to price %q: %w", userID, priceID, err)
		}
		// SuccessURL: an in-place upgrade IS a purchase; ReturnURL left every Pro-to-Ultra sale
		// looking abandoned.
		return s.cfg.SuccessURL(), Discount{}, nil
	}
	// No entitling subscription (free account or lapsed subscriber): open a checkout.

	// A coupon is minted per session from the percentage this account is owed. A mint failure is
	// fatal on purpose: sending someone to pay full price for an offer they were shown is worse.
	var couponID string
	if !discount.none() {
		couponID, err = s.spendableCoupon(ctx, discount)
		if err != nil {
			return "", Discount{}, fmt.Errorf("billing: minting a discount for user %d: %w", userID, err)
		}
	}

	// Success and cancel differ by one marker; see Config.SuccessURL.
	url, err := s.client.createCheckoutSession(ctx, userID, "",
		priceID, s.cfg.SuccessURL(), s.cfg.ReturnURL(), customerID, couponID)
	if err != nil {
		return "", Discount{}, err
	}
	return url, discount, nil
}

// spendableCoupon is the coupon this checkout may carry.
//
// The provider spends a coupon's one redemption on the first checkout that TRIES to pay,
// declined or not, so a spent coupon is replaced by the coupon keyed on it. The replacements
// form one chain with at most one usable link, so two open checkouts cannot both be discounted.
//
// shortcut: a spent coupon may be one a SUCCESSFUL purchase used and is replaced anyway; no
// wider than the stable key's 24-hour idempotency window already allows. Close both by
// recording when an offer is consumed if repeat discounted purchases show up.
func (s *Service) spendableCoupon(ctx context.Context, discount Discount) (string, error) {
	key := discount.Key
	for range maxCouponChain {
		id, err := s.client.createCoupon(ctx, discount.PercentOff, discount.Label, key)
		if err != nil {
			return "", err
		}
		valid, err := s.client.couponValid(ctx, id)
		if err != nil || valid {
			return id, err
		}
		key = discount.Key + "_after_" + id
	}
	return "", fmt.Errorf("billing: %d coupons in a row were spent for key %q", maxCouponChain, discount.Key)
}

// maxCouponChain bounds how many spent replacements one checkout walks past; each is a
// declined payment, and a dozen in a day is a card being tested.
const maxCouponChain = 12

// HasCollectedAtLeast reports whether this customer has had an invoice collecting at least
// minCents. The referral reward must be earned by ENOUGH money: an invitee on a large discount
// can pay less than the reward is worth. It takes a customer id because the worker already holds
// the binding and uses it to decide whether an API call is worth making.
func (s *Service) HasCollectedAtLeast(ctx context.Context, customerID string, minCents int64) (bool, error) {
	if !s.cfg.Enabled() {
		return false, ErrDisabled
	}
	return s.client.hasCollectedAtLeast(ctx, customerID, minCents)
}

// CreditAccount places credit on this account's provider customer, creating and binding one if
// the account never bought anything. The binding uses the write-once setter, so it cannot
// repoint an account; if it loses a race, our new customer is abandoned and the credit goes to
// the winner. cents is POSITIVE; the provider's sign convention is applied once, in the client.
func (s *Service) CreditAccount(ctx context.Context, userID, cents int64, idempotencyKey string) error {
	if !s.cfg.Enabled() {
		return ErrDisabled
	}
	if cents <= 0 {
		return fmt.Errorf("billing: refusing to credit user %d with %d cents", userID, cents)
	}

	currency, err := s.checkoutCurrency(ctx)
	if err != nil {
		return err
	}

	customerID, err := s.stripe().customerOf(ctx, userID)
	if errors.Is(err, ErrNoSubscription) {
		customerID, err = s.newCustomerFor(ctx, userID, idempotencyKey)
	}
	if err != nil {
		return err
	}

	return s.client.creditCustomerBalance(ctx, customerID, cents, currency,
		"freehire invite reward", idempotencyKey)
}

// newCustomerFor registers an account with the provider and binds it. The idempotency key is
// suffixed, not reused: the same key on two endpoints would replay the first response.
func (s *Service) newCustomerFor(ctx context.Context, userID int64, idempotencyKey string) (string, error) {
	var email string
	if addr, emailErr := s.q.UserEmail(ctx, userID); emailErr == nil {
		email = addr
	}

	customerID, err := s.client.createCustomer(ctx, userID, email, idempotencyKey+"_customer")
	if err != nil {
		return "", fmt.Errorf("billing: creating a customer for user %d: %w", userID, err)
	}

	if err := s.q.SetStripeCustomerID(ctx, db.SetStripeCustomerIDParams{
		ID:               userID,
		StripeCustomerID: customerID,
	}); err != nil {
		return "", fmt.Errorf("billing: binding user %d to a new customer: %w", userID, err)
	}

	// Re-read: the setter is write-once, so a concurrent purchase may have bound a different
	// customer first, and crediting ours would strand the reward.
	return s.stripe().customerOf(ctx, userID)
}

// CheckoutPriceCents is what a new subscriber is sold, in the smallest currency unit. It reads
// the provider's price through the public price cache rather than storing a copy; a wrong
// answer would misprice a referral reward that can never be recomputed.
func (s *Service) CheckoutPriceCents(ctx context.Context) (int64, error) {
	price, err := s.checkoutPrice(ctx)
	if err != nil {
		return 0, err
	}
	return price.AmountCents, nil
}

// checkoutCurrency is the currency the sold price is denominated in; a credit must match the
// invoices it reduces.
func (s *Service) checkoutCurrency(ctx context.Context) (string, error) {
	price, err := s.checkoutPrice(ctx)
	if err != nil {
		return "", err
	}
	return price.Currency, nil
}

// checkoutPrice is the configured sale price as the provider reports it. An unreadable price is
// an error, not a default: PublicPrices returns an empty list when the provider is unreachable,
// and treating that as zero would silently pay a referral reward of nothing.
func (s *Service) checkoutPrice(ctx context.Context) (PublicPrice, error) {
	wanted := s.cfg.CheckoutPrice()
	for _, price := range s.PublicPrices(ctx) {
		if price.ID == wanted {
			return price, nil
		}
	}
	return PublicPrice{}, fmt.Errorf("billing: cannot read price %q from the provider", wanted)
}

// ManagementURL is the provider's own page where this subscriber changes their card or cancels.
// It is short-lived, so it is fetched on demand, and it is the provider's page so a cancellation
// flow of ours cannot disagree with what happened to the money.
func (s *Service) ManagementURL(ctx context.Context, userID int64) (string, error) {
	if !s.cfg.CanCheckout() {
		return "", ErrNoCheckout
	}
	customer, err := s.stripe().customerOf(ctx, userID)
	if err != nil {
		return "", err
	}
	return s.client.createPortalSession(ctx, customer, s.cfg.ReturnURL())
}

// stripe is the engine's provider, narrowed back to the concrete type for the surfaces that
// are Stripe's alone. The assertion cannot fail: this type's only constructor builds it.
func (s *Service) stripe() *stripeProvider { return s.p.(*stripeProvider) }
