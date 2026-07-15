package sources

import "testing"

// TestProviderKind pins the status-page taxonomy to the adapter markers: a
// board-based platform is an ATS, a boardless single-company adapter is that
// company's own careers page, a boardless many-company adapter is an aggregator,
// and an unregistered provider is "other".
func TestProviderKind(t *testing.T) {
	cases := map[string]string{
		"greenhouse":                KindATS,
		"workday":                   KindATS,
		"lever":                     KindATS,
		"apple":                     KindCompany,
		"google":                    KindCompany,
		"jobstash":                  KindAggregator,
		"justjoin":                  KindAggregator,
		"definitely-not-a-provider": KindOther,
	}
	reg := All(nil)
	for provider, want := range cases {
		if got := ProviderKind(reg, provider); got != want {
			t.Errorf("ProviderKind(%q) = %q, want %q", provider, got, want)
		}
	}
}
