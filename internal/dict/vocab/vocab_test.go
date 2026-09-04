package vocab

import (
	"slices"
	"strings"
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

// TestEngineeringDesignIsNonTech pins the split of the design craft: engineering
// draughting (mechanical, electrical, civil, architectural) is its own category and
// counts as non-technical, so it is surfaced as a facet while staying off the LLM
// enrichment and embedding budgets. `design` keeps meaning product/visual design only,
// and silicon design stays in `hardware`.
func TestEngineeringDesignIsNonTech(t *testing.T) {
	if !slices.Contains(CategoryValues, "engineering_design") {
		t.Fatal("CategoryValues must contain engineering_design")
	}
	if !slices.Contains(NonTechCategories, "engineering_design") {
		t.Error("engineering_design must be a NonTechCategories member")
	}
	if !slices.Contains(TechCategories, "design") {
		t.Error("design (product/visual design) must stay a technical category")
	}
}

// TestCreativeIsTech pins the placement of the media-production category. It is
// technical, so its postings are enriched and embedded like design and product,
// and — because cmd/prune's business rule deletes from NonTechCategories — a
// posting can never become removable by resolving to it.
func TestCreativeIsTech(t *testing.T) {
	if !slices.Contains(CategoryValues, "creative") {
		t.Fatal("CategoryValues must contain creative")
	}
	if !slices.Contains(TechCategories, "creative") {
		t.Error("creative must be a TechCategories member")
	}
	if slices.Contains(NonTechCategories, "creative") {
		t.Error("creative must not be a NonTechCategories member — that set is what prune deletes from")
	}
}

// TestCraftCategoriesAreNonTech pins the set cmd/prune's business rule subtracts.
// Those categories are non-technical because the CRAFT sits outside IT, not because
// the posting is back-office work at a software employer — deleting them would take
// out an engineering employer's whole catalogue the moment its board is retired.
// A member that is not non-technical would mean the subtraction is pointless; a craft
// category left out of the set becomes deletable in silence.
func TestCraftCategoriesAreNonTech(t *testing.T) {
	if len(NonTechCraftCategories) == 0 {
		t.Fatal("NonTechCraftCategories must not be empty")
	}
	for _, c := range NonTechCraftCategories {
		if !slices.Contains(NonTechCategories, c) {
			t.Errorf("craft category %q is not a NonTechCategories member, so subtracting it is meaningless", c)
		}
		if !slices.Contains(CategoryValues, c) {
			t.Errorf("craft category %q is not a member of CategoryValues", c)
		}
	}
	for _, want := range []string{"engineering_design", "industrial_engineering"} {
		if !slices.Contains(NonTechCraftCategories, want) {
			t.Errorf("NonTechCraftCategories must contain %q", want)
		}
	}
	// The consumer industries are deliberately NOT craft-protected. They are exactly
	// the non-technical business prune's rule exists to remove at a company with no
	// technical history, and leaving them out is also what keeps this vocabulary
	// growth behaviour-neutral: the same postings match ruleUnknown before and
	// ruleBusiness after. Adding one here would make a quarter of a million postings
	// newly undeletable — a policy change wearing a vocabulary change's clothes.
	for _, notCraft := range []string{
		"healthcare", "skilled_trades", "retail", "hospitality",
		"logistics", "education", "personal_services", "administration",
	} {
		if slices.Contains(NonTechCraftCategories, notCraft) {
			t.Errorf("%q must NOT be craft-protected: it is the business prune exists to remove", notCraft)
		}
	}
}

func TestConsumerCategoriesAreNonTech(t *testing.T) {
	for _, c := range []string{
		"healthcare", "skilled_trades", "retail", "hospitality",
		"logistics", "education", "personal_services", "administration",
	} {
		if !slices.Contains(CategoryValues, c) {
			t.Errorf("CategoryValues must contain %q", c)
			continue
		}
		if !slices.Contains(NonTechCategories, c) {
			t.Errorf("%q must be a NonTechCategories member", c)
		}
		if slices.Contains(TechCategories, c) {
			t.Errorf("%q must not be a TechCategories member", c)
		}
	}
}

func TestIndustrialEngineeringIsNonTech(t *testing.T) {
	if !slices.Contains(CategoryValues, "industrial_engineering") {
		t.Fatal("CategoryValues must contain industrial_engineering")
	}
	if !slices.Contains(NonTechCategories, "industrial_engineering") {
		t.Error("industrial_engineering must be a NonTechCategories member")
	}
	if slices.Contains(TechCategories, "industrial_engineering") {
		t.Error("industrial_engineering must not be a TechCategories member")
	}
}

func TestDomainGlossCoversVocabulary(t *testing.T) {
	for _, d := range DomainValues {
		if strings.TrimSpace(DomainGloss[d]) == "" {
			t.Errorf("domain %q has no gloss for the enrichment prompt", d)
		}
	}
	for d := range DomainGloss {
		if !slices.Contains(DomainValues, d) {
			t.Errorf("DomainGloss has %q not in DomainValues", d)
		}
	}
}

func TestCompanyTypeGlossCoversVocabulary(t *testing.T) {
	for _, ct := range CompanyTypeValues {
		if strings.TrimSpace(CompanyTypeGloss[ct]) == "" {
			t.Errorf("company_type %q has no gloss for the enrichment prompt", ct)
		}
	}
	for ct := range CompanyTypeGloss {
		if !slices.Contains(CompanyTypeValues, ct) {
			t.Errorf("CompanyTypeGloss has %q not in CompanyTypeValues", ct)
		}
	}
}
