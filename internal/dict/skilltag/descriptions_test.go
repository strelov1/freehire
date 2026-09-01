package skilltag

import (
	"slices"
	"strings"
	"testing"
)

func TestDescriptionResolvesACuratedSkill(t *testing.T) {
	got := Description("dbt")
	if got == "" {
		t.Fatal(`Description("dbt") is empty`)
	}
	if !strings.Contains(got, "SQL") {
		t.Errorf(`Description("dbt") = %q, want it to name SQL`, got)
	}
}

// A skill no wave has reached yet, and a slug that is not a skill at all, answer the
// same way: nothing. Every surface renders a described skill differently from an
// undescribed one, so the distinction has to be an empty string rather than a
// placeholder that could reach a reader.
func TestDescriptionIsEmptyForAnUndescribedSlug(t *testing.T) {
	for _, slug := range []string{"some-new-thing", ""} {
		if got := Description(slug); got != "" {
			t.Errorf("Description(%q) = %q, want empty", slug, got)
		}
	}
}

// The mirror of TestDisplayNamesHaveNoOrphans: a description for a canonical the
// dictionary no longer emits is dead weight that still reads as curated, and nothing
// else would ever notice it.
func TestDescriptionsHaveNoOrphans(t *testing.T) {
	canonicals := Canonicals()
	for slug := range Descriptions() {
		if !slices.Contains(canonicals, slug) {
			t.Errorf("descriptions.tsv has %q, which is not a canonical skill", slug)
		}
	}
}

// The file is hand-written and reviewed, not scraped, so a malformed row is a mistake
// in this repository rather than noise in someone else's dataset. It fails the build
// instead of being skipped the way internal/dict/location tolerates GeoNames rows.
func TestLoadDescriptionsRejectsMalformedRows(t *testing.T) {
	tests := []struct {
		name string
		tsv  string
	}{
		{"no tab", "dbt a SQL transformation tool\n"},
		{"blank description", "dbt\t\n"},
		{"blank slug", "\tA SQL transformation tool.\n"},
		{"duplicate slug", "dbt\tOne.\ndbt\tTwo.\n"},
		{"untrimmed description", "dbt\t A SQL transformation tool. \n"},
		{"uppercase slug", "DBT\tA SQL transformation tool.\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := loadDescriptions(tc.tsv); err == nil {
				t.Errorf("loadDescriptions(%q) = nil error, want one", tc.tsv)
			}
		})
	}
}

func TestLoadDescriptionsParsesRowsAndSkipsComments(t *testing.T) {
	got, err := loadDescriptions("# a comment\n\ndbt\tA SQL transformation tool.\ngo\tA compiled language.\n")
	if err != nil {
		t.Fatalf("loadDescriptions: %v", err)
	}
	want := map[string]string{"dbt": "A SQL transformation tool.", "go": "A compiled language."}
	if len(got) != len(want) {
		t.Fatalf("loadDescriptions gave %d entries, want %d", len(got), len(want))
	}
	for slug, desc := range want {
		if got[slug] != desc {
			t.Errorf("loadDescriptions[%q] = %q, want %q", slug, got[slug], desc)
		}
	}
}

// The ratchet. Descriptions land in waves ordered by how often a skill appears in the
// catalogue, so this cannot be "every canonical has one" until the last wave — but it
// can refuse to go backwards, which is the failure that would otherwise be silent.
func TestDescriptionCoverageDoesNotRegress(t *testing.T) {
	described, total := len(Descriptions()), len(Canonicals())
	if described < describedFloor {
		t.Errorf("%d canonicals are described, below the recorded floor of %d — raise the "+
			"coverage or restore what was removed, but do not lower the floor",
			described, describedFloor)
	}
	t.Logf("described %d of %d canonicals (floor %d)", described, total, describedFloor)
}

// Descriptions returns a copy: a caller that mutates the map it is handed must not be
// able to edit the shipped dictionary for every other caller in the process.
func TestDescriptionsReturnsACopy(t *testing.T) {
	Descriptions()["dbt"] = "mutated"
	if Description("dbt") == "mutated" {
		t.Error("Descriptions() handed out the live map")
	}
}
