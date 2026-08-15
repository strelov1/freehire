package skilltag

import (
	"slices"
	"strings"
	"testing"
)

// A display name for a canonical the dictionary no longer emits is dead weight that
// still reads as curated. The catalog is built from Canonicals(), so such an entry would
// never be served — this is the only thing that notices it.
func TestDisplayNamesHaveNoOrphans(t *testing.T) {
	canonicals := Canonicals()
	for slug := range displayNames {
		if !slices.Contains(canonicals, slug) {
			t.Errorf("displayNames has %q, which is not a canonical skill", slug)
		}
	}
}

// An entry that only repeats what title-casing already produces reads as a curated
// decision and is none: it costs a line to maintain and changes nothing. The table is
// for the cases the machine gets wrong.
func TestDisplayNamesHoldsOnlyExceptions(t *testing.T) {
	for slug, name := range displayNames {
		if name == titleCase(slug) {
			t.Errorf("displayNames[%q] = %q is what title-casing gives — drop the entry", slug, name)
		}
	}
}

// Every canonical must render as words. The mechanical fallback makes this hard to fail
// outright, so the check is on the shape a label must have rather than on its presence:
// non-empty, trimmed, and free of the slug's own hyphens-as-separators.
func TestEveryCanonicalHasAReadableLabel(t *testing.T) {
	for _, slug := range Canonicals() {
		label := Label(slug)
		switch {
		case label == "":
			t.Errorf("%q has an empty label", slug)
		case strings.TrimSpace(label) != label:
			t.Errorf("%q label %q has surrounding space", slug, label)
		case strings.Contains(label, "  "):
			t.Errorf("%q label %q has a double space", slug, label)
		}
	}
}

func TestLabelCuratesAndFallsBack(t *testing.T) {
	tests := []struct {
		canonical string
		want      string
	}{
		{"aws", "AWS"},
		{"cpp", "C++"},
		{"dbt", "dbt"},
		{"nodejs", "Node.js"},
		{"postgresql", "PostgreSQL"},
		{"ci-cd", "CI/CD"},
		// No entry of its own — title-cased on the hyphens.
		{"data-engineering", "Data Engineering"},
		{"kubernetes", "Kubernetes"},
		// Not in the vocabulary at all: still words, never an error.
		{"some-new-thing", "Some New Thing"},
	}
	for _, tc := range tests {
		if got := Label(tc.canonical); got != tc.want {
			t.Errorf("Label(%q) = %q, want %q", tc.canonical, got, tc.want)
		}
	}
}

// The catalog is what the SPA renders, so it must cover the vocabulary exactly — a
// missing key would surface a raw slug in the interface.
func TestLabelsCoversEveryCanonical(t *testing.T) {
	canonicals := Canonicals()
	labels := Labels()
	if len(labels) != len(canonicals) {
		t.Fatalf("Labels() has %d entries, Canonicals() has %d", len(labels), len(canonicals))
	}
	for _, slug := range canonicals {
		if labels[slug] == "" {
			t.Errorf("Labels() is missing %q", slug)
		}
	}
}
