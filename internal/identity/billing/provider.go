package billing

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// provider is one payment provider, reduced to what the shared engine needs of it.
//
// There are two, and they are less alike than "a billing provider" suggests. Stripe sells on
// the web, holds its own customer records, and cannot be used at all inside an app — the
// stores forbid it. RevenueCat fronts the App Store and Google Play, which sell, cancel and
// refund without asking us, and addresses an account by an identifier WE chose. So the seam
// is drawn where the two genuinely differ rather than where the words match:
//
//   - How a delivery is authenticated and read (a header name, a secret, an envelope).
//   - How a delivery names one of our accounts, and what must be remembered to find that
//     account again later — a whole stored column for Stripe, nothing at all for RevenueCat.
//   - How far the provider's entitlement currently reaches, as one instant.
//   - Which source column of users that instant belongs in.
//
// What is NOT here is as deliberate. Checkout, a management portal, prices and receipts are
// Stripe's alone: a store subscription is bought, changed and cancelled somewhere we have no
// API into. Putting them in this interface would give RevenueCat four methods that can only
// answer "not applicable", and an interface whose implementations refuse half of it is not an
// abstraction, it is a union type wearing one.
//
// The rule that survives across both, and belongs to neither: a webhook is a SIGNAL that
// something changed, never a fact about what it changed to. That is why reach() re-reads the
// provider instead of taking the event's word, and why the engine owns that decision rather
// than each implementation repeating it.
type provider interface {
	// name is what billing_events.provider records. It is part of the idempotency key, so
	// two providers' opaque event ids cannot collide.
	name() string

	// enabled reports whether this provider is configured. An unconfigured provider is
	// absent, not broken: its routes are never mounted and its reconciler pass is skipped.
	enabled() bool

	// signatureHeader is the header this provider signs its deliveries with. Both use the
	// same `t=<unix>,v1=<hex>` scheme over "<t>.<raw body>"; only the name differs.
	signatureHeader() string

	// accept verifies a delivery came from the provider, recently, and parses it. raw MUST be
	// the request body exactly as received — both providers sign the bytes, so a
	// parse-and-reserialise rejects valid deliveries.
	//
	// Verification and parsing are one method because the engine must not be able to do the
	// second without the first: the only way to obtain an Event is to have authenticated the
	// bytes it came from.
	accept(raw []byte, signature string, now time.Time) (Event, error)

	// account is which of our users a delivery is about. False means nobody we can resolve,
	// which is a reportable outcome rather than a failure — an event about something we do
	// not meter, or an object created outside this integration.
	account(ctx context.Context, ev Event) (int64, bool)

	// bind remembers whatever this provider needs remembered to reach the account again from
	// a scheduled re-check, which starts from a user rather than from an event.
	//
	// For Stripe that is the customer id, without which the reconciler cannot ask about
	// anybody. For RevenueCat it is nothing at all: the subscriber IS users.id, so there is
	// no second identifier to lose. A no-op implementation is the honest answer there, not a
	// stub — see revenuecatProvider.bind.
	bind(ctx context.Context, userID int64, ev Event) error

	// knows reports whether this provider has ever been heard from about this account.
	//
	// It is separate from reach because asking a provider about a stranger is not free in the
	// same way for both. RevenueCat's subscribers endpoint CREATES the subscriber when the id
	// is unknown, so a pass over the user table would enrol every account we have; Stripe
	// simply has no customer to name. Either way, a BULK pass must ask this first.
	//
	// A SELF-SERVICE call must not — see engine.SyncCaller. That distinction is the whole
	// reason this is a separate method rather than a check hidden inside reach: the guard was
	// once inside it, and it made a first purchase whose webhook was lost unrecoverable, since
	// the buyer has no footprint precisely until the first delivery lands.
	knows(ctx context.Context, userID int64) (bool, error)

	// reach reads the provider's CURRENT state for one account and reduces it to how far its
	// entitlement extends. The zero time means this provider confers nothing, which is not
	// the same as the account being free — another source may still confer.
	//
	// It may return ErrNoSubscription for an account this provider cannot address at all, so
	// a caller can tell "we asked and there is nothing" from "there was nobody to ask".
	reach(ctx context.Context, userID int64) (time.Time, error)

	// store writes this provider's own source column of users, and no other. The derived
	// users.pro_until follows by way of the schema (migration 0135); assigning it directly
	// is refused by Postgres, which is what makes revoking another origin's grant
	// unwritable rather than merely discouraged.
	store(ctx context.Context, userID int64, until pgtype.Timestamptz) error

	// dueSoon lists accounts whose entitlement FROM THIS PROVIDER expires inside the window
	// — the reconciler's second pass, which repairs a renewal whose webhook never arrived.
	//
	// The predicate belongs on the provider's own source column and never on the derived
	// pro_until: that column is the furthest of three sources, so a subscriber whose other
	// source reaches beyond this renewal would sit outside the window and never be
	// re-checked. The lost renewal this pass exists to repair would stay lost, silently, on
	// an account that is paying.
	dueSoon(ctx context.Context, from, to time.Time, max int32) ([]int64, error)
}
