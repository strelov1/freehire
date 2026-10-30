package normalize

import "testing"

func TestSlugPredatesRule(t *testing.T) {
	for _, tc := range []struct {
		stored, name string
		want         bool
	}{
		{"acme-pvt-ltd", "Acme Pvt Ltd", true}, // the plain slug, a form the rule now strips
		{"acme-pvt", "Acme Pvt Ltd", true},     // a half-stripped slug from an older rule
		{"acme", "Acme Pvt Ltd", false},        // already the current key
		{"oldco", "NewCo", false},              // renamed: no rule made this slug of this name
		{"acme-p", "Acme Pvt Ltd", false},      // cut inside a word
		{"acme-holdings", "Acme Pvt Ltd", false},
		{"ac-me", "Acme", false}, // the rule changes nothing here, so another spelling did
		// The South Slavic forms read as written: a lone company keyed before them is planned
		// (and aliased) by merge-companies and backfill-derive alike.
		{"lidl-hrvatska-d-o-o-k-d", "Lidl Hrvatska d.o.o. k.d.", true},
		{"adria-mobil-d-o-o", "Adria Mobil, d. o. o.", true},
		{"banca-intesa-a-d", "Banca Intesa a.d.", true},
		{"ragazzo-brooklin-sp", "Ragazzo Brooklin SP", false}, // a bare SP is no form
	} {
		if got := SlugPredatesRule(tc.stored, tc.name); got != tc.want {
			t.Errorf("SlugPredatesRule(%q, %q) = %v, want %v", tc.stored, tc.name, got, tc.want)
		}
	}
}
