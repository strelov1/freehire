package location

import "testing"

// Every beacon city the parser can emit into the `cities` facet must resolve to a
// single valid ISO 3166-1 alpha-2 country code, so the frontend can nest cities
// under their country. Coverage is over the canonical city names (the facet values).
func TestCityToCountryCoversEveryBeaconCity(t *testing.T) {
	cc := CityToCountry()

	seen := make(map[string]bool)
	for _, canonical := range nameToCity {
		if seen[canonical] {
			continue
		}
		seen[canonical] = true

		code, ok := cc[canonical]
		if !ok {
			t.Errorf("beacon city %q has no country mapping", canonical)
			continue
		}
		if len(code) != 2 {
			t.Errorf("city %q maps to non-ISO code %q", canonical, code)
		}
		if _, ok := countryToRegion[code]; !ok {
			t.Errorf("city %q country %q is not in any region", canonical, code)
		}
	}
}
