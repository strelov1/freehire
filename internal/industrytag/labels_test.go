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
