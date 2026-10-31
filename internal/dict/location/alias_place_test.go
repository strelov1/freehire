package location

import (
	"slices"
	"testing"
)

// TestAliasCityAgreesWithStatedPlace holds a city read through another place's alternate
// name to the place the line states. GeoNames lists a town's old or foreign names among its
// aliases, and the city table hands an alias to the most populous place carrying it — so
// "Harrison" (Scranton's name until 1851) reached Scranton, Pennsylvania even in a line
// that says New Jersey, where a town is called Harrison today.
func TestAliasCityAgreesWithStatedPlace(t *testing.T) {
	tests := []struct {
		location string
		cities   []string
		metros   []string
	}{
		// A stated state names the town of the name in that state.
		{"Harrison, NJ", []string{"Harrison"}, []string{"New York City (US)"}},
		{"Harrison, New Jersey, United States", []string{"Harrison"}, []string{"New York City (US)"}},
		{"Harrison, NJ; Philadelphia, PA", []string{"Harrison", "Philadelphia"}, []string{"New York City (US)", "Philadelphia (US)"}},
		{"Harrison, NJ | Scranton, PA", []string{"Harrison", "Scranton"}, []string{"New York City (US)", "Scranton (US)"}},
		{"Frisco, TX; Frisco, CA", []string{"Frisco", "San Francisco"}, []string{"Dallas (US)", "San Francisco Bay Area (US)"}},
		{"Frisco; Harrison, NJ", []string{"Harrison", "San Francisco"}, []string{"New York City (US)", "San Francisco Bay Area (US)"}},
		{"Franklin, MA; El Paso, TX; Nashville, TN", []string{"El Paso", "Franklin", "Nashville"}, []string{"El Paso (US)", "Nashville (US)", "Providence (US)"}},
		{"Franklin, MA; Franklin, TN", []string{"Franklin"}, []string{"Nashville (US)", "Providence (US)"}},
		{"Morton, IL", []string{"Morton"}, []string{"Peoria (US)"}},
		{"Frisco, TX", []string{"Frisco"}, []string{"Dallas (US)"}},
		{"Apex, NC", []string{"Apex"}, []string{"Raleigh (US)"}},
		{"Salem, MA", []string{"Salem"}, []string{"Boston (US)"}},
		{"Canton, Ohio", []string{"Canton"}, []string{"Canton (US)"}},
		{"Saint John, NB", []string{"Saint John"}, nil},
		// The town of the stated state is found under the spelling the line uses.
		{"St Petersburg, FL", []string{"St. Petersburg"}, []string{"Tampa (US)"}},
		{"Saint Charles, Illinois", []string{"St. Charles"}, []string{"Chicago (US)"}},
		// A state alone names no city through a town's alias ("missouri" is one of
		// Bozeman's, in Montana).
		{"Missouri", nil, nil},
		// A stated state with no town of the name names no city: the alias's place is in
		// another state, or another country.
		{"Calcutta, OH", nil, nil},
		{"Bombay, NY", nil, nil},
		// The alias's own place, where the line states it, is untouched.
		{"Scranton, PA", []string{"Scranton"}, []string{"Scranton (US)"}},
		{"Calcutta", []string{"Kolkata"}, []string{"Kolkata (IN)"}},
		{"Bombay, India", []string{"Mumbai"}, []string{"Mumbai (IN)"}},
		{"Frisco, CA", []string{"San Francisco"}, []string{"San Francisco Bay Area (US)"}},
	}
	for _, tc := range tests {
		t.Run(tc.location, func(t *testing.T) {
			g := Parse(tc.location)
			if !slices.Equal(g.Cities, tc.cities) {
				t.Errorf("Parse(%q).Cities = %v, want %v", tc.location, g.Cities, tc.cities)
			}
			if !slices.Equal(g.Metros, tc.metros) {
				t.Errorf("Parse(%q).Metros = %v, want %v", tc.location, g.Metros, tc.metros)
			}
		})
	}
}
