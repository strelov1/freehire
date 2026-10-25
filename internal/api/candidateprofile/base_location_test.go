package candidateprofile

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/strelov1/freehire/internal/identity/userprofile"
)

func TestFormatBase(t *testing.T) {
	cases := []struct {
		base userprofile.BaseLocation
		want string
	}{
		{userprofile.BaseLocation{City: "Florianópolis", Country: "br"}, "Florianópolis, Brazil"},
		{userprofile.BaseLocation{Country: "de"}, "Germany"},
		{userprofile.BaseLocation{City: "Lisbon"}, "Lisbon"},
		// An unknown code is left out, not printed raw.
		{userprofile.BaseLocation{City: "Somewhere", Country: "zz-not-a-code"}, "Somewhere"},
		{userprofile.BaseLocation{}, ""},
	}
	for _, c := range cases {
		if got := formatBase(c.base); got != c.want {
			t.Errorf("formatBase(%+v) = %q, want %q", c.base, got, c.want)
		}
	}
}

type fakeProfiles struct{ prefs string }

func (f fakeProfiles) Get(context.Context, int64) (userprofile.Profile, error) {
	return userprofile.Profile{LocationPreferences: json.RawMessage(f.prefs)}, nil
}

// The profile's base is read and rendered; with no reader there is nothing to add.
func TestBaseLocation_RendersTheProfileBase(t *testing.T) {
	a := (&Assembler{}).WithProfileLocation(fakeProfiles{prefs: `{"base":{"city":"Florianópolis","country":"br"}}`})

	got, err := a.baseLocation(context.Background(), 1)
	if err != nil {
		t.Fatalf("baseLocation: %v", err)
	}
	if got != "Florianópolis, Brazil" {
		t.Errorf("baseLocation = %q", got)
	}

	if got, _ := (&Assembler{}).baseLocation(context.Background(), 1); got != "" {
		t.Errorf("no profile reader: baseLocation = %q, want empty", got)
	}
}
