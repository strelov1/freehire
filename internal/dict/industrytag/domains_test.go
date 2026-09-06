package industrytag

import (
	"reflect"
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/dict/vocab"
)

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

// DomainIndustryPairs is consumed as two Postgres text[] query parameters, so its
// two return values must line up positionally and cover the same domainIndustry
// table domains_test.go already asserts against elsewhere in this file.
func TestDomainIndustryPairs(t *testing.T) {
	domains, industries := DomainIndustryPairs()

	if len(domains) != len(domainIndustry) {
		t.Fatalf("len(domains) = %d, want %d (one per domainIndustry entry)", len(domains), len(domainIndustry))
	}
	if len(industries) != len(domains) {
		t.Fatalf("len(industries) = %d, want %d (parallel to domains)", len(industries), len(domains))
	}
	if !slices.IsSorted(domains) {
		t.Errorf("domains %q is not sorted", domains)
	}

	got := make(map[string]string, len(domains))
	for i, domain := range domains {
		got[domain] = industries[i]
	}
	if !reflect.DeepEqual(got, domainIndustry) {
		t.Errorf("DomainIndustryPairs() pairs = %v, want %v", got, domainIndustry)
	}
}
