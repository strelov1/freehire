package handler

import (
	"testing"

	"github.com/strelov1/freehire/internal/ai/plan"
)

// usageEntryLabel maps a ledger row's (kind, feature, resolved subject) onto the label and
// subtitle the history page shows. Cover every metered feature, a release, the
// missing-subject fallback, and a feature this build does not know.
func TestUsageEntryLabel(t *testing.T) {
	cases := []struct {
		name         string
		kind         string
		feature      plan.Feature
		subject      string
		wantLabel    string
		wantSubtitle string
	}{
		{"fit analysis names the job", "consume", plan.FeatureFit, "Senior Go Engineer", "Job analysis", "Senior Go Engineer"},
		{"tailoring names the vacancy", "consume", plan.FeatureTailor, "Platform Engineer", "CV editing session", "Platform Engineer"},
		{"analysis whose job was deleted", "consume", plan.FeatureFit, "", "Job analysis", ""},
		{"assistant message names nothing", "consume", plan.FeatureAssistant, "", "Assistant message", ""},
		{"dictation names nothing", "consume", plan.FeatureDictation, "", "Dictation", ""},
		// A release reads as a release whatever it gave back, because what the reader needs
		// to understand is that they were not charged for it.
		{"a returned reservation", "release", plan.FeatureFit, "Senior Go Engineer", "Returned — nothing was produced", "Senior Go Engineer"},
		{"a feature this build does not know", "consume", plan.Feature("brand-new"), "", "Used", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			label, subtitle := usageEntryLabel(tc.kind, tc.feature, tc.subject)
			if label != tc.wantLabel {
				t.Errorf("label = %q, want %q", label, tc.wantLabel)
			}
			if subtitle != tc.wantSubtitle {
				t.Errorf("subtitle = %q, want %q", subtitle, tc.wantSubtitle)
			}
		})
	}
}
