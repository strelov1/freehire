package location

import (
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/vocab"
)

// TestDictionariesStayInVocabulary pins the hand-maintained dictionaries to the
// enrichment contract's vocabularies exhaustively (not just for sampled inputs):
// every region code the parser can ever emit is a member of vocab.RegionValues,
// and every country code is a plausible ISO 3166-1 alpha-2 (two lowercase
// letters). It guards against a future dictionary edit introducing a region the
// search facet would expose but the enrichment path would Sanitize away.
func TestDictionariesStayInVocabulary(t *testing.T) {
	for region, codes := range regionCountries {
		if !slices.Contains(vocab.RegionValues, region) {
			t.Errorf("regionCountries key %q is not in vocab.RegionValues", region)
		}
		for _, code := range codes {
			if !isAlpha2(code) {
				t.Errorf("regionCountries[%q] has non-alpha2 country code %q", region, code)
			}
		}
	}

	for name, code := range nameToCountry {
		if !isAlpha2(code) {
			t.Errorf("nameToCountry[%q] = %q is not a two-letter lowercase code", name, code)
		}
		if _, ok := countryToRegion[code]; !ok {
			t.Errorf("nameToCountry[%q] = %q has no region in countryToRegion", name, code)
		}
	}

	for name, region := range nameToRegion {
		if !slices.Contains(vocab.RegionValues, region) {
			t.Errorf("nameToRegion[%q] = %q is not in vocab.RegionValues", name, region)
		}
	}
}

// TestEveryPlaceableCountryHasAName closes the other half of the invariant
// TestDictionariesStayInVocabulary opens. That test walks nameToCountry and demands a
// region for every name; this one walks countryToRegion and demands a name for every
// code. A code with a region but no name is a country the parser can place once
// something else has identified it, yet can never identify itself — so a posting or a
// CV that spells the country out ("Honduras", "Rwanda") resolves to nothing.
//
// The drift is silent from the outside: an unresolvable country and a country the
// dictionary was never meant to cover look identical (both emit nothing), which is why
// it needs a test rather than review.
func TestEveryPlaceableCountryHasAName(t *testing.T) {
	named := make(map[string]struct{}, len(nameToCountry))
	for _, code := range nameToCountry {
		named[code] = struct{}{}
	}
	for code := range countryToRegion {
		if _, ok := named[code]; !ok {
			t.Errorf("countryToRegion has %q but no name in nameToCountry resolves to it", code)
		}
	}
}

func isAlpha2(s string) bool {
	if len(s) != 2 {
		return false
	}
	for _, r := range s {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	return true
}
