package billing

import (
	"context"
	"errors"
	"fmt"
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

// Service is the whole of billing: accept a delivery, record it, and derive
// users.pro_until from the provider's current state.
//
// It needs no pool and takes no transaction. Recording is one insert and applying is one
// read followed by one update, and neither has an invariant spanning both — the recorded
// event IS the durable state, and applying it is idempotent by construction, so a crash
// between the two costs a reconciler pass rather than correctness.
type Service struct {
	cfg    Config
	q      *db.Queries
	client *client
}

// New constructs a Service. It never fails: an unconfigured Service reports itself disabled
// and refuses every operation with ErrDisabled.
func New(cfg Config, q *db.Queries) *Service {
	return NewWithBase(cfg, q, apiBaseURL)
}

// NewWithBase is New pointed at a different provider base URL, for tests that stand a stub
// in front of it.
//
// It dials WITHOUT the SSRF guard, because the guard would refuse the loopback address
// every stub server listens on. That costs nothing real: the guard defends against a
// CALLER-SUPPLIED destination, and there is none here — the production base URL is a
// constant. New still uses the guarded client, so nothing in production takes this path.
func NewWithBase(cfg Config, q *db.Queries, baseURL string) *Service {
	s := &Service{cfg: cfg, q: q}
	if !cfg.Enabled() {
		return s
	}
	if baseURL == apiBaseURL {
		s.client = newProviderClient(cfg.APIKey)
	} else {
		s.client = newClient(cfg.APIKey, baseURL, &http.Client{Timeout: requestTimeout})
	}
	return s
}

// Enabled reports whether billing is configured.
func (s *Service) Enabled() bool { return s.cfg.Enabled() }

// Config exposes the configuration for the surfaces that need it.
func (s *Service) Config() Config { return s.cfg }

// Accept verifies a delivery's signature and parses it. It is deliberately separate from
// Record so the handler cannot record something it has not authenticated: the only way to
// obtain an Event is to have verified the bytes it came from.
func (s *Service) Accept(raw []byte, signature string, now time.Time) (Event, error) {
	if !s.Enabled() {
		return Event{}, ErrDisabled
	}
	if err := verifySignature(raw, signature, s.cfg.WebhookSecret, now); err != nil {
		return Event{}, err
	}
	return parseEvent(raw)
}

// Record stores an accepted event, once, and reports whether this delivery was the first.
//
// A false second return is a REDELIVERY, not an error: the provider retries anything it did
// not get a 2xx for and reuses the event id, so a duplicate is the normal case and the
// caller answers 200 to it.
//
// It also BINDS the account to the provider's customer when the event carries both. That
// binding is what makes the reconciler possible at all: a webhook names a customer, but a
// scheduled re-check starts from a user and has to name one.
func (s *Service) Record(ctx context.Context, ev Event) (rowID int64, recorded bool, err error) {
	if !s.Enabled() {
		return 0, false, ErrDisabled
	}

	userID, hasUser := s.resolveUser(ctx, ev)

	params := db.InsertBillingEventParams{
		Provider:  Provider,
		EventID:   ev.ID,
		AppUserID: ev.CustomerID,
		EventType: ev.Type,
		Payload:   ev.Payload,
	}
	if hasUser {
		params.UserID = pgtype.Int8{Int64: userID, Valid: true}
	}

	rowID, err = s.q.InsertBillingEvent(ctx, params)
	if errors.Is(err, pgx.ErrNoRows) {
		// ON CONFLICT DO NOTHING returned nothing: we already hold this event.
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("billing: recording event %q: %w", ev.ID, err)
	}

	if hasUser && ev.CustomerID != "" {
		if bindErr := s.q.SetStripeCustomerID(ctx, db.SetStripeCustomerIDParams{
			ID: userID, StripeCustomerID: ev.CustomerID,
		}); bindErr != nil {
			// The event is stored, which is what the acknowledgement claims. A failed
			// binding costs the reconciler this account until the next event, not the money.
			return rowID, true, nil
		}
	}
	return rowID, true, nil
}

// resolveUser finds which account an event is about.
//
// Two routes, and the order matters. The customer binding is authoritative for everything
// after the first purchase. The account reference the provider echoes back is what covers
// the first purchase itself, where no binding exists yet — a checkout completion carries it
// precisely because nothing else could.
func (s *Service) resolveUser(ctx context.Context, ev Event) (int64, bool) {
	if ev.CustomerID != "" {
		if id, err := s.q.GetUserIDByStripeCustomer(ctx, ev.CustomerID); err == nil {
			return id, true
		}
	}
	if id, ok := userIDFromRef(ev.UserRef); ok {
		return id, true
	}
	return 0, false
}

// Apply brings one recorded event's account up to date and marks the event processed.
//
// The order is load-bearing. Syncing first and stamping second means a crash in between
// leaves the event unprocessed and the reconciler repeats a sync that changes nothing — the
// harmless direction. Stamping first would drop the work on the floor.
func (s *Service) Apply(ctx context.Context, rowID int64, ev Event) error {
	userID, ok := s.resolveUser(ctx, ev)
	if !ok {
		return fmt.Errorf("%w: customer %q, ref %q", ErrUnknownSubscriber, ev.CustomerID, ev.UserRef)
	}
	if err := s.SyncUser(ctx, userID); err != nil {
		return err
	}
	return s.q.MarkBillingEventProcessed(ctx, rowID)
}

// MarkProcessed stamps an event without applying it. It exists for the cases nobody can
// ever resolve — an event about something we do not meter, or an object created outside
// this integration — where retrying forever would be the only alternative.
func (s *Service) MarkProcessed(ctx context.Context, rowID int64) error {
	return s.q.MarkBillingEventProcessed(ctx, rowID)
}

// PendingEvents returns events recorded but never applied, oldest first. It is the
// reconciler's first pass, and the reason a failure to apply inside the webhook handler
// costs nothing.
func (s *Service) PendingEvents(ctx context.Context, max int32) ([]db.ListUnprocessedBillingEventsRow, error) {
	if !s.Enabled() {
		return nil, ErrDisabled
	}
	return s.q.ListUnprocessedBillingEvents(ctx, max)
}

// SubscribersNearExpiry returns the accounts whose plan expiry falls within window either
// side of now — the reconciler's second pass, which repairs a renewal whose webhook was
// never delivered at all.
//
// It reaches them through the customer binding, so the candidate set is the accounts that
// have actually transacted rather than every account on the site.
func (s *Service) SubscribersNearExpiry(ctx context.Context, window time.Duration, max int32) ([]int64, error) {
	if !s.Enabled() {
		return nil, ErrDisabled
	}
	now := time.Now().UTC()
	rows, err := s.q.ListSubscribersNearProExpiryStripe(ctx, db.ListSubscribersNearProExpiryStripeParams{
		FromTime: stamp(now.Add(-window)),
		ToTime:   stamp(now.Add(window)),
		MaxRows:  max,
	})
	if err != nil {
		return nil, err
	}
	out := make([]int64, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.ID)
	}
	return out, nil
}

// stamp converts a time to the pgtype the queries take.
func stamp(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// SyncUser reads the provider's current state for one account and writes users.pro_until
// from it.
//
// This is the only place the column is derived, and it is derived WHOLE every time rather
// than adjusted. That is what makes it idempotent, order-independent and safe to repeat:
// applying the same truth twice changes nothing, and a refund, a cancellation or a failed
// card needs no code of its own because it is already reflected in what we read.
func (s *Service) SyncUser(ctx context.Context, userID int64) error {
	if !s.Enabled() {
		return ErrDisabled
	}

	customer, err := s.customerOf(ctx, userID)
	if err != nil {
		return err
	}

	sub, err := s.client.subscriberState(ctx, customer)
	if err != nil {
		return err
	}

	until := proUntilFrom(sub, s.cfg.Prices)
	proUntil := pgtype.Timestamptz{Time: until, Valid: !until.IsZero()}

	if err := s.q.SetProUntil(ctx, db.SetProUntilParams{ProUntil: proUntil, ID: userID}); err != nil {
		return fmt.Errorf("billing: writing pro_until for user %d: %w", userID, err)
	}
	return nil
}

// customerOf is the provider's customer for one account, or ErrNoSubscription when the
// account has never transacted.
func (s *Service) customerOf(ctx context.Context, userID int64) (string, error) {
	customer, err := s.q.GetStripeCustomerID(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("billing: reading the customer of user %d: %w", userID, err)
	}
	if !customer.Valid || customer.String == "" {
		return "", fmt.Errorf("%w: user %d", ErrNoSubscription, userID)
	}
	return customer.String, nil
}

// CheckoutURL is where this account buys Pro.
//
// The account's id is taken from the caller's session and never from the request: it
// decides who gets charged and who becomes Pro, and a value the browser composes is a value
// the browser can change.
func (s *Service) CheckoutURL(ctx context.Context, userID int64) (string, error) {
	if !s.cfg.CanCheckout() {
		return "", ErrNoCheckout
	}

	// An existing customer is reused so a second purchase cannot create a second customer
	// for one person — which would leave two subscriptions nobody sums.
	existing, err := s.q.GetStripeCustomerID(ctx, userID)
	var customerID string
	if err == nil && existing.Valid {
		customerID = existing.String
	}

	return s.client.createCheckoutSession(ctx, userID,
		s.cfg.CheckoutPrice(), s.cfg.ReturnURL(), s.cfg.ReturnURL(), customerID)
}

// ManagementURL is the provider's own page where this subscriber changes their card or
// cancels.
//
// It is generated per visit and short-lived, which is why it is fetched when asked for
// rather than stored. And it is THEIR page: a cancellation flow of our own would be one
// more thing that can disagree with what actually happened to the money.
func (s *Service) ManagementURL(ctx context.Context, userID int64) (string, error) {
	if !s.cfg.CanCheckout() {
		return "", ErrNoCheckout
	}
	customer, err := s.customerOf(ctx, userID)
	if err != nil {
		return "", err
	}
	return s.client.createPortalSession(ctx, customer, s.cfg.ReturnURL())
}
