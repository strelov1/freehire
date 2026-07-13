package enrich

import (
	"slices"
	"testing"
)

// TestCategoryPartition locks the invariant the is_tech derivation relies on:
// TechCategories, NonTechCategories, and the residual {"other"} must partition
// CategoryValues exactly — every category classified once, none twice, none left
// out. If a new category is added to CategoryValues without placing it, this fails.
func TestCategoryPartition(t *testing.T) {
	seen := map[string]int{}
	for _, c := range TechCategories {
		seen[c]++
	}
	for _, c := range NonTechCategories {
		seen[c]++
	}
	seen["other"]++

	for _, c := range CategoryValues {
		switch seen[c] {
		case 0:
			t.Errorf("category %q is in CategoryValues but neither Tech nor NonTech nor other", c)
		case 1:
			// classified exactly once — good
		default:
			t.Errorf("category %q is classified %d times (must be exactly one bucket)", c, seen[c])
		}
	}
	for c, n := range seen {
		if !slices.Contains(CategoryValues, c) {
			t.Errorf("category %q is bucketed (%d) but not a member of CategoryValues", c, n)
		}
	}
}

func TestTechCategoriesExcludesNonTech(t *testing.T) {
	if !slices.Contains(TechCategories, "backend") {
		t.Error("TechCategories must contain a recognized technical category like backend")
	}
	for _, nt := range NonTechCategories {
		if slices.Contains(TechCategories, nt) {
			t.Errorf("TechCategories must not contain non-tech category %q", nt)
		}
	}
	if slices.Contains(TechCategories, "other") {
		t.Error("TechCategories must not contain the residual \"other\"")
	}
}
