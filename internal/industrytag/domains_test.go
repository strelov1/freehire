package industrytag

import (
	"reflect"
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/vocab"
)

func TestDomainsForIndustries(t *testing.T) {
	tests := []struct {
		name       string
		industries []string
		want       []string
	}{
		{
			name:       "an industry the domain vocabulary spells differently",
			industries: []string{"developer-tools"},
			want:       []string{"devtools"},
		},
		{
			name:       "several industries, sorted and de-duplicated",
			industries: []string{"fintech", "crypto", "fintech"},
			want:       []string{"crypto", "fintech"},
		},
		{
			// The alias table already routes digital-media and media-and-entertainment
			// to entertainment, so holding the domain itself to a stricter standard was
			// the inconsistency, not the mapping.
			name:       "media resolves to the industry its own synonyms already use",
			industries: []string{"entertainment"},
			want:       []string{"media"},
		},
		{
			// Settled against NAICS: ride-hailing is 485310 under Transit and Ground
			// Passenger Transportation, distinct from 3361 Motor Vehicle Manufacturing.
			// So the mobility domain answers transportation and NOT automotive — the
			// latter would file taxi platforms under vehicle manufacturing.
			name:       "mobility answers transportation, not automotive",
			industries: []string{"transportation"},
			want:       []string{"mobility"},
		},
		{
			name:       "automotive is not reachable through the mobility domain",
			industries: []string{"automotive"},
			want:       []string{},
		},
		{
			name:       "a value outside the curated vocabulary",
			industries: []string{"saas", "", "not-an-industry"},
			want:       []string{},
		},
		{
			name:       "no industries at all",
			industries: nil,
			want:       []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DomainsForIndustries(tt.industries)
			if got == nil {
				t.Fatal("DomainsForIndustries returned nil; a caller writing this into a text[] must not have to special-case it")
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DomainsForIndustries(%q) = %q, want %q", tt.industries, got, tt.want)
			}
		})
	}
}

// The mapping may only speak in vocabularies that exist. A typo on either side would
// produce a filter fragment that can never match — silently, since an over-narrow
// filter looks exactly like a genuinely empty result.
func TestDomainIndustryTableSpeaksBothVocabularies(t *testing.T) {
	for domain, industry := range domainIndustry {
		if !slices.Contains(vocab.DomainValues, domain) {
			t.Errorf("key %q is not a vocab.DomainValues member", domain)
		}
		if _, ok := displayNames[industry]; !ok {
			t.Errorf("%q maps to %q, which is not a canonical industry", domain, industry)
		}
	}
}

// The domains left out are left out on purpose, so the omission is asserted rather
// than left to be "fixed" by someone reading the table as incomplete.
func TestDeliberatelyUnmappedDomains(t *testing.T) {
	excluded := []string{"other"}

	for _, domain := range excluded {
		if industry, ok := domainIndustry[domain]; ok {
			t.Errorf("%q maps to %q; it is meant to map to nothing (see the change design)", domain, industry)
		}
	}
	// Everything else must map: an unmapped domain costs reach silently, so a newly
	// added one should fail here rather than just quietly narrow the filter.
	for _, domain := range vocab.DomainValues {
		if slices.Contains(excluded, domain) {
			continue
		}
		if _, ok := domainIndustry[domain]; !ok {
			t.Errorf("domain %q maps to no industry; map it or add it to the deliberate exclusions", domain)
		}
	}
}

// Whatever comes out is fed to a filter over the domains column, so it must be a
// value that column can actually hold.
func TestDomainsForIndustriesEmitsOnlyRealDomains(t *testing.T) {
	for _, industry := range Canonicals() {
		for _, domain := range DomainsForIndustries([]string{industry}) {
			if !slices.Contains(vocab.DomainValues, domain) {
				t.Errorf("industry %q yielded %q, which no company's domains can contain", industry, domain)
			}
		}
	}
}
