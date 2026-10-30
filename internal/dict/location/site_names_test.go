package location

import (
	"slices"
	"testing"
)

// TestSiteNamesStateNoCity holds the line between a place and a site named after one. A
// K-12 board (applitrack) writes the SITE inside the district before the state — a school,
// an administration building — and the strings below are the most common ones on the
// live catalogue (2026-10-30). The countries come from the state; a city is read only
// where the line states a town, because a wrong city is worse than none.
func TestSiteNamesStateNoCity(t *testing.T) {
	tests := []struct {
		location string
		cities   []string
	}{
		// "Central Office" is the district's administration building, not Central,
		// Louisiana: "office" is cut so "San Francisco Office" reads as the city, and what
		// it left behind was a descriptor that also happens to be a town.
		{"Central Office, Ohio", nil},
		{"CENTRAL OFFICE, Illinois", nil},
		{"Branch Office, Ohio", nil},
		{"Central Regional Office, Ohio", nil},
		// ...but a town whose own name ends in a descriptor keeps it.
		{"Olive Branch Office, Mississippi", []string{"Olive Branch"}},
		// A school named after two people (or two towns) is not a "City-District" token:
		// the district rule read Scranton (through its historical name, Harrison) and
		// Lincoln out of them.
		{"Harrison-Morton Middle School, Virginia", nil},
		{"Lincoln-Way Area Special Education District 843, Illinois", nil},
		// Scotch Plains-Fanwood High School does stand in Scotch Plains, but its name says
		// so only by the luck of a regional district naming both its towns; the rule that
		// would read it is the one that read Harrison above.
		{"Scotch Plains-Fanwood High School, New Jersey", nil},
		{"Kenston High School, Ohio", nil},
		{"District Wide, Ohio", nil},

		// A town the line does state still resolves, beside a site or not.
		{"Lincoln School, Palatine, IL", []string{"Palatine"}},
		{"Administration Center - Allentown, PA", []string{"Allentown"}},
		{"Centennial High School - Peoria, AZ", []string{"Peoria"}},
		{"San Francisco Office", []string{"San Francisco"}},
	}
	for _, tc := range tests {
		t.Run(tc.location, func(t *testing.T) {
			g := ParseIn(tc.location, []string{"us"})
			if !slices.Equal(g.Cities, tc.cities) {
				t.Errorf("ParseIn(%q, us).Cities = %v, want %v", tc.location, g.Cities, tc.cities)
			}
			if !slices.Equal(g.Countries, []string{"us"}) {
				t.Errorf("ParseIn(%q, us).Countries = %v, want [us]", tc.location, g.Countries)
			}
		})
	}
}
