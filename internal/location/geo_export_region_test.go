package location

import "testing"

// The exported country→region map must cover every country the dictionary groups,
// each mapping to exactly one controlled region value — so the frontend can group
// countries under their region without ambiguity.
func TestCountryToRegionIsOneRegionPerCountry(t *testing.T) {
	cr := CountryToRegion()

	validRegion := make(map[string]bool)
	occurrences := make(map[string]int)
	for region, codes := range regionCountries {
		validRegion[region] = true
		for _, code := range codes {
			occurrences[code]++
		}
	}

	for code, n := range occurrences {
		if n != 1 {
			t.Errorf("country %q appears in %d regions, want exactly 1", code, n)
		}
		region, ok := cr[code]
		if !ok {
			t.Errorf("country %q missing from CountryToRegion export", code)
			continue
		}
		if !validRegion[region] {
			t.Errorf("country %q maps to unknown region %q", code, region)
		}
	}

	if len(cr) != len(occurrences) {
		t.Errorf("export covers %d countries, grouping has %d", len(cr), len(occurrences))
	}
}
