package billing

import (
	"context"
	"fmt"
	"time"

	"github.com/strelov1/freehire/internal/platform/db"
)

// stripeName is what billing_events.provider records for this provider.
const stripeName = "stripe"

// stripeSignatureHeader is the header Stripe signs each delivery with.
const stripeSignatureHeader = "Stripe-Signature"

// stripeProvider is Stripe behind the provider seam.
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

// account finds which account an event is about. The customer binding is authoritative; the
// echoed account reference covers the first purchase, where no binding exists yet.
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

// bind records which provider customer an account is. Idempotent (guarded by IS DISTINCT
// FROM). An event with no customer binds nothing; the engine calls this for every delivery.
func (p *stripeProvider) bind(ctx context.Context, userID int64, ev Event) error {
	if ev.CustomerID == "" {
		return nil
	}
	return p.q.SetStripeCustomerID(ctx, db.SetStripeCustomerIDParams{
		ID: userID, StripeCustomerID: ev.CustomerID,
	})
}

// knows reports whether this account has a Stripe customer to ask about.
func (p *stripeProvider) knows(ctx context.Context, userID int64) (bool, error) {
	customer, err := p.q.GetStripeCustomerID(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("billing: reading the customer of user %d: %w", userID, err)
	}
	return customer.Valid && customer.String != "", nil
}

// reach reads Stripe's current subscriptions for one account and reduces them to the furthest
// point a conferring one still stands, per tier. One listing serves every tier, since the
// tier is only which configured price list the price is in.
func (p *stripeProvider) reach(ctx context.Context, userID int64) (entitlement, error) {
	customer, err := p.customerOf(ctx, userID)
	if err != nil {
		return entitlement{}, err
	}
	sub, err := p.client.subscriberState(ctx, customer)
	if err != nil {
		return entitlement{}, err
	}
	return entitlementFrom(sub, p.cfg), nil
}

// store writes all of Stripe's source columns in one statement (see the query's note).
func (p *stripeProvider) store(ctx context.Context, userID int64, ent entitlement) error {
	return p.q.SetStripeEntitlement(ctx, db.SetStripeEntitlementParams{
		ProUntil:     nullable(ent.Pro),
		UltraUntil:   nullable(ent.Ultra),
		ApiUntil:     nullable(ent.API),
		ApiDataUntil: nullable(ent.Data),
		ID:           userID,
	})
}

// dueSoon goes through the customer binding, so only accounts that have transacted are candidates.
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
