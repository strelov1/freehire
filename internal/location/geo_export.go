package location

// CityToCountry returns a map from each canonical beacon-city display name (the
// values of nameToCity — i.e. the values the `cities` facet can carry) to its ISO
// 3166-1 alpha-2 country code. It is derived from the existing dictionaries: for
// each city alias the parser resolves to a canonical name, if that same alias also
// resolves to a country, the canonical city inherits that country. Dictionary-only,
// never guessed — a canonical city with no resolvable alias is simply absent.
//
// Exported so cmd/gen-contracts can emit it to the frontend, letting the client nest
// cities under their country in the location filter tree.
func CityToCountry() map[string]string {
	out := make(map[string]string, len(nameToCity))
	for alias, canonical := range nameToCity {
		if code, ok := nameToCountry[alias]; ok {
			out[canonical] = code
		}
	}
	return out
}

// CountryToRegion returns a copy of the country→region grouping (ISO 3166-1 alpha-2
// code → controlled region value), so the frontend can group countries under their
// region in the location filter tree. It mirrors the internal grouping exactly.
func CountryToRegion() map[string]string {
	out := make(map[string]string, len(countryToRegion))
	for code, region := range countryToRegion {
		out[code] = region
	}
	return out
}
