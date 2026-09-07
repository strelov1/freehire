package discordlink

import (
	"testing"

	"github.com/strelov1/freehire/internal/ai/plan"
)

func TestWarrantsPaidRole(t *testing.T) {
	for _, tc := range []struct {
		tier plan.Tier
		want bool
	}{
		{plan.TierFree, false},
		{plan.TierPro, true},
		{plan.TierUltra, true},
	} {
		if got := WarrantsPaidRole(tc.tier); got != tc.want {
			t.Errorf("WarrantsPaidRole(%v) = %v, want %v", tc.tier, got, tc.want)
		}
	}
}

// The four cases are the whole of reconciliation. Two of them do nothing, and that is the
// property worth protecting: a run over a catalogue that has not moved must make no calls
// to Discord at all, or the hourly timer turns into an hourly write of every role we manage.
func TestReconcile(t *testing.T) {
	for _, tc := range []struct {
		name        string
		tier        plan.Tier
		roleGranted bool
		want        Action
	}{
		{"paying without the role", plan.TierPro, false, ActionGrant},
		{"paying with the role", plan.TierUltra, true, ActionNone},
		{"lapsed with the role", plan.TierFree, true, ActionRevoke},
		{"lapsed without the role", plan.TierFree, false, ActionNone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Reconcile(tc.tier, tc.roleGranted); got != tc.want {
				t.Errorf("Reconcile(%v, %v) = %v, want %v", tc.tier, tc.roleGranted, got, tc.want)
			}
		})
	}
}
