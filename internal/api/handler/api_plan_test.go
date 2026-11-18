package handler

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/identity/billing"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestAPIPlanFrom(t *testing.T) {
	now := time.Date(2026, time.November, 4, 12, 0, 0, 0, time.UTC)
	future := pgtype.Timestamptz{Time: now.Add(time.Hour), Valid: true}
	past := pgtype.Timestamptz{Time: now.Add(-time.Hour), Valid: true}
	for _, tc := range []struct {
		name    string
		row     db.GetUserAPIEntitlementRow
		product billing.APITier
	}{
		{"nothing", db.GetUserAPIEntitlementRow{}, billing.APITierNone},
		{"pro is a separate product", db.GetUserAPIEntitlementRow{ProUntil: future}, billing.APITierNone},
		{"api", db.GetUserAPIEntitlementRow{ApiUntilStripe: future, UltraUntil: future}, billing.APITierAPI},
		{"data", db.GetUserAPIEntitlementRow{ApiUntilStripe: future, ApiDataUntilStripe: future}, billing.APITierData},
		{"expired", db.GetUserAPIEntitlementRow{ApiUntilStripe: past, ApiDataUntilStripe: past}, billing.APITierNone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := apiPlanFrom(tc.row, now)
			if got.Tier != tc.product {
				t.Fatalf("product = %q, want %q", got.Tier, tc.product)
			}
			if tc.product == billing.APITierNone {
				if got.Until != nil {
					t.Fatalf("until = %v for an inactive product", got.Until)
				}
			} else if got.Until == nil || !got.Until.Equal(future.Time) {
				t.Fatalf("until = %v, want %v", got.Until, future.Time)
			}
		})
	}
}
