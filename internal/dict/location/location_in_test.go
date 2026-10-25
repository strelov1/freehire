package location

import (
	"slices"
	"testing"
)

// Indian location lines. Every input is a real location field from the Indian part of
// the catalogue (naukri, zohorecruit, keka, freshteam, workday, oracle, greenhouse and
// others; sampled 2026-10-25) unless the case says otherwise.
func TestParseIndianLocations(t *testing.T) {
	tests := []struct {
		name      string
		location  string
		countries []string
		cities    []string
		metros    []string
		workMode  string
	}{
		// The capital region names a metro, not a city: an NCR posting may be in Delhi,
		// Gurugram or Noida, so naming one of them would be a guess.
		{"Delhi NCR", "Delhi NCR", []string{"in"}, nil, []string{"Delhi (IN)"}, ""},
		{"bare NCR", "NCR", []string{"in"}, nil, []string{"Delhi (IN)"}, ""},
		{"NCR with the country", "India, Delhi NCR", []string{"in"}, nil, []string{"Delhi (IN)"}, ""},
		{"Greater Delhi Area", "Greater Delhi Area", []string{"in"}, nil, []string{"Delhi (IN)"}, ""},
		{"Delhi and NCR", "Delhi / NCR", []string{"in"}, []string{"Delhi"}, []string{"Delhi (IN)"}, ""},
		{"Mumbai Metropolitan Region", "Mumbai Metropolitan Region, India", []string{"in"}, nil, []string{"Mumbai (IN)"}, ""},

		// Country-wide phrasings state the country and no city, and "anywhere" inside
		// them is not a remote marker: a pan-India posting is deployable anywhere.
		{"Pan India", "Pan India", []string{"in"}, nil, nil, ""},
		{"Pan-India", "Pan-India", []string{"in"}, nil, nil, ""},
		{"Anywhere in India", "Anywhere in India", []string{"in"}, nil, nil, ""},
		{"remote across India stays remote", "Remote - Anywhere in India", []string{"in"}, nil, nil, "remote"},

		// Old city names whose GeoNames alias other countries also claim.
		{"Faridabad", "Faridabad", []string{"in"}, []string{"Faridabad"}, []string{"Delhi (IN)"}, ""},
		{"Madras", "Madras", []string{"in"}, []string{"Chennai"}, []string{"Chennai (IN)"}, ""},
		{"Calcutta", "Calcutta", []string{"in"}, []string{"Kolkata"}, []string{"Kolkata (IN)"}, ""},
		{"Lucknow", "Lucknow", []string{"in"}, []string{"Lucknow"}, []string{"Lucknow (IN)"}, ""},
		{"Bombay", "Bombay", []string{"in"}, []string{"Mumbai"}, []string{"Mumbai (IN)"}, ""},
		{"Gurgaon", "Gurgaon, Haryana", []string{"in"}, []string{"Gurugram"}, []string{"Delhi (IN)"}, ""},

		// GeoNames files these names under a neighbouring place.
		{"Visakhapatnam", "Visakhapatnam, India", []string{"in"}, []string{"Visakhapatnam"}, []string{"Visakhapatnam (IN)"}, ""},
		{"Vizag", "Vizag", []string{"in"}, []string{"Visakhapatnam"}, []string{"Visakhapatnam (IN)"}, ""},
		{"Secunderabad", "Secunderabad", []string{"in"}, []string{"Secunderabad"}, []string{"Hyderabad (IN)"}, ""},
		{"Hyderabad and Secunderabad", "Hyderabad/Secunderabad", []string{"in"}, []string{"Hyderabad", "Secunderabad"}, []string{"Hyderabad (IN)"}, ""},

		// Indian names are written without the transliteration's macrons.
		{"Thane", "Thane, Maharashtra, India", []string{"in"}, []string{"Thane"}, []string{"Mumbai (IN)"}, ""},
		{"Ghaziabad", "Ghaziabad", []string{"in"}, []string{"Ghaziabad"}, []string{"Delhi (IN)"}, ""},
		{"Rajkot", "Rajkot", []string{"in"}, []string{"Rajkot"}, []string{"Rajkot (IN)"}, ""},
		{"Hosur", "Hosur", []string{"in"}, []string{"Hosur"}, []string{"Bengaluru (IN)"}, ""},

		// The code exports: country, state, city, office — dash-, dot- or space-joined.
		{"IN-KA-Bangalore", "IN-KA-Bangalore", []string{"in"}, []string{"Bengaluru"}, []string{"Bengaluru (IN)"}, ""},
		{"IN-HR-Gurgaon", "IN-HR-Gurgaon", []string{"in"}, []string{"Gurugram"}, []string{"Delhi (IN)"}, ""},
		{"IND-MM-Mumbai", "IND-MM-Mumbai", []string{"in"}, []string{"Mumbai"}, []string{"Mumbai (IN)"}, ""},
		{"IN-DL-New Delhi", "IN-DL-New Delhi", []string{"in"}, []string{"Delhi"}, []string{"Delhi (IN)"}, ""},
		{"India-Karnataka-Bangalore", "India-Karnataka-Bangalore", []string{"in"}, []string{"Bengaluru"}, []string{"Bengaluru (IN)"}, ""},
		{"India-Telangana-Hyderabad", "India-Telangana-Hyderabad", []string{"in"}, []string{"Hyderabad"}, []string{"Hyderabad (IN)"}, ""},
		{"IN-Goa is a state", "IN-Goa", []string{"in"}, nil, nil, ""},
		{"IND-Pune-Smartworks", "IND-Pune-Smartworks", []string{"in"}, []string{"Pune"}, []string{"Pune (IN)"}, ""},
		{"IN-KA-Bengaluru with an office", "IN-KA-Bengaluru-Northgate", []string{"in"}, []string{"Bengaluru"}, []string{"Bengaluru (IN)"}, ""},
		{"IN MH Mumbai Eureka", "IN MH Mumbai Eureka", []string{"in"}, []string{"Mumbai"}, []string{"Mumbai (IN)"}, ""},
		{"in.tn.chennai", "IN.TN.Chennai.Guindy", []string{"in"}, []string{"Chennai"}, []string{"Chennai (IN)"}, ""},
		{"India Mumbai", "India Mumbai", []string{"in"}, []string{"Mumbai"}, []string{"Mumbai (IN)"}, ""},
		{"India- Gurugram", "India- Gurugram", []string{"in"}, []string{"Gurugram"}, []string{"Delhi (IN)"}, ""},

		// A city's revenue divisions and municipal forms are the city.
		{"Bangalore North", "Bangalore North, India", []string{"in"}, []string{"Bengaluru"}, []string{"Bengaluru (IN)"}, ""},
		{"Bengaluru East", "Bengaluru East, Karnataka, India", []string{"in"}, []string{"Bengaluru"}, []string{"Bengaluru (IN)"}, ""},
		{"Pune City", "Pune City, India", []string{"in"}, []string{"Pune"}, []string{"Pune (IN)"}, ""},
		{"Coimbatore South", "Coimbatore South, India", []string{"in"}, []string{"Coimbatore"}, []string{"Coimbatore (IN)"}, ""},
		{"Chennai City Corporation", "Chennai City Corporation, India", []string{"in"}, []string{"Chennai"}, []string{"Chennai (IN)"}, ""},
		{"Mumbai Suburban", "Mumbai Suburban", []string{"in"}, []string{"Mumbai"}, []string{"Mumbai (IN)"}, ""},
		{"Bangalore Urban", "Bangalore Urban", []string{"in"}, []string{"Bengaluru"}, []string{"Bengaluru (IN)"}, ""},
		{"Bangalore Metropolitan Area", "Bangalore Metropolitan Area, Karnataka, India", []string{"in"}, []string{"Bengaluru"}, []string{"Bengaluru (IN)"}, ""},
		// Bangalore Rural is a different district, around Doddaballapura.
		{"Bangalore Rural is not the city", "Bangalore Rural, India", []string{"in"}, nil, nil, ""},

		// An office named after its city.
		{"Bengaluru Bagmane", "Bengaluru Bagmane", []string{"in"}, []string{"Bengaluru"}, []string{"Bengaluru (IN)"}, ""},
		{"Chennai DLF", "Chennai DLF", []string{"in"}, []string{"Chennai"}, []string{"Chennai (IN)"}, ""},
		{"New Delhi office", "New Delhi IBU", []string{"in"}, []string{"Delhi"}, []string{"Delhi (IN)"}, ""},

		// Several cities, and states alone.
		{"three cities", "Bengaluru, Hyderabad, Pune", []string{"in"}, []string{"Bengaluru", "Hyderabad", "Pune"}, []string{"Bengaluru (IN)", "Hyderabad (IN)", "Pune (IN)"}, ""},
		{"naukri all areas", "Mumbai; Navi Mumbai; Mumbai (All Areas)", []string{"in"}, []string{"Mumbai", "Navi Mumbai"}, []string{"Mumbai (IN)"}, ""},
		{"a state alone", "Karnataka, India", []string{"in"}, nil, nil, ""},
		{"remote India", "Remote - India", []string{"in"}, nil, nil, "remote"},

		// IN beside a stated United States is Indiana, whatever the city.
		{"Whitestown IN US", "Whitestown, IN, US", []string{"us"}, nil, nil, ""},
		{"Hobart IN United States", "Hobart, IN, United States", []string{"us"}, nil, nil, ""},
		{"Bangalore IN stays India", "Bangalore, IN", []string{"in"}, []string{"Bengaluru"}, []string{"Bengaluru (IN)"}, ""},
		{"Carmel IN stays Indiana", "Carmel, IN", []string{"us"}, []string{"Carmel"}, nil, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := Parse(tc.location)
			if !slices.Equal(g.Countries, tc.countries) {
				t.Errorf("Parse(%q).Countries = %v, want %v", tc.location, g.Countries, tc.countries)
			}
			if tc.cities != nil || len(tc.countries) == 0 || tc.countries[0] == "in" {
				if !slices.Equal(g.Cities, tc.cities) {
					t.Errorf("Parse(%q).Cities = %v, want %v", tc.location, g.Cities, tc.cities)
				}
			}
			if tc.countries != nil && tc.countries[0] == "in" && !slices.Equal(g.Metros, tc.metros) {
				t.Errorf("Parse(%q).Metros = %v, want %v", tc.location, g.Metros, tc.metros)
			}
			if g.WorkMode != tc.workMode {
				t.Errorf("Parse(%q).WorkMode = %q, want %q", tc.location, g.WorkMode, tc.workMode)
			}
		})
	}
}
