// Package billing connects the Pro subscription at the payment provider to the one column
// that decides a user's plan, users.pro_until.
//
// THIS IS FREEHIRE.ME'S HOSTED BILLING AND IS NOT SUPPORTED FOR SELF-HOSTING. It is in the
// open repository under the same licence as the rest, because closing it would protect
// nothing — secrets live in the environment either way, and the moat is the catalogue, not
// this code — but nothing here is built to be run by anyone else. Without its environment
// variables the package reports itself disabled, its routes answer 404, and its worker
// exits without opening a connection.
//
// The shape is deliberately small, because migration 0120 made it small: the provider owns
// the subscription, we own one derived timestamp, and the metered request path reads a
// column rather than an API. A provider that is slow or unreachable can therefore never
// delay a user's next question.
//
// The one rule worth stating twice: a webhook is a SIGNAL that something about a user
// changed, never a fact about what it changed to. Delivery is unordered and duplicates are
// possible, so the handler re-reads the subscriber's current state and derives the column
// from that. Nothing here branches on the event type.
package billing

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

// Config is everything the environment decides about billing.
type Config struct {
	// APIKey is the provider's SECRET key (sk_…), used server-side only to read subscriber
	// state. A public key gets 401 on that endpoint, by their design and ours.
	APIKey string
	// WebhookSecret signs incoming deliveries. See verifySignature.
	WebhookSecret string
	// Entitlements are the entitlement identifiers that confer Pro. Usually one: an
	// entitlement already sits above products, so monthly and annual share it.
	Entitlements []string
	// CheckoutURL is everything up to and including the hosted paywall's token. The user's
	// identifier is appended as a further path segment — see CheckoutURLFor.
	CheckoutURL string
}

// defaultEntitlement is the identifier assumed when the environment names none.
const defaultEntitlement = "pro"

// ConfigFromEnv reads the configuration. It NEVER fails.
//
// Absent credentials mean billing is switched off, not that the deployment is broken. This
// is the same degradation the rest of the fleet has for optional subsystems, and here it
// is also what makes the package harmless to anyone running freehire themselves.
func ConfigFromEnv() Config {
	cfg := Config{
		APIKey:        strings.TrimSpace(os.Getenv("REVENUECAT_API_KEY")),
		WebhookSecret: strings.TrimSpace(os.Getenv("REVENUECAT_WEBHOOK_SECRET")),
		Entitlements:  entitlementList(os.Getenv("REVENUECAT_ENTITLEMENT")),
		// Trimmed of a trailing slash so appending a segment cannot produce "//", which the
		// paywall answers with a 404 — an easy thing to write in an env file and a
		// miserable thing to diagnose from the other end.
		CheckoutURL: strings.TrimRight(strings.TrimSpace(os.Getenv("BILLING_CHECKOUT_URL")), "/"),
	}
	return cfg
}

// entitlementList parses the comma-separated entitlement identifiers, falling back to the
// default. An empty list would silently mean "nobody is ever Pro", so it cannot be one.
func entitlementList(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if name := strings.TrimSpace(part); name != "" {
			out = append(out, name)
		}
	}
	if len(out) == 0 {
		return []string{defaultEntitlement}
	}
	return out
}

// Enabled reports whether billing can do its job: verify a delivery and read subscriber
// state. Both credentials are needed, and one without the other is a half-configured
// deployment that would accept unverifiable webhooks or record events it can never apply.
func (c Config) Enabled() bool {
	return c.APIKey != "" && c.WebhookSecret != ""
}

// CanCheckout reports whether we can send anyone to buy.
//
// Separate from Enabled on purpose: a missing paywall URL must not stop the webhook from
// recording events. Purchases made on another surface — the mobile app, through the App
// Store — still arrive, and refusing to record them because the web paywall is not set up
// would lose subscriptions we were paid for.
func (c Config) CanCheckout() bool {
	return c.Enabled() && c.CheckoutURL != ""
}

// ErrNoCheckout is returned when no hosted paywall is configured.
var ErrNoCheckout = errors.New("billing: no checkout URL is configured")

// CheckoutURLFor is where this user buys Pro.
//
// The identifier is a PATH SEGMENT, not a query parameter — `https://pay.rev.cat/<token>/
// <app_user_id>` — which is the single detail that decides whether the link resolves.
//
// It takes a user id rather than a string so that the app-user identifier cannot be
// anything but the account's own. The provider assigns an anonymous identifier to a client
// it has not been told about, and a purchase made under one lands on a subscriber we can
// never resolve to a person; the type is what makes that unreachable rather than merely
// documented.
func (c Config) CheckoutURLFor(userID int64) (string, error) {
	if !c.CanCheckout() {
		return "", ErrNoCheckout
	}
	// A decimal integer needs no escaping, and saying so here is cheaper than an import
	// that suggests the value might ever contain something that does.
	return c.CheckoutURL + "/" + strconv.FormatInt(userID, 10), nil
}
