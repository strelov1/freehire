package billing

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// A subscriber body as RevenueCat's v1 API returns it, trimmed to the fields we read.
func rcBody(entitlements string) []byte {
	return []byte(`{"subscriber":{"entitlements":{` + entitlements + `}}}`)
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	at, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return at
}

// TestRevenueCatReach is the whole entitlement rule, driven from the wire shape rather than
// from a struct we built: what the provider sends is what decides a plan, so that is what the
// table holds.
func TestRevenueCatReach(t *testing.T) {
	now := mustTime(t, "2026-09-04T12:00:00Z")

	cases := []struct {
		name    string
		body    []byte
		want    time.Time
		wantErr bool
	}{
		{
			name: "an active entitlement reaches its expiry",
			body: rcBody(`"pro":{"expires_date":"2026-10-04T12:00:00Z"}`),
			want: mustTime(t, "2026-10-04T12:00:00Z"),
		},
		{
			// The same reasoning that puts Stripe's past_due among the entitling statuses: a
			// card being retried has not stopped paying.
			name: "a grace period extends the reach",
			body: rcBody(`"pro":{"expires_date":"2026-09-03T12:00:00Z","grace_period_expires_date":"2026-09-20T12:00:00Z"}`),
			want: mustTime(t, "2026-09-20T12:00:00Z"),
		},
		{
			name: "a grace period shorter than the expiry does not shorten it",
			body: rcBody(`"pro":{"expires_date":"2026-10-04T12:00:00Z","grace_period_expires_date":"2026-09-05T12:00:00Z"}`),
			want: mustTime(t, "2026-10-04T12:00:00Z"),
		},
		{
			// Cancelled but paid for. The date is in the future and that is all that matters:
			// cancelling says "do not renew", not "refund me".
			name: "a cancelled subscription still reaches the end of what was paid for",
			body: rcBody(`"pro":{"expires_date":"2026-09-20T12:00:00Z","unsubscribe_detected_at":"2026-09-04T09:00:00Z"}`),
			want: mustTime(t, "2026-09-20T12:00:00Z"),
		},
		{
			// A past date is returned rather than zeroed. Whether the plan is over is
			// plan.TierOf's question, asked against now; answering it here would erase the
			// only record of when the subscription ended.
			name: "an expired entitlement returns the date it ended",
			body: rcBody(`"pro":{"expires_date":"2026-08-04T12:00:00Z"}`),
			want: mustTime(t, "2026-08-04T12:00:00Z"),
		},
		{
			name: "no entitlements at all confer nothing",
			body: rcBody(``),
		},
		{
			// A subscription to something else must not confer Pro, exactly as a Stripe
			// subscription on an unlisted price must not.
			name: "an entitlement we do not sell confers nothing",
			body: rcBody(`"coaching":{"expires_date":"2026-10-04T12:00:00Z"}`),
		},
		{
			// Fail closed. Failing open costs the revenue and hides that it is doing so.
			name:    "an unreadable expiry confers nothing and says so",
			body:    rcBody(`"pro":{"expires_date":"whenever"}`),
			wantErr: true,
		},
		{
			// Null means non-expiring, not expired — reading it as zero would silently
			// downgrade a lifetime grant. The column cannot say "forever", so it says "for as
			// long as we keep confirming": a horizon the reconciler renews.
			name: "a non-expiring entitlement is carried to the horizon, not to zero",
			body: rcBody(`"pro":{"expires_date":null}`),
			want: now.Add(revenuecatLifetimeHorizon),
		},
		{
			// The distinction the case above rests on: PRESENT-and-null means "does not
			// expire", ABSENT means we are reading a shape we do not understand. Collapsing
			// them grants a month of Pro for a renamed field — failing open, silently, in the
			// direction that costs the revenue.
			name:    "an entitlement carrying no dates at all confers nothing",
			body:    rcBody(`"pro":{}`),
			wantErr: true,
		},
		{
			name:    "an entitlement of an unfamiliar shape confers nothing",
			body:    rcBody(`"pro":{"expiresDate":"2026-10-04T12:00:00Z"}`),
			wantErr: true,
		},
		{
			// A grace period alone is still a readable reach.
			name: "a grace period without an expiry still confers",
			body: rcBody(`"pro":{"grace_period_expires_date":"2026-09-20T12:00:00Z"}`),
			want: mustTime(t, "2026-09-20T12:00:00Z"),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := revenuecatReach(c.body, "pro", now)
			if c.wantErr {
				if err == nil {
					t.Fatalf("reach = %v, want an error", got)
				}
				if !got.IsZero() {
					t.Fatalf("reach = %v alongside an error, want the zero time", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("reach: %v", err)
			}
			if !got.Equal(c.want) {
				t.Fatalf("reach = %v, want %v", got, c.want)
			}
		})
	}
}

// TestRevenueCatParseEvent covers what a delivery is reduced to. Note what is not read: the
// event's own claims about entitlement. It says something changed; what it changed to is read
// from the provider.
func TestRevenueCatParseEvent(t *testing.T) {
	ev, err := parseRevenueCatEvent([]byte(`{"api_version":"1.0","event":{
		"id":"evt_1","type":"INITIAL_PURCHASE","app_user_id":"42","expiration_at_ms":1790000000000}}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ev.ID != "evt_1" {
		t.Fatalf("id = %q, want evt_1", ev.ID)
	}
	if ev.Type != "INITIAL_PURCHASE" {
		t.Fatalf("type = %q, want INITIAL_PURCHASE", ev.Type)
	}
	// app_user_id IS our users.id, which is the whole reason this provider needs no binding.
	if ev.CustomerID != "42" || ev.UserRef != "42" {
		t.Fatalf("subject = (%q, %q), want (42, 42)", ev.CustomerID, ev.UserRef)
	}
	if len(ev.Payload) == 0 {
		t.Fatal("payload is empty; the event as received is what gets stored")
	}
}

func TestRevenueCatParseEventRefusesADeliveryWithNoID(t *testing.T) {
	if _, err := parseRevenueCatEvent([]byte(`{"event":{"type":"RENEWAL","app_user_id":"42"}}`)); err == nil {
		t.Fatal("accepted an event with no id; without one a redelivery would be applied twice")
	}
}

func TestRevenueCatParseEventRefusesAnEmptyEnvelope(t *testing.T) {
	if _, err := parseRevenueCatEvent([]byte(`{"api_version":"1.0"}`)); err == nil {
		t.Fatal("accepted an envelope carrying no event")
	}
}

// An unenumerated type is still recorded and applied: branching on event types would build a
// copy of the provider's state machine here, and there are dozens of them.
func TestRevenueCatParseEventAcceptsAnUnknownType(t *testing.T) {
	ev, err := parseRevenueCatEvent([]byte(`{"event":{"id":"evt_2","type":"SOMETHING_NEW","app_user_id":"7"}}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ev.Type != "SOMETHING_NEW" {
		t.Fatalf("type = %q, want it carried through unchanged", ev.Type)
	}
}

func TestRevenueCatConfigIsDisabledWithoutBothCredentials(t *testing.T) {
	full := RevenueCatConfig{APIKey: "sk_rc", WebhookSecret: "whsec", Entitlement: "pro"}
	if !full.Enabled() {
		t.Fatal("a fully configured provider reports itself disabled")
	}
	for _, c := range []struct {
		name string
		cfg  RevenueCatConfig
	}{
		{"no key", RevenueCatConfig{WebhookSecret: "whsec", Entitlement: "pro"}},
		{"no webhook secret", RevenueCatConfig{APIKey: "sk_rc", Entitlement: "pro"}},
		{"no entitlement", RevenueCatConfig{APIKey: "sk_rc", WebhookSecret: "whsec"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			if c.cfg.Enabled() {
				t.Fatal("a half-configured provider reports itself enabled")
			}
		})
	}
}

// The entitlement id has a default because it is not a secret and one value is right for
// almost everyone; the credentials have none, because a guess at either is a security
// decision made by omission.
func TestRevenueCatConfigDefaultsTheEntitlement(t *testing.T) {
	t.Setenv("REVENUECAT_API_KEY", "sk_rc")
	t.Setenv("REVENUECAT_WEBHOOK_SECRET", "whsec")
	t.Setenv("REVENUECAT_ENTITLEMENT", "")

	cfg := RevenueCatConfigFromEnv()
	if cfg.Entitlement != defaultRevenueCatEntitlement {
		t.Fatalf("entitlement = %q, want the default %q", cfg.Entitlement, defaultRevenueCatEntitlement)
	}
	if !cfg.Enabled() {
		t.Fatal("a provider with both credentials and a defaulted entitlement reports itself disabled")
	}
}

// TestTheLifetimeHorizonOutlivesTheSyncWindow pins the coupling that keeps a non-expiring
// entitlement alive.
//
// A lifetime grant is written at now+horizon and only stays Pro because the reconciler
// re-reads it before it lapses. That re-read happens when the row falls inside dueSoon's band
// — one window either side of now — so the horizon has to be comfortably longer than the
// window, and the hourly schedule has to give many chances inside the band. Shorten one or
// widen the other and the grant lapses silently at the horizon, with no webhook to notice.
//
// The window is the reconciler's (cmd/billing-sync's expiryWindow, 24h) and cannot be
// imported from a main package, so it is restated here — which is exactly why this test names
// it and why the constant's own comment points at this test.
func TestTheLifetimeHorizonOutlivesTheSyncWindow(t *testing.T) {
	const reconcilerWindow = 24 * time.Hour
	const reconcilerInterval = time.Hour

	if revenuecatLifetimeHorizon <= reconcilerWindow {
		t.Fatalf("horizon %s is not longer than the reconciler's window %s; a lifetime grant would be re-read from the moment it is written and never leave the band",
			revenuecatLifetimeHorizon, reconcilerWindow)
	}

	// The band is one window either side of the recorded instant, so the number of scheduled
	// passes that can see it is how much slack there is for downtime.
	chances := int((2 * reconcilerWindow) / reconcilerInterval)
	if chances < 24 {
		t.Fatalf("a lifetime grant gets only %d scheduled passes inside the band; one bad day would drop it", chances)
	}
}

// TestRefuseRedirect pins that the secret key travels to one host and no other.
//
// Go's default policy follows up to ten hops, carries Authorization to the original host and
// its subdomains, and permits HTTPS to be redirected to plain HTTP. safehttp re-dials each hop
// through the SSRF guard, which stops an internal address — and says nothing about a public
// host receiving a `Bearer sk_…` meant for api.revenuecat.com.
func TestRefuseRedirect(t *testing.T) {
	for _, target := range []string{
		"https://api.revenuecat.com.evil.test/v1/subscribers/1",
		"http://api.revenuecat.com/v1/subscribers/1",
		"https://api.revenuecat.com/v2/elsewhere",
	} {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
		if err != nil {
			t.Fatalf("build request: %v", err)
		}
		if err := refuseRedirect(req, nil); err == nil {
			t.Fatalf("a redirect to %s was allowed; the key must not follow one anywhere", target)
		}
	}
}
