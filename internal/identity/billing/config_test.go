package billing

import (
	"slices"
	"testing"
)

// setEnv points the whole configuration at test values; "" removes a variable.
func setEnv(t *testing.T, apiKey, secret, prices, siteURL string) {
	t.Helper()
	t.Setenv("STRIPE_SECRET_KEY", apiKey)
	t.Setenv("STRIPE_WEBHOOK_SECRET", secret)
	t.Setenv("STRIPE_PRICE_IDS", prices)
	t.Setenv("FRONTEND_ORIGIN", siteURL)
}

// TestConfigFromEnvDisabledIsNotAnError is the property that makes this package safe to ship
// in a public repository. A self-hoster who never sets these variables must not be able to
// tell the code is there, and must certainly not have their server refuse to boot over a
// subsystem they never asked for.
func TestConfigFromEnvDisabledIsNotAnError(t *testing.T) {
	cases := []struct {
		name                   string
		apiKey, secret, prices string
	}{
		{name: "nothing configured"},
		{name: "api key only", apiKey: "sk_test"},
		{name: "webhook secret only", secret: "whsec_test"},
		{name: "credentials but no prices", apiKey: "sk_test", secret: "whsec_test"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setEnv(t, tc.apiKey, tc.secret, tc.prices, "")
			cfg := ConfigFromEnv()
			if cfg.Enabled() {
				t.Fatal("want billing disabled")
			}
			if cfg.CanCheckout() {
				t.Fatal("want checkout unavailable while billing is disabled")
			}
		})
	}
}

func TestConfigFromEnvEnabled(t *testing.T) {
	setEnv(t, "sk_test", "whsec_test", "price_a", "")
	cfg := ConfigFromEnv()

	if !cfg.Enabled() {
		t.Fatal("want billing enabled with credentials and a price")
	}
	// Checkout needs somewhere to send the buyer back to, and its absence must not disable
	// the webhook — subscriptions already sold keep renewing, and refusing to record their
	// renewals would lose money we have been paid.
	if cfg.CanCheckout() {
		t.Fatal("want checkout unavailable with no site URL")
	}
}

func TestConfigFromEnvPriceList(t *testing.T) {
	setEnv(t, "sk_test", "whsec_test", " price_a , price_b ,, ", "https://freehire.me")
	cfg := ConfigFromEnv()

	want := []string{"price_a", "price_b"}
	if !slices.Equal(cfg.Prices, want) {
		t.Fatalf("want %v, got %v", want, cfg.Prices)
	}
	// The first is what a NEW subscriber is sold; the rest stay recognised so somebody on an
	// older or annual price keeps their plan.
	if got := cfg.CheckoutPrice(); got != "price_a" {
		t.Fatalf("checkout price: want price_a, got %q", got)
	}
	if !cfg.CanCheckout() {
		t.Fatal("want checkout available")
	}
}

func TestReturnURL(t *testing.T) {
	// A trailing slash is the obvious way to write it in an env file, and a naive join would
	// produce "//my/plan".
	setEnv(t, "sk_test", "whsec_test", "price_a", "https://freehire.me/")
	if got := ConfigFromEnv().ReturnURL(); got != "https://freehire.me/my/plan" {
		t.Fatalf("want https://freehire.me/my/plan, got %q", got)
	}
}
