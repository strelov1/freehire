package jobderive

import (
	"slices"
	"testing"
)

// Derive carries the engagement terms the title or description states, so every write
// path that derives — ingest, moderator authoring, the employer form, a pasted private
// JD and backfill-derive — fills the column without opting in.
func TestDerive_Engagement(t *testing.T) {
	got := Derive(Input{Title: "Data Engineer - Outside IR35", Description: "<p>Rate: £550/day.</p>"}).Engagement
	if want := []string{"ir35_outside"}; !slices.Equal(got, want) {
		t.Errorf("Engagement from the title = %v, want %v", got, want)
	}
	got = Derive(Input{Title: "Data Engineer", Description: "<p>IR35 Status: Inside</p>"}).Engagement
	if want := []string{"ir35_inside"}; !slices.Equal(got, want) {
		t.Errorf("Engagement from the description = %v, want %v", got, want)
	}
	if got := Derive(Input{Title: "Backend Engineer", Description: "Write Go."}).Engagement; got != nil {
		t.Errorf("Engagement with none stated = %v, want nil", got)
	}
}
