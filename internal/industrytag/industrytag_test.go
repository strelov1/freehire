package industrytag

import (
	"reflect"
	"testing"
)

func TestCanonicalize(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "separator and case variants of one industry collapse",
			in:   []string{"Financial-Services", "Financial Services", "financial services"},
			want: []string{"financial-services"},
		},
		{
			name: "ampersand and the word and are the same separator",
			in:   []string{"Food & Beverage", "Food-and-Beverage"},
			want: []string{"food-and-beverage"},
		},
		{
			name: "curated synonyms collapse to one canonical",
			in:   []string{"AI", "Artificial Intelligence"},
			want: []string{"ai"},
		},
		{
			name: "an unknown label emits nothing",
			in:   []string{"CTRM-(Commodity-Trading-and-Risk-Management)"},
			want: []string{},
		},
		{
			name: "an already-canonical value resolves to itself",
			in:   []string{"medical-devices"},
			want: []string{"medical-devices"},
		},
		{
			name: "output is sorted and de-duplicated",
			in:   []string{"Retail", "AI", "retail"},
			want: []string{"ai", "retail"},
		},
		{
			name: "an ampersand with no surrounding spaces is still a separator",
			in:   []string{"Food&Beverage"},
			want: []string{"food-and-beverage"},
		},
		{
			name: "blank input yields an empty result",
			in:   []string{"", "   "},
			want: []string{},
		},
		{
			name: "no input yields an empty result",
			in:   nil,
			want: []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Canonicalize(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Canonicalize(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// A mistyped alias target must not become a stored value. Such a value would be
// absent from Canonicals() — so the facet could never offer it — would render as a
// raw slug, and would be silently deleted by the next normalization pass, which is
// exactly the "no canonical value is invented" rule the dictionary exists to keep.
func TestCanonicalizeRejectsAliasPointingAtUnknownCanonical(t *testing.T) {
	const bogus = "not-a-real-canonical"
	aliases["probe-broken-alias"] = bogus
	t.Cleanup(func() { delete(aliases, "probe-broken-alias") })

	if got := Canonicalize([]string{"probe broken alias"}); len(got) != 0 {
		t.Errorf("Canonicalize resolved an alias whose target is not canonical: got %q, want none", got)
	}
}

// Every canonical value must survive a round trip, or the dictionary holds entries
// the resolver can never produce — dead options in the facet list.
func TestEveryCanonicalRoundTrips(t *testing.T) {
	canonicals := Canonicals()
	if got := Canonicalize(canonicals); !reflect.DeepEqual(got, canonicals) {
		t.Errorf("Canonicalize(Canonicals()) = %q, want %q", got, canonicals)
	}
}
