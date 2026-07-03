package location

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
