package location

import "testing"

// TestStateCountry pins the picklist reading of a US state or Canadian province: the full name
// resolves to its country, "Georgia" included (in a field that names a state and nothing else it
// is the state, which a free-text line cannot assume), while a postal code, a mangled value and an
// ordinary place resolve to nothing.
func TestStateCountry(t *testing.T) {
	cases := map[string]string{
		"Ohio":                   "us",
		"New Mexico":             "us",
		" district of columbia ": "us",
		"Georgia":                "us",
		"Saskatchewan":           "ca",
		"OH":                     "",
		"ON":                     "",
		"Californicredential or": "",
		"Tbilisi":                "",
		"":                       "",
	}
	for name, want := range cases {
		if got := StateCountry(name); got != want {
			t.Errorf("StateCountry(%q) = %q, want %q", name, got, want)
		}
	}
}
