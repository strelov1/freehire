package jobderive

import (
	"slices"
	"testing"
)

// Derive carries the perks the description states, so every write path that derives
// — ingest, moderator authoring, the employer form, a pasted private JD and
// backfill-derive — fills the column without opting in.
func TestDerive_Perks(t *testing.T) {
	got := Derive(Input{
		Title:       "Backend Engineer",
		Description: "<p>Benefits: Unlimited PTO, 401(k) with company match, a sabbatical.</p>",
	}).Perks
	want := []string{"generous_pto", "401k_match", "retirement_plan", "unlimited_pto", "sabbatical"}
	if !slices.Equal(got, want) {
		t.Errorf("Perks = %v, want %v", got, want)
	}
	if got := Derive(Input{Title: "Backend Engineer", Description: "Write Go."}).Perks; got != nil {
		t.Errorf("Perks with none stated = %v, want nil", got)
	}
}
