package billing

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

// ErrDisabled is returned by every operation when billing is not configured. Callers render
// it as 404 rather than 500: an unconfigured optional subsystem should look absent, not
// broken.
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
// Every rule it holds is true of both providers, which is why it holds them once. A webhook
// is a signal and never a fact, so applying one re-reads the provider. The source is derived
// whole rather than adjusted, so a repeat is free and an out-of-order delivery is harmless.
// Record before applying, because the acknowledgement claims durability and nothing else.
//
// It needs no pool and takes no transaction. Recording is one insert and applying is one read
// followed by one update, and neither has an invariant spanning both — the recorded event IS
// the durable state, and applying it is idempotent by construction, so a crash between the
// two costs a reconciler pass rather than correctness.
type engine struct {
	p provider
	q *db.Queries
}

// Enabled reports whether this provider is configured.
func (e *engine) Enabled() bool { return e.p.enabled() }

// SignatureHeader is the header this provider signs its deliveries with. The handler asks
// rather than knows: the two providers use different header names for the same scheme, and a
// package-level constant could only ever be right for one of them.
func (e *engine) SignatureHeader() string { return e.p.signatureHeader() }

// Accept verifies a delivery's signature and parses it. It is deliberately separate from
// Record so the handler cannot record something it has not authenticated: the only way to
// obtain an Event is to have verified the bytes it came from.
func (e *engine) Accept(raw []byte, signature string, now time.Time) (Event, error) {
	if !e.Enabled() {
		return Event{}, ErrDisabled
	}
	return e.p.accept(raw, signature, now)
}

// Record stores an accepted event, once, and reports whether this delivery was the first.
//
// A false second return is a REDELIVERY, not an error: the provider retries anything it did
// not get a 2xx for and reuses the event id, so a duplicate is the normal case and the caller
// answers 200 to it.
//
// It also BINDS the account to whatever the provider needs to be reachable by later. For
// Stripe that is the customer id, without which a scheduled re-check cannot name anybody; for
// RevenueCat it is nothing, because the subscriber already is our own account id.
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
		// A failed binding does not fail the delivery: the event IS stored, which is all the
		// acknowledgement claims. But it is not nothing either — the binding is how a
		// scheduled re-check reaches this account, so losing it silently is how an account
		// stops being reconcilable without anyone noticing. Logged, and the recorded row
		// still carries the user, so the reconciler can work without it.
		if err := e.p.bind(ctx, userID, ev); err != nil {
			log.Printf("billing: %s event %s recorded but user %d not bound: %v",
				e.p.name(), ev.ID, userID, err)
		}
	}
	return rowID, true, nil
}

// Apply brings one recorded event's account up to date and marks the event processed.
//
// The order is load-bearing. Syncing first and stamping second means a crash in between
// leaves the event unprocessed and the reconciler repeats a sync that changes nothing — the
// harmless direction. Stamping first would drop the work on the floor.
func (e *engine) Apply(ctx context.Context, rowID int64, ev Event) error {
	userID, ok := e.p.account(ctx, ev)
	if !ok {
		return fmt.Errorf("%w: customer %q, ref %q", ErrUnknownSubscriber, ev.CustomerID, ev.UserRef)
	}

	// REPAIR THE BINDING BEFORE SYNCING, because the sync needs it and the event has it.
	//
	// Without this the replay is worse than useless: the user resolves from the event, then
	// the sync looks the customer up in a column nothing ever wrote, gets ErrNoSubscription,
	// and the worker reads that as "nothing to apply" and stamps the row. A paid subscription
	// marked done, forever. Binding here makes the retry self-healing — which is what a retry
	// is for. Unlike in Record, a failure here IS fatal: the sync that follows depends on it.
	if err := e.p.bind(ctx, userID, ev); err != nil {
		return fmt.Errorf("billing: binding user %d for %s event %s: %w", userID, e.p.name(), ev.ID, err)
	}

	// e.sync, not SyncUser: the stranger check does not bind this path, and binding it would
	// be the same mistake it made once already. The guard exists to stop a pass that reaches
	// accounts WITHOUT evidence — a walk over users — from enrolling every one of them with the
	// provider. Applying holds evidence: a signed delivery naming this account. That delivery
	// IS the footprint, and refusing to act on it because the footprint has not been written
	// yet would stamp a paid purchase processed having conferred nothing.
	if err := e.sync(ctx, userID); err != nil {
		return err
	}
	return e.q.MarkBillingEventProcessed(ctx, rowID)
}

// MarkProcessed stamps an event without applying it. It exists for the cases nobody can ever
// resolve — an event about something we do not meter, or an object created outside this
// integration — where retrying forever would be the only alternative.
func (e *engine) MarkProcessed(ctx context.Context, rowID int64) error {
	return e.q.MarkBillingEventProcessed(ctx, rowID)
}

// PendingEvents returns THIS PROVIDER'S events recorded but never applied, oldest first. It
// is the reconciler's first pass, and the reason a failure to apply inside the webhook
// handler costs nothing.
//
// The scoping is a correctness requirement, not a filter for tidiness: applying an event
// means re-reading the subscriber and writing one source column, and the two providers
// address accounts differently. Handed the other's row, a pass would write the wrong column
// and stamp the row processed — a purchase marked done having conferred nothing.
func (e *engine) PendingEvents(ctx context.Context, max int32) ([]db.ListUnprocessedBillingEventsRow, error) {
	if !e.Enabled() {
		return nil, ErrDisabled
	}
	return e.q.ListUnprocessedBillingEvents(ctx, db.ListUnprocessedBillingEventsParams{
		Provider: e.p.name(),
		MaxRows:  max,
	})
}

// SubscribersNearExpiry returns the accounts whose entitlement from this provider falls
// within window either side of now — the reconciler's second pass, which repairs a renewal
// whose webhook was never delivered at all.
func (e *engine) SubscribersNearExpiry(ctx context.Context, window time.Duration, max int32) ([]int64, error) {
	if !e.Enabled() {
		return nil, ErrDisabled
	}
	now := time.Now().UTC()
	return e.p.dueSoon(ctx, now.Add(-window), now.Add(window), max)
}

// SyncUser reads the provider's current state for one account and writes THIS PROVIDER'S
// source column from it. users.pro_until follows, derived by the schema.
//
// The column written is the provider's own and never the derived one. A provider reporting no
// subscription is saying "I confer nothing", not "this account is not Pro" — the account may
// hold a store subscription or a manual grant, and before migration 0135 this write would
// have revoked either of them without a trace.
//
// This is the only place the source is derived, and it is derived WHOLE every time rather
// than adjusted. That is what makes it idempotent, order-independent and safe to repeat:
// applying the same truth twice changes nothing, and a refund, a cancellation or a failed
// card needs no code of its own because it is already reflected in what we read.
func (e *engine) SyncUser(ctx context.Context, userID int64) error {
	if !e.Enabled() {
		return ErrDisabled
	}

	// A stranger is not asked about. This is the path the reconciler and the event replay
	// take, and both can reach accounts in bulk — see provider.knows for what asking costs.
	known, err := e.p.knows(ctx, userID)
	if err != nil {
		return err
	}
	if !known {
		return fmt.Errorf("%w: user %d", ErrNoSubscription, userID)
	}
	return e.sync(ctx, userID)
}

// SyncCaller is SyncUser for an account asking about ITSELF, and it deliberately skips the
// stranger check.
//
// The check exists to stop a bulk pass enrolling every account with RevenueCat. One
// authenticated, rate-limited caller asking about their own id is not that: they have just
// bought something, so the provider's own device SDK created that subscriber before the app
// ever reached us.
//
// Without this distinction the route built to close the lost-webhook window refused the only
// case it was built for. A first-time buyer has no recorded event and a NULL source column —
// that is what "first" means — so the guard turned "your webhook was lost" into "you are not
// a subscriber", and neither reconciler pass would ever have found them either.
func (e *engine) SyncCaller(ctx context.Context, userID int64) error {
	if !e.Enabled() {
		return ErrDisabled
	}
	return e.sync(ctx, userID)
}

// sync is the shared body: read the provider whole, write this provider's source column.
func (e *engine) sync(ctx context.Context, userID int64) error {
	until, err := e.p.reach(ctx, userID)
	if err != nil {
		return err
	}

	if err := e.p.store(ctx, userID, pgtype.Timestamptz{Time: until, Valid: !until.IsZero()}); err != nil {
		return fmt.Errorf("billing: writing the %s source for user %d: %w", e.p.name(), userID, err)
	}
	return nil
}

// stamp converts a time to the pgtype the queries take.
func stamp(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// Service is Stripe: the shared engine, plus the surface that exists only where a card is
// entered on the web — a checkout, a management portal, prices and receipts.
//
// None of those has a RevenueCat counterpart, which is why they live on this type rather than
// behind the provider seam. A store subscription is bought, changed, cancelled and refunded
// inside the App Store or Google Play, where we have no API and no business having one.
type Service struct {
	*engine
	cfg    Config
	q      *db.Queries
	client *client
	prices priceCache
}

// New constructs a Service. It never fails: an unconfigured Service reports itself disabled
// and refuses every operation with ErrDisabled.
func New(cfg Config, q *db.Queries) *Service {
	return NewWithBase(cfg, q, apiBaseURL)
}

// NewWithBase is New pointed at a different provider base URL, for tests that stand a stub in
// front of it.
//
// It dials WITHOUT the SSRF guard, because the guard would refuse the loopback address every
// stub server listens on. That costs nothing real: the guard defends against a
// CALLER-SUPPLIED destination, and there is none here — the production base URL is a
// constant. New still uses the guarded client, so nothing in production takes this path.
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

// There is deliberately no Config() accessor. Config carries the secret key and the webhook
// secret, and an exported getter for them is a way for a future surface to serialise them by
// accident. Everything outside this package needs an answer about billing, not the
// credentials it was derived from — Enabled() is that answer.

// CheckoutURL is where this account buys Pro.
//
// The account's id is taken from the caller's session and never from the request: it decides
// who gets charged and who becomes Pro, and a value the browser composes is a value the
// browser can change.
func (s *Service) CheckoutURL(ctx context.Context, userID int64, priceID string) (string, error) {
	if !s.cfg.CanCheckout() {
		return "", ErrNoCheckout
	}

	// A price the browser named must be one we actually sell. The parameter arrives from the
	// client, and a caller who could name any price could name one costing nothing.
	if priceID == "" {
		priceID = s.cfg.CheckoutPrice()
	} else if !s.cfg.Sells(priceID) {
		return "", fmt.Errorf("%w: price %q is not offered", ErrNoCheckout, priceID)
	}

	// An existing customer is reused so a second purchase cannot create a second customer for
	// one person — which would leave two subscriptions nobody sums.
	existing, err := s.q.GetStripeCustomerID(ctx, userID)
	var customerID string
	if err == nil && existing.Valid {
		customerID = existing.String
	}

	// Pre-fill the address we already hold, so a buyer does not retype what we asked them for
	// at sign-up. Only for a NEW customer: the provider refuses an email alongside an existing
	// one, since that customer already has theirs.
	var email string
	if customerID == "" {
		if addr, emailErr := s.q.UserEmail(ctx, userID); emailErr == nil {
			email = addr
		}
	}

	return s.client.createCheckoutSession(ctx, userID, email,
		priceID, s.cfg.ReturnURL(), s.cfg.ReturnURL(), customerID)
}

// ManagementURL is the provider's own page where this subscriber changes their card or
// cancels.
//
// It is generated per visit and short-lived, which is why it is fetched when asked for rather
// than stored. And it is THEIR page: a cancellation flow of our own would be one more thing
// that can disagree with what actually happened to the money.
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
