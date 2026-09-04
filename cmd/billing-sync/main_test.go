package main

import (
	"testing"
)

// TestRunIsANoOpWithoutCredentials asserts the gate sits BEFORE Bootstrap.
//
// DATABASE_URL is deliberately cleared: if run() reached Bootstrap it would fail to open a
// pool and return 1, so a zero here is proof that it never tried. That is the property the
// spec asks for — an unconfigured deployment must run without touching the database — and
// it is also what makes this binary harmless in a checkout that is not freehire.me.
func TestRunIsANoOpWithoutCredentials(t *testing.T) {
	t.Setenv("STRIPE_SECRET_KEY", "")
	t.Setenv("STRIPE_WEBHOOK_SECRET", "")
	t.Setenv("DATABASE_URL", "")

	if code := run(); code != 0 {
		t.Fatalf("want a clean no-op exit, got %d", code)
	}
}

// TestRunIsANoOpWithHalfCredentials covers the misconfiguration that is worse than none: a
// key without a signing secret would accept unverifiable webhooks, and a secret without a
// key would record events it can never apply.
func TestRunIsANoOpWithHalfCredentials(t *testing.T) {
	for _, tc := range []struct{ name, apiKey, secret string }{
		{name: "api key only", apiKey: "sk_test"},
		{name: "webhook secret only", secret: "whsec_test"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("STRIPE_SECRET_KEY", tc.apiKey)
			t.Setenv("STRIPE_WEBHOOK_SECRET", tc.secret)
			t.Setenv("DATABASE_URL", "")

			if code := run(); code != 0 {
				t.Fatalf("want a clean no-op exit, got %d", code)
			}
		})
	}
}

func TestMaxPerRun(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want int32
	}{
		{name: "unset keeps the default", raw: "", want: maxPerRunDefault},
		{name: "a positive number is honoured", raw: "50", want: 50},
		// A typo resolving to zero would make every run a silent no-op, which looks exactly
		// like a backlog that has already been drained.
		{name: "zero keeps the default", raw: "0", want: maxPerRunDefault},
		{name: "negative keeps the default", raw: "-5", want: maxPerRunDefault},
		{name: "nonsense keeps the default", raw: "lots", want: maxPerRunDefault},
		// Parsed at the width it is used at. Read as a platform int and converted, this
		// would WRAP to a negative limit — a query error, not a big batch — and the
		// "not a positive number" log line would be a lie.
		{name: "beyond a batch size keeps the default", raw: "3000000000", want: maxPerRunDefault},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("BILLING_SYNC_MAX_PER_RUN", tc.raw)
			if got := maxPerRun(); got != tc.want {
				t.Fatalf("want %d, got %d", tc.want, got)
			}
		})
	}
}
