package cvmatch

import (
	"slices"
	"strings"
	"testing"
)

// The candidate is told what to add, not only how far short they fell.
func TestKeywordCategoryNamesTheMissingSkills(t *testing.T) {
	c, matched, missing := keywordCategory(
		[]string{"go", "postgresql"},
		[]string{"go", "kubernetes", "postgresql", "terraform"},
	)

	if !c.Available {
		t.Fatalf("category is unavailable: %q", c.Reason)
	}
	if !slices.Equal(missing, []string{"kubernetes", "terraform"}) {
		t.Errorf("missing = %v, want [kubernetes terraform]", missing)
	}
	if !slices.Equal(matched, []string{"go", "postgresql"}) {
		t.Errorf("matched = %v, want [go postgresql]", matched)
	}
	// Half of four skills present, at weight 30.
	if c.Earned != 15 {
		t.Errorf("earned = %d, want 15", c.Earned)
	}
	if c.Weight != WeightKeyword {
		t.Errorf("weight = %d, want %d", c.Weight, WeightKeyword)
	}
}

// A vacancy whose canonical skills we never resolved is a gap in our dictionaries, not in
// the CV: awarding either full or zero marks would be an opinion we have no basis for.
func TestKeywordCategoryUnavailableWithoutVacancySkills(t *testing.T) {
	c, matched, missing := keywordCategory([]string{"go"}, nil)

	if c.Available {
		t.Fatal("a vacancy with no canonical skills must make the category unavailable")
	}
	if c.Reason == "" {
		t.Error("an unavailable category must carry a reason")
	}
	if c.Earned != 0 || len(c.Items) != 0 {
		t.Errorf("an unavailable category must score nothing and carry no items; got %+v", c)
	}
	if len(matched) != 0 || len(missing) != 0 {
		t.Errorf("matched/missing = %v/%v, want both empty", matched, missing)
	}
}

// Full coverage earns the full weight and raises nothing to fix — a warning beside a
// perfect score teaches the candidate to ignore warnings.
func TestKeywordCategoryFullCoverageRaisesNoWarning(t *testing.T) {
	c, _, missing := keywordCategory([]string{"go", "docker", "aws"}, []string{"go", "docker"})

	if c.Earned != WeightKeyword {
		t.Errorf("earned = %d, want %d", c.Earned, WeightKeyword)
	}
	if len(missing) != 0 {
		t.Errorf("missing = %v, want empty", missing)
	}
	for _, it := range c.Items {
		if it.Status != StatusPass {
			t.Errorf("full coverage raised a %s item: %q", it.Status, it.Text)
		}
	}
}

// A document that matched nothing still earns a scored category — it is checkable, it just
// failed the check.
func TestKeywordCategoryNoOverlapIsAFailNotAnAbsence(t *testing.T) {
	c, matched, missing := keywordCategory([]string{"cobol"}, []string{"go", "rust"})

	if !c.Available {
		t.Fatalf("a checkable category became unavailable: %q", c.Reason)
	}
	if c.Earned != 0 {
		t.Errorf("earned = %d, want 0", c.Earned)
	}
	if len(matched) != 0 {
		t.Errorf("matched = %v, want empty", matched)
	}
	if !slices.Equal(missing, []string{"go", "rust"}) {
		t.Errorf("missing = %v, want [go rust]", missing)
	}
	if len(c.Items) == 0 {
		t.Fatal("a scored category must attribute its score to at least one item")
	}
	if !strings.Contains(strings.ToLower(c.Items[0].Text), "0 of 2") {
		t.Errorf("the line item does not state the tally: %q", c.Items[0].Text)
	}
}
