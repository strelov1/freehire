package billing

import (
	"context"
	"time"
)

// provider is one payment provider (Stripe or RevenueCat), reduced to what the shared engine
// needs: how a delivery is authenticated and read, how it names one of our accounts and what
// must be remembered to find it again, how far the entitlement reaches, and which source
// column of users it belongs in.
//
// Checkout, portal, prices and receipts are deliberately absent: they are Stripe's alone.
//
// A webhook is a signal that something changed, never a fact about what it changed to; the
// engine, not each implementation, re-reads the provider through reach().
type provider interface {
	// name is what billing_events.provider records. It is part of the idempotency key, so
	// two providers' event ids cannot collide.
	name() string

	// enabled reports whether this provider is configured. An unconfigured provider is
	// absent: its routes are not mounted and its reconciler pass is skipped.
	enabled() bool

	// signatureHeader is the header carrying the signature. Both providers use the same
	// `t=<unix>,v1=<hex>` scheme over "<t>.<raw body>".
	signatureHeader() string

	// accept verifies a delivery came from the provider, recently, and parses it. raw MUST be
	// the body exactly as received (the signature covers the bytes). Verify and parse are one
	// method so an Event can only come from authenticated bytes.
	accept(raw []byte, signature string, now time.Time) (Event, error)

	// account is which of our users a delivery is about. False means nobody we can resolve
	// (something we do not meter, or created outside this integration); it is not a failure.
	account(ctx context.Context, ev Event) (int64, bool)

	// bind remembers what this provider needs to reach the account from a scheduled re-check,
	// which starts from a user, not an event. For Stripe that is the customer id; RevenueCat
	// needs nothing because the subscriber is users.id (see revenuecatProvider.bind).
	bind(ctx context.Context, userID int64, ev Event) error

	// knows reports whether this provider has ever been heard from about this account.
	//
	// RevenueCat's subscribers endpoint creates the subscriber for an unknown id, so a BULK pass
	// must ask this first. A self-service call must not (see engine.SyncCaller): a buyer whose
	// first webhook was lost has no footprint yet and would be unrecoverable.
	knows(ctx context.Context, userID int64) (bool, error)

	// reach reads the provider's current state for one account and reduces it to how far its
	// entitlement extends for each tier. A zero time means this provider confers that tier on
	// nobody, not that the account lacks it; another source may confer it.
	//
	// One call covers every tier so the state is read and applied whole (see AGENTS.md); two
	// reads could straddle a change. It may return ErrNoSubscription for an account this
	// provider cannot address at all.
	reach(ctx context.Context, userID int64) (entitlement, error)

	// store writes this provider's own source columns of users, and no others. The derived
	// users.pro_until and users.ultra_until follow via the schema; Postgres refuses direct
	// assignment, so another origin's grant cannot be revoked here.
	//
	// It writes every tier column the provider owns on every call, including those it confers
	// nothing for, or a cancellation could never take back what was granted.
	store(ctx context.Context, userID int64, ent entitlement) error

	// dueSoon lists accounts whose entitlement from this provider expires inside the window
	// (the reconciler pass that repairs a renewal whose webhook never arrived).
	//
	// The predicate must use the provider's own source column, never the derived pro_until: that
	// is the furthest of three sources, so a paying account whose other source reaches further
	// would never be re-checked.
	dueSoon(ctx context.Context, from, to time.Time, max int32) ([]int64, error)
}
