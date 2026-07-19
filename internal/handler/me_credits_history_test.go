package handler

import "testing"

// creditEntryLabel maps a ledger row's (kind, feature, resolved subject) onto the display
// label + subtitle shown on the Credits history page. Cover every kind, the two debit
// features, the missing-subject fallback, and an unrecognized kind.
func TestCreditEntryLabel(t *testing.T) {
	cases := []struct {
		name         string
		kind         string
		feature      string
		subject      string
		wantLabel    string
		wantSubtitle string
	}{
		{"monthly grant", "grant", "", "", "Monthly grant", ""},
		{"contribution reward", "reward", "", "", "Board contribution", ""},
		{"credit purchase", "purchase", "", "", "Credit purchase", ""},
		{"match debit names the job", "debit", "match", "Senior Go Engineer", "Match analysis", "Senior Go Engineer"},
		{"tailor debit names the job", "debit", "tailor", "Platform Engineer", "CV tailoring", "Platform Engineer"},
		{"match debit with deleted job", "debit", "match", "", "Match analysis", ""},
		{"debit with unknown feature", "debit", "", "", "Credit used", ""},
		{"unrecognized kind", "mystery", "", "", "Credit adjustment", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			label, subtitle := creditEntryLabel(tc.kind, tc.feature, tc.subject)
			if label != tc.wantLabel {
				t.Errorf("label = %q, want %q", label, tc.wantLabel)
			}
			if subtitle != tc.wantSubtitle {
				t.Errorf("subtitle = %q, want %q", subtitle, tc.wantSubtitle)
			}
		})
	}
}
