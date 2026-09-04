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
// The one rule worth stating twice: a webhook is a SIGNAL that something about a customer
// changed, never a fact about what it changed to. Delivery is unordered and duplicates are
// possible — the provider says so itself — so the handler re-reads the customer's current
// subscriptions and derives the column from that. Nothing here branches on the event type,
// and there are hundreds of them.
package billing

import (
	"errors"
	"os"
	"strings"
)

// Config is everything the environment decides about billing.
type Config struct {
	// APIKey is the provider's SECRET key (sk_…), used server-side only.
	APIKey string
	// WebhookSecret (whsec_…) signs incoming deliveries. See verifySignature.
	WebhookSecret string
	// Prices are the price identifiers that confer Pro — usually two, a monthly and an
	// annual. A subscription for anything else must not make anyone Pro.
	Prices []string
	// SiteURL is where a customer comes back to after paying or after managing their
	// subscription. Empty disables checkout: there would be nowhere to return them to.
	//
	// It reads FRONTEND_ORIGIN, which the fleet already sets and which already means exactly
	// this. A SITE_URL of its own would be a second name for one fact, and the two would
	// disagree the first time somebody changed one.
	SiteURL string
}

// ConfigFromEnv reads the configuration. It NEVER fails.
//
// Absent credentials mean billing is switched off, not that the deployment is broken. This
// is the same degradation the rest of the fleet has for optional subsystems, and here it is
// also what makes the package harmless to anyone running freehire themselves.
func ConfigFromEnv() Config {
	return Config{
		APIKey:        strings.TrimSpace(os.Getenv("STRIPE_SECRET_KEY")),
		WebhookSecret: strings.TrimSpace(os.Getenv("STRIPE_WEBHOOK_SECRET")),
		Prices:        priceList(os.Getenv("STRIPE_PRICE_IDS")),
		SiteURL:       strings.TrimRight(strings.TrimSpace(os.Getenv("FRONTEND_ORIGIN")), "/"),
	}
}

// priceList parses the comma-separated price identifiers.
//
// There is no default, and an unset value yields an EMPTY list rather than a guess. An
// empty list confers Pro on nobody (see subscription.coversAny), which is the right way for
// a misconfiguration to fail: a guessed default would either match nothing — the same
// outcome, less honestly — or, worse, match a price we did not mean.
func priceList(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if id := strings.TrimSpace(part); id != "" {
			out = append(out, id)
		}
	}
	return out
}

// Enabled reports whether billing can do its job: verify a delivery and read customer
// state. Both credentials are needed, and one without the other is a half-configured
// deployment that would either accept unverifiable webhooks or record events it can never
// apply.
//
// The price list is required too. Without it every sync would derive "no Pro" and quietly
// downgrade every subscriber — a failure that looks exactly like nobody having paid.
func (c Config) Enabled() bool {
	return c.APIKey != "" && c.WebhookSecret != "" && len(c.Prices) > 0
}

// CanCheckout reports whether we can send anyone to buy.
//
// Separate from Enabled on purpose: a missing site URL must not stop the webhook from
// recording events. Subscriptions already sold keep renewing, and refusing to record their
// renewals because nobody can start a NEW purchase would lose money we have been paid.
func (c Config) CanCheckout() bool {
	return c.Enabled() && c.SiteURL != ""
}

// ErrNoCheckout is returned when checkout is not configured.
var ErrNoCheckout = errors.New("billing: checkout is not configured")

// CheckoutPrice is the price a new subscriber is sold. The first configured price is the
// one offered; the rest stay recognised so that a subscriber on an older or annual price
// keeps their plan.
func (c Config) CheckoutPrice() string {
	if len(c.Prices) == 0 {
		return ""
	}
	return c.Prices[0]
}

// ReturnURL is where the provider sends a browser back to, for both a finished purchase and
// a visit to the management portal.
func (c Config) ReturnURL() string { return c.SiteURL + "/my/plan" }
