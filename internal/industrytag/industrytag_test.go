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
