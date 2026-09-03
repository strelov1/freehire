package billing

import (
	"testing"
	"time"
)

// ms is a v2 expiry: milliseconds since the epoch. Taking a string keeps the cases
// readable while the type under test stays the one the provider actually sends.
func ms(t *testing.T, s string) *int64 {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("bad test timestamp %q: %v", s, err)
	}
	out := v.UnixMilli()
	return &out
}

// sub builds a v2 customer carrying the given active entitlements.
func sub(items ...entitlement) subscriber {
	var s subscriber
	s.ActiveEntitlements.Items = items
	return s
}

// TestProUntilFrom walks every shape the provider's active-entitlement list arrives in.
//
// Note what is NOT here any more: a grace period. The v2 list carries only entitlements
// that are ACTIVE, so the provider has already applied its own grace rule and what is left
// is one expiry per entitlement. The v1 shape this replaced needed both fields and a rule
// to combine them.
func TestProUntilFrom(t *testing.T) {
	proIDs := []string{"freehire Pro"}

	cases := []struct {
		name string
		sub  subscriber
		ids  []string
		want string // RFC3339, or "" for the zero time
	}{
		{
			name: "an active entitlement",
			sub:  sub(entitlement{EntitlementID: "freehire Pro", ExpiresAt: ms(t, "2026-10-01T00:00:00Z")}),
			want: "2026-10-01T00:00:00Z",
		},
		{
			// Lapsed, refunded and transferred all look the same from here: the entitlement
			// is simply not in the ACTIVE list, and no branch was needed to notice.
			name: "no active entitlements at all",
			sub:  sub(),
			want: "",
		},
		{
			name: "only an entitlement that does not confer pro",
			sub:  sub(entitlement{EntitlementID: "extra_storage", ExpiresAt: ms(t, "2026-10-01T00:00:00Z")}),
			want: "",
		},
		{
			// The trap. A null expiry means the entitlement does not expire, and reading it
			// as the zero time would silently downgrade a lifetime purchaser — and the
			// catalogue already carries a lifetime product.
			name: "an entitlement with no expiry is not expired",
			sub:  sub(entitlement{EntitlementID: "freehire Pro", ExpiresAt: nil}),
			want: neverExpires.Format(time.RFC3339),
		},
		{
			name: "several entitlements confer pro; the latest wins",
			sub: sub(
				entitlement{EntitlementID: "freehire Pro", ExpiresAt: ms(t, "2026-10-01T00:00:00Z")},
				entitlement{EntitlementID: "entl58d5471b41", ExpiresAt: ms(t, "2027-03-01T00:00:00Z")},
			),
			ids:  []string{"freehire Pro", "entl58d5471b41"},
			want: "2027-03-01T00:00:00Z",
		},
		{
			name: "a non-expiring entitlement outranks a dated one",
			sub: sub(
				entitlement{EntitlementID: "freehire Pro", ExpiresAt: ms(t, "2026-10-01T00:00:00Z")},
				entitlement{EntitlementID: "entl58d5471b41", ExpiresAt: nil},
			),
			ids:  []string{"freehire Pro", "entl58d5471b41"},
			want: neverExpires.Format(time.RFC3339),
		},
		{
			// The payload names the entitlement with ONE of the provider's two identifiers
			// and we do not get to choose which. Configuring both is what makes that a
			// non-question rather than a production incident.
			name: "the internal id is matched as readily as the lookup key",
			sub:  sub(entitlement{EntitlementID: "entl58d5471b41", ExpiresAt: ms(t, "2026-10-01T00:00:00Z")}),
			ids:  []string{"freehire Pro", "entl58d5471b41"},
			want: "2026-10-01T00:00:00Z",
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
	s := sub(entitlement{EntitlementID: "freehire Pro", ExpiresAt: ms(t, "2026-10-01T00:00:00Z")})
	first := proUntilFrom(s, []string{"freehire Pro"})
	second := proUntilFrom(s, []string{"freehire Pro"})
	if !first.Equal(second) {
		t.Fatalf("not idempotent: %s then %s", first, second)
	}
}
