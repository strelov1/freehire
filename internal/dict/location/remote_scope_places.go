package location

import (
	"slices"
	"strings"
)

// WithoutExcludedPlaces removes metros and cities whose country can be
// established as excluded. For a shared city name, the source's country
// context must resolve it before the exclusion can apply.
func WithoutExcludedPlaces(cities, metros, sourceCountries, excluded []string) ([]string, []string) {
	if len(excluded) == 0 {
		return cities, metros
	}
	cities = slices.DeleteFunc(slices.Clone(cities), func(city string) bool {
		name, qualifier, _ := strings.Cut(strings.ToLower(city), ",")
		qualifier = strings.TrimSpace(qualifier[strings.LastIndexByte(qualifier, ',')+1:])
		if country := namedCountry(qualifier); country != "" {
			return slices.Contains(excluded, country)
		}
		name = strings.TrimSpace(name)
		if entry, ok := cityDict[name]; ok && !entry.Contested {
			return slices.Contains(excluded, entry.Country)
		}
		found := false
		for _, country := range sourceCountries {
			if !hasPlaceIn(name, country) {
				continue
			}
			if !slices.Contains(excluded, country) {
				return false
			}
			found = true
		}
		return found
	})
	metros = slices.DeleteFunc(slices.Clone(metros), func(metro string) bool {
		return slices.ContainsFunc(excluded, func(country string) bool {
			return strings.HasSuffix(metro, " ("+strings.ToUpper(country)+")")
		})
	})
	return cities, metros
}
