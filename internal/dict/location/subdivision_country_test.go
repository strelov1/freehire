package location

import (
	"reflect"
	"testing"
)

// TestSubdivisionCodeBeforeAnotherCountry pins that a US/CA or Australian subdivision code
// written between a town and a DIFFERENT country the line names is that country's own
// subdivision, not a second country: "Breda, NB, Netherlands" is Noord-Brabant, not New
// Brunswick (an API customer's countries=ca returned it), "Milano, MI, Italia" the province
// of Milan, not Michigan. The ordinary North American shape is untouched.
func TestSubdivisionCodeBeforeAnotherCountry(t *testing.T) {
	tests := []struct {
		location  string
		countries []string
	}{
		{"Breda, NB, Netherlands", []string{"nl"}},
		{"Milano, MI, Italia", []string{"it"}},
		{"Torino, TO, Italy", []string{"it"}},
		{"Barcelona, CT, Spain", []string{"es"}},
		{"Tijuana, BC, Mexico", []string{"mx"}},
		{"Sydney, NS, AU", []string{"au"}},
		{"Breda, NB, Netherlands; Austin, TX, US", []string{"nl", "us"}},
		// The token after the code must close the location: here it is a city that the
		// curated table also knows, and the line is a repeated US location.
		{"Naples, FL, Naples, FL, US", []string{"us"}},
		// A closing city is not a closing country, even one the curated table places.
		{"London, ON, Manchester", []string{"ca", "gb"}},
		// A code that spells the closing country is that country, not a subdivision to drop
		// (and see TestCountryCodeClosingCityLocation for why it is no longer Illinois too).
		{"Herzliya, IL, IL", []string{"il"}},
		// The same codes in their own country, or with no country after them.
		{"Moncton, NB, Canada", []string{"ca"}},
		{"Moncton, NB", []string{"ca"}},
		{"Detroit, MI, USA", []string{"us"}},
		{"Detroit, MI", []string{"us"}},
		{"Seattle, WA", []string{"us"}},
		{"Perth, WA, Australia", []string{"au"}},
		// A town the dictionary puts in the subdivision's own country keeps that reading:
		// the trailing country is a second location, not the town's.
		{"Austin, TX, Canada", []string{"ca", "us"}},
		// A town nobody knows proves nothing, so nothing is withdrawn.
		{"Zzyzxville, NB, Netherlands", []string{"ca", "nl"}},
	}
	for _, tt := range tests {
		if g := Parse(tt.location); !reflect.DeepEqual(g.Countries, tt.countries) {
			t.Errorf("Parse(%q) countries = %v, want %v", tt.location, g.Countries, tt.countries)
		}
	}
}

// TestCountryCodeClosingCityLocation pins that a code which is both a US/CA subdivision
// and an ISO country code is the COUNTRY when it closes a location that opens with a city
// the dictionary places in that country: "Toronto, CA, CA" and "Toronto, ON, CA" are
// Canada, not California, and "Tel Aviv, IL, IL" is Israel, not Illinois. A city the
// dictionary places in the US keeps the state, and the two-token "City, CA" shape is
// untouched.
func TestCountryCodeClosingCityLocation(t *testing.T) {
	tests := []struct {
		location  string
		countries []string
	}{
		{"Toronto, CA, CA", []string{"ca"}},
		{"Montreal, CA, CA", []string{"ca"}},
		{"Toronto, ON, CA", []string{"ca"}},
		{"Vancouver, BC, CA", []string{"ca"}},
		{"Montreal, QC, CA", []string{"ca"}},
		{"Toronto, CA, Canada", []string{"ca"}},
		{"Tel Aviv, IL, IL", []string{"il"}},
		// After a Canadian province the code is Canada even when the town's first reading
		// is elsewhere: London, Ontario; Sydney, Nova Scotia.
		{"London, ON, CA", []string{"ca"}},
		{"Perth, ON, CA", []string{"ca"}},
		{"Sydney, NS, CA", []string{"ca"}},
		{"Toronto, ON, CA; Austin, TX, US", []string{"ca", "us"}},
		// Guards: a US city, the two-token shape, and a US state between them.
		{"San Jose, CA", []string{"us"}},
		{"San Jose, CA, CA", []string{"us"}},
		{"San Jose, CA, US", []string{"us"}},
		{"Los Angeles, CA, US", []string{"us"}},
		{"Toronto, CA, US", []string{"us"}},
		{"Toronto, TX, CA", []string{"ca", "us"}},
		{"London, ON, Canada", []string{"ca"}},
		{"Lima, PE, PE", []string{"pe"}},
		{"Bratislava, SK, SK", []string{"sk"}},
	}
	for _, tt := range tests {
		if g := Parse(tt.location); !reflect.DeepEqual(g.Countries, tt.countries) {
			t.Errorf("Parse(%q) countries = %v, want %v", tt.location, g.Countries, tt.countries)
		}
	}
}

// TestUSStateAndUSDropTownNamesake pins that a location naming a US state AND the United
// States places a town there even when a foreign city shares its name: "Valencia, CA, US"
// is Valencia, California, not also Spain. Without the closing country the namesake was
// already withdrawn ("Paris, TX"); with it, the state code was read as the closing
// country's subdivision (or, for a code that also spells a country, as the state) and the
// withdrawal was skipped.
func TestUSStateAndUSDropTownNamesake(t *testing.T) {
	tests := []struct {
		location  string
		countries []string
	}{
		{"Valencia, CA, US", []string{"us"}},
		{"Dublin, CA, US", []string{"us"}},
		{"Dublin, CA, USA", []string{"us"}},
		{"Valencia, CA, United States", []string{"us"}},
		{"Valencia, PA, US", []string{"us"}},
		{"Paris, IL, US", []string{"us"}},
		{"Berlin, MD, US", []string{"us"}},
		{"Moscow, ID, US", []string{"us"}},
		{"Florence, AL, US", []string{"us"}},
		{"Milan, TN, US", []string{"us"}},
		{"Cairo, IL, US", []string{"us"}},
		{"Lisbon, ME, US", []string{"us"}},
		// Guards that already held.
		{"Paris, TX", []string{"us"}},
		{"Paris, TX, US", []string{"us"}},
		{"Dublin, OH, US", []string{"us"}},
		{"Cambridge, MA", []string{"us"}},
		{"Melbourne, FL", []string{"us"}},
		{"Melbourne, FL, US", []string{"us"}},
		{"Dublin, Ireland", []string{"ie"}},
		{"Valencia, Spain", []string{"es"}},
		// Another location in the line still states the foreign country.
		{"Valencia, Spain; Valencia, CA, US", []string{"es", "us"}},
		// An unknown town proves nothing either way.
		{"Zzyzxville, CA, US", []string{"us"}},
		// A town the US has no place of is not a namesake, and a list of codes names no
		// town at all.
		{"Jakarta, ID, United States", []string{"id", "us"}},
		{"CL, CO, MX, US", []string{"cl", "mx", "us"}},
		// A colliding province code before Canada stays the province.
		{"Estevan, SK, Canada", []string{"ca"}},
		{"Estevan, SK, CA", []string{"ca"}},
	}
	for _, tt := range tests {
		if g := Parse(tt.location); !reflect.DeepEqual(g.Countries, tt.countries) {
			t.Errorf("Parse(%q) countries = %v, want %v", tt.location, g.Countries, tt.countries)
		}
	}
}
