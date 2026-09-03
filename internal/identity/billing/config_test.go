package billing

import (
	"slices"
	"testing"
)

// setEnv points the whole configuration at test values; "" removes a variable.
const testProjectID = "proj_test"

func setEnv(t *testing.T, apiKey, secret, entitlements, checkout string) {
	t.Helper()
	t.Setenv("REVENUECAT_API_KEY", apiKey)
	t.Setenv("REVENUECAT_WEBHOOK_SECRET", secret)
	t.Setenv("REVENUECAT_PROJECT_ID", testProjectID)
	t.Setenv("REVENUECAT_ENTITLEMENT", entitlements)
	t.Setenv("BILLING_CHECKOUT_URL", checkout)
}

// TestConfigFromEnvDisabledIsNotAnError is the property that makes this package safe to
// ship in a public repository. A self-hoster who never sets these variables must not be
// able to tell the code is there, and must certainly not have their server refuse to boot
// over a subsystem they never asked for.
func TestConfigFromEnvDisabledIsNotAnError(t *testing.T) {
	cases := []struct {
		name           string
		apiKey, secret string
	}{
		{name: "nothing configured"},
		{name: "api key only", apiKey: "sk_test"},
		{name: "webhook secret only", secret: "whsec_test"},
	}

	// Every v2 call is scoped to a project, so a deployment without one has no URL to read
	// state from. Checked separately because setEnv always supplies it.
	t.Run("no project id", func(t *testing.T) {
		setEnv(t, "sk_test", "whsec_test", "", "")
		t.Setenv("REVENUECAT_PROJECT_ID", "")
		if ConfigFromEnv().Enabled() {
			t.Fatal("want billing disabled without a project id")
		}
	})

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setEnv(t, tc.apiKey, tc.secret, "", "")
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
	setEnv(t, "sk_test", "whsec_test", "", "")
	cfg := ConfigFromEnv()

	if !cfg.Enabled() {
		t.Fatal("want billing enabled with both credentials present")
	}
	// Checkout needs a paywall of its own, and its absence must not disable the webhook —
	// events still have to be recorded whether or not we can sell anything today.
	if cfg.CanCheckout() {
		t.Fatal("want checkout unavailable with no paywall URL")
	}
	if want := []string{"pro"}; !slices.Equal(cfg.Entitlements, want) {
		t.Fatalf("want the default entitlement %v, got %v", want, cfg.Entitlements)
	}
}

func TestConfigFromEnvEntitlementList(t *testing.T) {
	setEnv(t, "sk_test", "whsec_test", " pro , pro_annual ,, ", "")
	cfg := ConfigFromEnv()

	want := []string{"pro", "pro_annual"}
	if !slices.Equal(cfg.Entitlements, want) {
		t.Fatalf("want %v, got %v", want, cfg.Entitlements)
	}
}

// TestCheckoutURLFor builds the provider's hosted paywall link. The identifier is a PATH
// SEGMENT, not a query parameter, which is the detail that decides whether the URL works
// at all.
func TestCheckoutURLFor(t *testing.T) {
	cases := []struct {
		name    string
		base    string
		userID  int64
		want    string
		wantErr bool
	}{
		{
			name:   "identifier is appended as a path segment",
			base:   "https://pay.rev.cat/abcdef",
			userID: 601,
			want:   "https://pay.rev.cat/abcdef/601",
		},
		{
			// A base ending in a slash is the obvious way to write it in an env file, and
			// a naive join would produce a double slash and a 404.
			name:   "a trailing slash on the base does not double up",
			base:   "https://pay.rev.cat/abcdef/",
			userID: 601,
			want:   "https://pay.rev.cat/abcdef/601",
		},
		{
			name:    "no paywall configured",
			base:    "",
			userID:  601,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setEnv(t, "sk_test", "whsec_test", "", tc.base)
			cfg := ConfigFromEnv()

			got, err := cfg.CheckoutURLFor(tc.userID)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want an error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("want no error, got %v", err)
			}
			if got != tc.want {
				t.Fatalf("want %q, got %q", tc.want, got)
			}
		})
	}
}
