package location

import (
	"slices"
	"strings"
)

// WithoutShadowedGlobal drops the global region from a posting's regions when the same
// posting also names a narrower place — a country, or a region other than global.
//
// An open-anywhere word beside a place almost never means "open anywhere": it is a
// requisition bucket ("Anywhere, USA"), an ATS prefix ("International - Germany") or a
// headquarters label ("Global Headquarters - Dublin, OH"). When the reading is
// ambiguous the narrower one is the safe one: a missed worldwide posting costs a
// visitor one result, a false one costs them an application.
//
// The exception is a line that joins alternatives with "or" ("Remote anywhere or On-site
// Prague"): there the posting offers anywhere as one choice, and the place is the other.
//
// line is the posting's location line, read only for that exception; "" when there is
// none. regions is returned as is (not copied) when nothing is dropped.
func WithoutShadowedGlobal(countries, regions []string, line string) []string {
	if !slices.Contains(regions, globalRegion) {
		return regions
	}
	if len(countries) == 0 && len(regions) == 1 {
		return regions
	}
	if strings.Contains(strings.ToLower(line), " or ") {
		return regions
	}
	return withoutGlobal(regions)
}
