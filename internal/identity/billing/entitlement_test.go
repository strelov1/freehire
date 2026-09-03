package billing

import (
	"testing"
	"time"
)

func at(t *testing.T, s string) *time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("bad test timestamp %q: %v", s, err)
	}
	return &v
}

// TestProUntilFrom walks every shape the provider's entitlement map arrives in. It is a
// pure function precisely so that the two cases nobody would notice going wrong — a grace
// period and an absent expiry — are as cheap to assert as the ordinary one.
func TestProUntilFrom(t *testing.T) {
	proIDs := []string{"pro"}

	cases := []struct {
		name string
		sub  subscriber
		ids  []string
		want string // RFC3339, or "" for the zero time
	}{
		{
			name: "active entitlement",
			sub: subscriber{Entitlements: map[string]entitlement{
				"pro": {ExpiresDate: at(t, "2026-10-01T00:00:00Z")},
			}},
			want: "2026-10-01T00:00:00Z",
		},
		{
			// A lapsed subscription still writes the provider's date. Deciding it is over
			// is plan.TierOf's job, comparing against now; writing zero here would erase
			// when it ended, which is the only thing that says why the account is free.
			name: "lapsed entitlement keeps its date",
			sub: subscriber{Entitlements: map[string]entitlement{
				"pro": {ExpiresDate: at(t, "2020-01-01T00:00:00Z")},
			}},
			want: "2020-01-01T00:00:00Z",
		},
		{
			name: "no entitlements at all",
			sub:  subscriber{Entitlements: map[string]entitlement{}},
			want: "",
		},
		{
			name: "only an entitlement that does not confer pro",
			sub: subscriber{Entitlements: map[string]entitlement{
				"extra_storage": {ExpiresDate: at(t, "2026-10-01T00:00:00Z")},
			}},
			want: "",
		},
		{
			// The payment failed but the subscriber is still entitled. Reading expires_date
			// alone would take access away from someone who has it.
			name: "grace period outlasts the expiry",
			sub: subscriber{Entitlements: map[string]entitlement{
				"pro": {
					ExpiresDate:            at(t, "2026-09-04T00:00:00Z"),
					GracePeriodExpiresDate: at(t, "2026-09-20T00:00:00Z"),
				},
			}},
			want: "2026-09-20T00:00:00Z",
		},
		{
			name: "expiry outlasts a stale grace period",
			sub: subscriber{Entitlements: map[string]entitlement{
				"pro": {
					ExpiresDate:            at(t, "2026-12-01T00:00:00Z"),
					GracePeriodExpiresDate: at(t, "2026-09-04T00:00:00Z"),
				},
			}},
			want: "2026-12-01T00:00:00Z",
		},
		{
			// The trap. A null expires_date means the entitlement does not expire, and
			// reading it as the zero time would silently downgrade a lifetime purchaser to
			// the free plan — a wrong nobody would ever be looking for.
			name: "entitlement with no expiry is not expired",
			sub: subscriber{Entitlements: map[string]entitlement{
				"pro": {ExpiresDate: nil},
			}},
			want: neverExpires.Format(time.RFC3339),
		},
		{
			name: "several entitlements confer pro; the latest wins",
			sub: subscriber{Entitlements: map[string]entitlement{
				"pro":        {ExpiresDate: at(t, "2026-10-01T00:00:00Z")},
				"pro_annual": {ExpiresDate: at(t, "2027-03-01T00:00:00Z")},
			}},
			ids:  []string{"pro", "pro_annual"},
			want: "2027-03-01T00:00:00Z",
		},
		{
			name: "a non-expiring entitlement outranks a dated one",
			sub: subscriber{Entitlements: map[string]entitlement{
				"pro":        {ExpiresDate: at(t, "2026-10-01T00:00:00Z")},
				"pro_annual": {ExpiresDate: nil},
			}},
			ids:  []string{"pro", "pro_annual"},
			want: neverExpires.Format(time.RFC3339),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ids := tc.ids
			if ids == nil {
				ids = proIDs
			}
			got := proUntilFrom(tc.sub, ids)

			if tc.want == "" {
				if !got.IsZero() {
					t.Fatalf("want the zero time, got %s", got.Format(time.RFC3339))
				}
				return
			}
			want, err := time.Parse(time.RFC3339, tc.want)
			if err != nil {
				t.Fatalf("bad want %q: %v", tc.want, err)
			}
			if !got.Equal(want) {
				t.Fatalf("want %s, got %s", want.Format(time.RFC3339), got.Format(time.RFC3339))
			}
		})
	}
}

// TestProUntilFromIsIdempotent asserts the property the whole design rests on: the column
// is DERIVED from provider state, so applying the same state twice yields the same answer
// and a repeated sync is free.
func TestProUntilFromIsIdempotent(t *testing.T) {
	sub := subscriber{Entitlements: map[string]entitlement{
		"pro": {ExpiresDate: at(t, "2026-10-01T00:00:00Z")},
	}}
	first := proUntilFrom(sub, []string{"pro"})
	second := proUntilFrom(sub, []string{"pro"})
	if !first.Equal(second) {
		t.Fatalf("not idempotent: %s then %s", first, second)
	}
}
