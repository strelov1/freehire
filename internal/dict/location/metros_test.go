package location

import (
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestParseMetros(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"Folsom, CA", []string{"Sacramento (US)"}},
		{"Roseville, California, United States", []string{"Sacramento (US)"}},
		{"Roseville, MI", []string{"Detroit (US)"}},
		{"Roseville", nil},                      // three US Rosevilles, three metros
		{"Folsom", []string{"Sacramento (US)"}}, // one US Folsom in the table
		{"Khimki, Russia", []string{"Moscow (RU)"}},
		{"San Diego, CA", []string{"San Diego (US)"}},
		{"Remote", nil},
		{"Unknownville, CA", nil},
		{"Washington, DC", []string{"Washington (US)"}},
		// Two stated subdivisions: no bare-name fallback, so Folsom (which is not in PA
		// or TX) gets no metro while the cities each state does place still do.
		{"Folsom, PA; Austin, TX", []string{"Austin (US)"}},
		{"Folsom, Pennsylvania, USA / Seattle, WA", []string{"Seattle (US)"}},
		// The table records places in no metro, so a bare name shared with one is not
		// "unique": Jackson MS and Gainesville FL lie in no GHS-FUA, while a namesake
		// elsewhere does, and the bare name must not borrow the namesake's metro.
		{"Jackson", nil},
		{"Jackson, USA", nil},
		{"Jackson, NJ", []string{"Philadelphia (US)"}},
		{"Jackson, MS", nil},
		{"Gainesville, USA", nil},
		{"Dover, US", nil},
		{"Decatur, United States", nil},
		{"Elk Grove, CA", []string{"Sacramento (US)"}},
		{"Rancho Cordova, CA", []string{"Sacramento (US)"}},
		{"Podolsk, Russia", []string{"Moscow (RU)"}},
		{"Sumaré, Brazil", []string{"Campinas (BR)"}},
		{"Hortolândia, Brazil", []string{"Campinas (BR)"}},
	}
	for _, c := range cases {
		if got := Parse(c.in).Metros; !slices.Equal(got, c.want) {
			t.Errorf("Parse(%q).Metros = %v, want %v", c.in, got, c.want)
		}
	}
}

// Every subdivision the parser places in a country must also yield a code, or a
// "City, <that subdivision>" line silently loses its metro.
func TestEverySubdivisionHasACode(t *testing.T) {
	for tok := range subdivisionToCountry {
		if _, ok := subdivisionCode[tok]; !ok {
			t.Errorf("subdivision %q has a country but no code", tok)
		}
	}
}

func TestMetroTableLoads(t *testing.T) {
	if len(metroByPlace) < 10000 {
		t.Fatalf("metro table has %d places — embed or parse broken", len(metroByPlace))
	}
}

func TestLoadMetrosBareNameAmbiguity(t *testing.T) {
	tsv := "# comment\n" +
		"roseville\tus\tCA\tSacramento (US)\n" +
		"roseville\tus\tMI\tDetroit (US)\n" +
		"folsom\tus\tCA\tSacramento (US)\n"
	exact, byName := loadMetros(tsv)
	if _, ok := byName[[2]string{"roseville", "us"}]; ok {
		t.Error("ambiguous bare name roseville must not resolve")
	}
	if got := byName[[2]string{"folsom", "us"}]; got != "Sacramento (US)" {
		t.Errorf("folsom bare = %q", got)
	}
	if got := exact[metroKey{"roseville", "us", "MI"}]; got != "Detroit (US)" {
		t.Errorf("exact roseville MI = %q", got)
	}
}

// "No metro" is a value of its own on both keys: a bare name shared by a place in a
// metro and a place in none resolves to nothing, and so does an exact key repeated with
// and without one. A key whose only value is "no metro" stays, mapped to "".
func TestLoadMetrosNoMetroIsAValue(t *testing.T) {
	tsv := "jackson\tus\tMS\t\n" +
		"jackson\tus\tNJ\tPhiladelphia (US)\n" +
		"dover\tus\tNJ\tNew York City (US)\n" +
		"dover\tus\tNJ\t\n" +
		"tyler\tus\tTX\t\n"
	exact, byName := loadMetros(tsv)
	for _, k := range [][2]string{{"jackson", "us"}, {"dover", "us"}} {
		if v, ok := byName[k]; ok {
			t.Errorf("bare %v shared with a place in no metro resolved to %q", k, v)
		}
	}
	if v, ok := exact[metroKey{"dover", "us", "NJ"}]; ok {
		t.Errorf("exact key with and without a metro resolved to %q", v)
	}
	if got := exact[metroKey{"jackson", "us", "NJ"}]; got != "Philadelphia (US)" {
		t.Errorf("exact jackson NJ = %q", got)
	}
	if v, ok := exact[metroKey{"jackson", "us", "MS"}]; !ok || v != "" {
		t.Errorf("exact jackson MS = %q, %v; want the no-metro value kept", v, ok)
	}
	if v, ok := byName[[2]string{"tyler", "us"}]; !ok || v != "" {
		t.Errorf("bare tyler = %q, %v; want the no-metro value kept", v, ok)
	}
}

// A stated subdivision that places the city in no metro is an answer: it disagrees with
// one placing it in a metro, and on its own it yields nothing.
func TestMetroForNoMetroTakesPartInAgreement(t *testing.T) {
	both := map[string]struct{}{"MS": {}, "NJ": {}}
	if v, ok := metroFor("Jackson", "us", both); ok {
		t.Errorf("Jackson under MS and NJ = %q, want none", v)
	}
	if v, ok := metroFor("Jackson", "us", map[string]struct{}{"MS": {}}); ok {
		t.Errorf("Jackson under MS = %q, want none", v)
	}
}

func TestMetrosForCities(t *testing.T) {
	got := MetrosForCities([]string{"Folsom", "Elk Grove", "Jackson", "Unknownville"}, "us")
	if !slices.Equal(got, []string{"Sacramento (US)"}) {
		t.Errorf("MetrosForCities = %v", got)
	}
	if got := MetrosForCities([]string{"Khimki"}, "us"); got != nil {
		t.Errorf("Khimki under us = %v, want none", got)
	}
}

func TestLoadMetrosDuplicateExactKeyIsDropped(t *testing.T) {
	tsv := "brentwood\tus\tCA\tSan Francisco (US)\n" +
		"brentwood\tus\tCA\tSacramento (US)\n" +
		"akbarpur\tin\t\tDelhi (IN)\n" +
		"akbarpur\tin\t\tLucknow (IN)\n" +
		"folsom\tus\tCA\tSacramento (US)\n" +
		"folsom\tus\tCA\tSacramento (US)\n"
	exact, byName := loadMetros(tsv)
	for _, k := range []metroKey{{"brentwood", "us", "CA"}, {"akbarpur", "in", ""}} {
		if v, ok := exact[k]; ok {
			t.Errorf("duplicate exact key %v resolved to %q", k, v)
		}
	}
	for _, k := range [][2]string{{"brentwood", "us"}, {"akbarpur", "in"}} {
		if _, ok := byName[k]; ok {
			t.Errorf("duplicate bare key %v resolved", k)
		}
	}
	if got := exact[metroKey{"folsom", "us", "CA"}]; got != "Sacramento (US)" {
		t.Errorf("repeated identical line must still resolve, got %q", got)
	}
}

// Every metro value must survive a comma-joined filter and carry its country suffix.
// An empty value is allowed: it records a place in no metro.
func TestMetroValuesCarryNoComma(t *testing.T) {
	re := regexp.MustCompile(`^[^,]+ \([A-Z]{2}\)$`)
	for _, line := range strings.Split(metrosTSV, "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 4 {
			t.Errorf("line %q has %d columns, want 4", line, len(f))
			continue
		}
		if v := f[3]; v != "" && !re.MatchString(v) {
			t.Errorf("metro value %q does not match Name (CC)", v)
		}
	}
}
