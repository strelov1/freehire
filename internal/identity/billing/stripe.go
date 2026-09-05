package billing

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

// stripeName is what billing_events.provider records for this provider.
const stripeName = "stripe"

// stripeSignatureHeader is the header Stripe signs each delivery with.
const stripeSignatureHeader = "Stripe-Signature"

// stripeProvider is Stripe behind the provider seam.
//
// Everything here was already in this package and is unchanged in behaviour; what moved is
// only which type owns it. The existing Stripe tests are the proof — three of their lines
// changed and none was an assertion: a helper that moved types, a header constant written out
// locally, and a constructor call that stopped reaching into a field.
type stripeProvider struct {
	cfg    Config
	q      *db.Queries
	client *client
}

func (p *stripeProvider) name() string { return stripeName }

func (p *stripeProvider) enabled() bool { return p.cfg.Enabled() }

func (p *stripeProvider) signatureHeader() string { return stripeSignatureHeader }

func (p *stripeProvider) accept(raw []byte, signature string, now time.Time) (Event, error) {
	if err := verifySignature(raw, signature, p.cfg.WebhookSecret, stripeSignatureWindow, now); err != nil {
		return Event{}, err
	}
	return parseEvent(raw)
}

// account finds which account an event is about.
//
// Two routes, and the order matters. The customer binding is authoritative for everything
// after the first purchase. The account reference the provider echoes back is what covers
// the first purchase itself, where no binding exists yet — a checkout completion carries it
// precisely because nothing else could.
func (p *stripeProvider) account(ctx context.Context, ev Event) (int64, bool) {
	if ev.CustomerID != "" {
		if id, err := p.q.GetUserIDByStripeCustomer(ctx, ev.CustomerID); err == nil {
			return id, true
		}
	}
	if id, ok := userIDFromRef(ev.UserRef); ok {
		return id, true
	}
	return 0, false
}

// bind records which provider customer an account is. Idempotent: the query is guarded by
// IS DISTINCT FROM, so writing the value already there does nothing.
//
// An event carrying no customer binds nothing and is not an error — the engine calls this
// for every delivery, and only some of them name a customer.
func (p *stripeProvider) bind(ctx context.Context, userID int64, ev Event) error {
	if ev.CustomerID == "" {
		return nil
	}
	return p.q.SetStripeCustomerID(ctx, db.SetStripeCustomerIDParams{
		ID: userID, StripeCustomerID: ev.CustomerID,
	})
}

// knows reports whether this account has a Stripe customer to ask about. Asking about a
// stranger costs nothing here — there is simply nobody to name — but the answer is the same
// one reach would give, one round trip earlier.
func (p *stripeProvider) knows(ctx context.Context, userID int64) (bool, error) {
	customer, err := p.q.GetStripeCustomerID(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("billing: reading the customer of user %d: %w", userID, err)
	}
	return customer.Valid && customer.String != "", nil
}

// reach reads Stripe's current subscriptions for one account and reduces them to the furthest
// point a Pro-conferring one still stands.
func (p *stripeProvider) reach(ctx context.Context, userID int64) (time.Time, error) {
	customer, err := p.customerOf(ctx, userID)
	if err != nil {
		return time.Time{}, err
	}
	sub, err := p.client.subscriberState(ctx, customer)
	if err != nil {
		return time.Time{}, err
	}
	return proUntilFrom(sub, p.cfg.Prices), nil
}

func (p *stripeProvider) store(ctx context.Context, userID int64, until pgtype.Timestamptz) error {
	return p.q.SetProUntilStripe(ctx, db.SetProUntilStripeParams{Until: until, ID: userID})
}

// dueSoon reaches accounts through the customer binding, so the candidate set is the accounts
// that have actually transacted rather than every account on the site.
func (p *stripeProvider) dueSoon(ctx context.Context, from, to time.Time, max int32) ([]int64, error) {
	rows, err := p.q.ListSubscribersNearProExpiryStripe(ctx, db.ListSubscribersNearProExpiryStripeParams{
		FromTime: stamp(from),
		ToTime:   stamp(to),
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

// customerOf is the provider's customer for one account, or ErrNoSubscription when the
// account has never transacted.
func (p *stripeProvider) customerOf(ctx context.Context, userID int64) (string, error) {
	customer, err := p.q.GetStripeCustomerID(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("billing: reading the customer of user %d: %w", userID, err)
	}
	if !customer.Valid || customer.String == "" {
		return "", fmt.Errorf("%w: user %d", ErrNoSubscription, userID)
	}
	return customer.String, nil
}
