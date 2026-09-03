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

// ErrDisabled is returned by every operation when billing is not configured. Callers
// render it as 404 rather than 500: an unconfigured optional subsystem should look absent,
// not broken.
var ErrDisabled = errors.New("billing: not configured")

// ErrUnknownSubscriber is returned when the provider's identifier is not one of our
// accounts. It is a reportable outcome, not a failure — see userIDFromAppUserID.
var ErrUnknownSubscriber = errors.New("billing: app_user_id is not one of our accounts")

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

// New constructs a Service. It never fails: an unconfigured Service reports itself
// disabled and refuses every operation with ErrDisabled.
func New(cfg Config, q *db.Queries) *Service {
	return NewWithBase(cfg, q, apiBaseURL)
}

// NewWithBase is New pointed at a different provider base URL, for tests that stand a stub
// in front of it. It mirrors telegramnotify.NewClientWithBase, which exists for the same
// reason: the alternative is an unexported field a test in another package cannot reach.
//
// It dials WITHOUT the SSRF guard, because the guard would refuse the loopback address
// every stub server listens on. That costs nothing real: the guard defends against a
// CALLER-SUPPLIED destination, and there is none here — the production base URL is a
// constant and the only variable part of the request is a path segment, escaped. New still
// uses the guarded client, so nothing in production takes this path.
func NewWithBase(cfg Config, q *db.Queries, baseURL string) *Service {
	s := &Service{cfg: cfg, q: q}
	if !cfg.Enabled() {
		return s
	}
	if baseURL == apiBaseURL {
		s.client = newProviderClient(cfg.APIKey, cfg.ProjectID)
	} else {
		s.client = newClient(cfg.APIKey, cfg.ProjectID, baseURL, &http.Client{Timeout: requestTimeout})
	}
	return s
}

// Enabled reports whether billing is configured.
func (s *Service) Enabled() bool { return s.cfg.Enabled() }

// Config exposes the configuration for the surfaces that need the checkout URL.
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
// A false second return is a REDELIVERY, not an error: the provider retries anything it
// did not get a 200 for and reuses the event id, so a duplicate is the normal case and the
// caller answers 200 to it.
func (s *Service) Record(ctx context.Context, ev Event) (rowID int64, recorded bool, err error) {
	if !s.Enabled() {
		return 0, false, ErrDisabled
	}

	params := db.InsertBillingEventParams{
		Provider:  Provider,
		EventID:   ev.ID,
		AppUserID: ev.AppUserID,
		EventType: ev.Type,
		Payload:   ev.Payload,
	}
	if userID, ok := userIDFromAppUserID(ev.AppUserID); ok {
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
	return rowID, true, nil
}

// Apply brings one recorded event's user up to date and marks the event processed.
//
// The order is load-bearing. Syncing first and stamping second means a crash in between
// leaves the event unprocessed and the reconciler repeats a sync that changes nothing —
// the harmless direction. Stamping first would drop the work on the floor.
func (s *Service) Apply(ctx context.Context, rowID int64, appUserID string) error {
	if err := s.Sync(ctx, appUserID); err != nil {
		return err
	}
	return s.q.MarkBillingEventProcessed(ctx, rowID)
}

// MarkProcessed stamps an event without applying it. It exists for the one case the
// reconciler cannot resolve — an identifier that was never one of ours — where retrying
// forever would be the only alternative.
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
// It reaches them through billing_events rather than through users, which is both cheaper
// (the subscriber base, not 8M rows) and safer: the provider's subscriber GET creates
// whatever identifier it is handed, so a candidate set that can only contain accounts
// which have transacted is the difference between repairing subscriptions and inventing
// them.
func (s *Service) SubscribersNearExpiry(ctx context.Context, window time.Duration, max int32) ([]int64, error) {
	if !s.Enabled() {
		return nil, ErrDisabled
	}
	now := time.Now().UTC()
	rows, err := s.q.ListSubscribersNearProExpiry(ctx, db.ListSubscribersNearProExpiryParams{
		FromTime: stamp(now.Add(-window)),
		ToTime:   stamp(now.Add(window)),
		MaxRows:  max,
	})
	if err != nil {
		return nil, err
	}
	out := make([]int64, 0, len(rows))
	for _, r := range rows {
		if r.UserID.Valid {
			out = append(out, r.UserID.Int64)
		}
	}
	return out, nil
}

// stamp converts a time to the pgtype the queries take.
func stamp(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// There is no ManagementURL here, and its absence is a fact about the provider rather
// than a decision.
//
// v1's subscriber object carried `management_url` — where that subscriber cancels — and
// the delete-account surface was built to link to it, precisely so the destination came
// from the provider instead of being composed by us. The v2 customer object does not carry
// it. Inventing one would reintroduce exactly the staleness that linking to theirs
// avoided, so the surface states that deleting an account does not cancel a subscription
// and leaves it there. If v2 grows the field, the endpoint that served it is in this
// package's history.

// Sync reads the provider's current state for one subscriber and writes users.pro_until
// from it.
//
// This is the only place the column is derived, and it is derived WHOLE every time rather
// than adjusted. That is what makes it idempotent, order-independent and safe to repeat:
// applying the same truth twice changes nothing, and a refund, a transfer or a grace
// period needs no code of its own because it is already reflected in what we read.
func (s *Service) Sync(ctx context.Context, appUserID string) error {
	if !s.Enabled() {
		return ErrDisabled
	}
	userID, ok := userIDFromAppUserID(appUserID)
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownSubscriber, appUserID)
	}

	sub, err := s.client.subscriberState(ctx, appUserID)
	if err != nil {
		return err
	}

	until := proUntilFrom(sub, s.cfg.Entitlements)
	proUntil := pgtype.Timestamptz{Time: until, Valid: !until.IsZero()}

	if err := s.q.SetProUntil(ctx, db.SetProUntilParams{ProUntil: proUntil, ID: userID}); err != nil {
		return fmt.Errorf("billing: writing pro_until for user %d: %w", userID, err)
	}
	return nil
}
