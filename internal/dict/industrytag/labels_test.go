package industrytag

import (
	"reflect"
	"slices"
	"testing"
)

func TestLabelReturnsCuratedDisplayText(t *testing.T) {
	if got, want := Label("food-and-beverage"), "Food & Beverage"; got != want {
		t.Errorf("Label(%q) = %q, want %q", "food-and-beverage", got, want)
	}
}

// A value stored before a dictionary edit must still render as something, rather
// than disappearing from the UI.
func TestLabelFallsBackToTheSlug(t *testing.T) {
	if got, want := Label("no-longer-in-the-dictionary"), "no-longer-in-the-dictionary"; got != want {
		t.Errorf("Label(%q) = %q, want the slug back", "no-longer-in-the-dictionary", got)
	}
}

func TestCanonicalsIsTheSortedCanonicalSet(t *testing.T) {
	got := Canonicals()

	if !slices.IsSorted(got) {
		t.Errorf("Canonicals() is not sorted: %q", got)
	}

	want := make([]string, 0, len(displayNames))
	for canonical := range displayNames {
		want = append(want, canonical)
	}
	slices.Sort(want)

	if !reflect.DeepEqual(got, want) {
		t.Errorf("Canonicals() = %q, want every key of displayNames: %q", got, want)
	}
}

func TestSearchAliasesMapsNormalizedAliasesToCanonicalSlugs(t *testing.T) {
	got := SearchAliases()
	if !slices.Contains(got["financial-services"], "finance") {
		t.Fatalf("SearchAliases()[financial-services] = %q, want finance", got["financial-services"])
	}
	got["financial-services"][0] = "mutated"
	if slices.Contains(SearchAliases()["financial-services"], "mutated") {
		t.Fatal("SearchAliases returned a slice backed by the package dictionary")
	}
}

func TestCanonicalizeNormalizationVectors(t *testing.T) {
	tests := []struct {
		name  string
		label string
		want  []string
	}{
		{name: "ampersand", label: "Food&Beverage", want: []string{"food-and-beverage"}},
		{name: "whitespace and punctuation", label: "\u00a0 Food	&  Beverage \u00a0", want: []string{"food-and-beverage"}},
		{name: "Unicode simple lowercase", label: "İT Services", want: []string{"it-services"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Canonicalize([]string{tt.label}); !slices.Equal(got, tt.want) {
				t.Errorf("Canonicalize(%q) = %q, want %q", tt.label, got, tt.want)
			}
		})
	}
}
