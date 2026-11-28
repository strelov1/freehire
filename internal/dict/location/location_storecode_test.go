package location

import (
	"reflect"
	"testing"
)

// TestParseStoreCodePrefix: Workday's Texas Roadhouse lines glue a one-letter store code
// to the town ("J-Florence,KY"). Florence read as the Italian city and "J-Columbus, IN"
// as India, so a US restaurant manager surfaced under countries=IT.
func TestParseStoreCodePrefix(t *testing.T) {
	for loc, want := range map[string][]string{
		"J-Florence,KY":            {"us"},
		"J-Columbus, IN":           {"us"},
		"J-Jeffersonville,IN":      {"us"},
		"J-Louisville-Westport,KY": {"us"},
		"J-Greenwood, IN":          {"us"},
		"Florence, KY":             {"us"},
		"Florence, Italy":          {"it"},
		"s-Hertogenbosch":          {"nl"},
		"'s-Hertogenbosch, NL":     {"nl"},
	} {
		if got := Parse(loc).Countries; !reflect.DeepEqual(got, want) {
			t.Errorf("Parse(%q).Countries = %v, want %v", loc, got, want)
		}
	}
}

// TestParseStoreCodeKeepsPlaceNames: a place whose own name is a letter, a hyphen and a
// word keeps its city; only an unknown "x-town" token loses the letter.
func TestParseStoreCodeKeepsPlaceNames(t *testing.T) {
	for loc, want := range map[string][]string{
		"L-Iklin, Malta":                        {"L-Iklin"},
		"X-Can, Mexico":                         {"X-Cán"},
		"s-Gravendeel, Netherlands":             {"s-Gravendeel"},
		"s-Gravendeel en omgeving, Netherlands": {"s-Gravendeel"},
		"J-Florence, KY":                        {"Florence"},
	} {
		if got := Parse(loc).Cities; !reflect.DeepEqual(got, want) {
			t.Errorf("Parse(%q).Cities = %v, want %v", loc, got, want)
		}
	}
}
