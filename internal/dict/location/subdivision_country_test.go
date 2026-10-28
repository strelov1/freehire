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
		// A code that spells the closing country is that country, not a subdivision to drop.
		{"Herzliya, IL, IL", []string{"il", "us"}},
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
