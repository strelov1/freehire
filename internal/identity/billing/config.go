// Package billing connects the Pro subscription at the payment provider to the one column
// that decides a user's plan, users.pro_until.
//
// This is freehire.me's hosted billing and is not supported for self-hosting. Without its
// environment variables the package is disabled, its routes answer 404, and its worker
// exits without opening a connection.
//
// The provider owns the subscription, we own one derived timestamp, and the metered request
// path reads a column rather than an API, so a slow provider never delays a user.
//
// A webhook is a signal that something changed, never a fact about what it changed to.
// Delivery is unordered and may repeat, so the handler re-reads the customer's current
// subscriptions and derives the column; nothing branches on the event type.
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
	// UltraPrices are the price identifiers that confer Ultra. The tier is decided here and
	// nowhere else; none named means nobody resolves to ultra. A list rather than price
	// metadata, since a dashboard mis-click could silently upgrade everybody.
	UltraPrices []string
	// APIPrices and APIDataPrices are the price identifiers of the API product's two tiers
	// (API and Data). A separate product from Pro/Ultra: API access confers no plan, and
	// checkout never converts between products (see Config.checkoutFamily). Unset sells none.
	APIPrices     []string
	APIDataPrices []string
	// SiteURL is where a customer returns after paying or managing their subscription (read
	// from FRONTEND_ORIGIN). Empty disables checkout.
	SiteURL string
}

// ConfigFromEnv reads the configuration. It never fails: absent credentials mean billing is
// switched off.
func ConfigFromEnv() Config {
	return Config{
		APIKey:        strings.TrimSpace(os.Getenv("STRIPE_SECRET_KEY")),
		WebhookSecret: strings.TrimSpace(os.Getenv("STRIPE_WEBHOOK_SECRET")),
		Prices:        priceList(os.Getenv("STRIPE_PRICE_IDS")),
		UltraPrices:   priceList(os.Getenv("STRIPE_ULTRA_PRICE_IDS")),
		APIPrices:     priceList(os.Getenv("STRIPE_API_PRICE_IDS")),
		APIDataPrices: priceList(os.Getenv("STRIPE_API_DATA_PRICE_IDS")),
		SiteURL:       strings.TrimRight(strings.TrimSpace(os.Getenv("FRONTEND_ORIGIN")), "/"),
	}
}

// priceList parses the comma-separated price identifiers. Unset yields an empty list, which
// confers Pro on nobody (see subscription.entitles) rather than guessing a price.
func priceList(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if id := strings.TrimSpace(part); id != "" {
			out = append(out, id)
		}
	}
	return out
}

// Enabled reports whether billing can verify a delivery and read customer state. It needs
// both credentials and a price list; without prices every sync would derive "no Pro" and
// quietly downgrade every subscriber.
func (c Config) Enabled() bool {
	return c.APIKey != "" && c.WebhookSecret != "" && len(c.Prices) > 0
}

// CanCheckout reports whether we can send anyone to buy. Separate from Enabled so a missing
// site URL never stops recording renewals of subscriptions already sold.
func (c Config) CanCheckout() bool {
	return c.Enabled() && c.SiteURL != ""
}

// ErrNoCheckout is returned when checkout is not configured.
var ErrNoCheckout = errors.New("billing: checkout is not configured")

// CheckoutPrice is the price a new subscriber is sold: the first configured one. The rest
// stay recognised so existing subscribers keep their plan.
func (c Config) CheckoutPrice() string {
	if len(c.Prices) == 0 {
		return ""
	}
	return c.Prices[0]
}

// ReturnURL is where the provider sends a browser back to: an abandoned checkout, and a
// visit to the management portal.
func (c Config) ReturnURL() string { return c.SiteURL + "/my/plan" }

// SuccessURL is where a finished purchase lands: ReturnURL plus a marker, so the SPA can
// tell a completed checkout from a cancel and record the purchase. It is a marker, not the
// session id, so no identifier leaks into history and no editable value is trusted.
func (c Config) SuccessURL() string { return c.ReturnURL() + "?checkout=success" }
