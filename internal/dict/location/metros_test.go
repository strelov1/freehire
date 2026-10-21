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
func TestMetroValuesCarryNoComma(t *testing.T) {
	re := regexp.MustCompile(`^[^,]+ \([A-Z]{2}\)$`)
	for _, line := range strings.Split(metrosTSV, "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if v := f[len(f)-1]; !re.MatchString(v) {
			t.Errorf("metro value %q does not match Name (CC)", v)
		}
	}
}
