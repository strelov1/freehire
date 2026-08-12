package cvmatch

import (
	"strings"
	"testing"
)

func TestTitleCategoryExactTitleEarnsBothItems(t *testing.T) {
	c := titleCategory("Data Engineer", "JANE DOE\nSenior Data Engineer at Acme\nBuilt pipelines.")

	if !c.Available {
		t.Fatalf("category is unavailable: %q", c.Reason)
	}
	if c.Earned != c.Weight {
		t.Errorf("earned = %d of weight %d, want full marks", c.Earned, c.Weight)
	}
	if c.Weight != WeightTitle {
		t.Errorf("weight = %d, want %d", c.Weight, WeightTitle)
	}
}

// The title is matched case- and spacing-insensitively: a CV that sets its heading in caps
// is the same claim as one that does not.
func TestTitleCategoryMatchIgnoresCaseAndSpacing(t *testing.T) {
	c := titleCategory("Data  Engineer", "jane doe — DATA ENGINEER\n")

	if c.Earned != c.Weight {
		t.Errorf("earned = %d of weight %d, want full marks", c.Earned, c.Weight)
	}
}

// No exact title, but the CV spans the vacancy's field: partial marks, and the item says
// which field it matched rather than leaving the candidate to guess.
func TestTitleCategorySameCategoryEarnsPartialMarks(t *testing.T) {
	c := titleCategory("Data Engineer", "Analytics specialist who built data engineering pipelines in Spark.")

	if !c.Available {
		t.Fatalf("category is unavailable: %q", c.Reason)
	}
	if c.Earned == 0 || c.Earned == c.Weight {
		t.Errorf("earned = %d of weight %d, want partial marks", c.Earned, c.Weight)
	}
	var named bool
	for _, it := range c.Items {
		if it.Status == StatusPass && strings.Contains(strings.ToLower(it.Text), "data engineering") {
			named = true
		}
	}
	if !named {
		t.Errorf("no passing item names the matched role category; got %+v", c.Items)
	}
}

// The title must match on word boundaries: "Data Engineer" occurs inside the phrase "data
// engineering", and a bare substring test hands the full exact-title award to any CV that
// mentions the field in passing.
func TestTitleCategoryDoesNotMatchInsideALongerWord(t *testing.T) {
	c := titleCategory("Data Engineer", "Analytics specialist who built data engineering pipelines.")

	for _, it := range c.Items {
		if it.Status == StatusPass && strings.Contains(it.Text, "states the title") {
			t.Errorf("a mention of the field was credited as the title: %q", it.Text)
		}
	}
}

func TestTitleCategoryNoMatchScoresZeroButStaysAvailable(t *testing.T) {
	c := titleCategory("Data Engineer", "Pastry chef with fifteen years in Michelin kitchens.")

	if !c.Available {
		t.Fatalf("a checkable category became unavailable: %q", c.Reason)
	}
	if c.Earned != 0 {
		t.Errorf("earned = %d, want 0", c.Earned)
	}
}

func TestTitleCategoryUnavailableWithoutAVacancyTitle(t *testing.T) {
	c := titleCategory("   ", "Data Engineer")

	if c.Available {
		t.Fatal("a vacancy with no title must make the category unavailable")
	}
	if c.Reason == "" {
		t.Error("an unavailable category must carry a reason")
	}
}

// The recursive half of the unverifiable rule: a check the dictionaries cannot evaluate
// leaves the CATEGORY's own denominator. "Product Engineer" resolves no role category —
// prod titles split ~2:1 software vs manufacturing for that exact phrase, so classify
// deliberately emits nothing rather than guess — so the category scores out of the
// exact-title check alone, not out of 20 with 8 points the candidate could never have
// earned.
func TestTitleCategoryUnresolvedRoleCategoryShrinksTheDenominator(t *testing.T) {
	c := titleCategory("Product Engineer", "Product Engineer at Acme, five years in Go.")

	if !c.Available {
		t.Fatalf("category is unavailable: %q", c.Reason)
	}
	if c.Weight >= WeightTitle {
		t.Errorf("weight = %d, want less than the full %d — the unresolvable check must leave the denominator", c.Weight, WeightTitle)
	}
	if c.Earned != c.Weight {
		t.Errorf("earned = %d of weight %d: an exact title match must earn everything still on the table", c.Earned, c.Weight)
	}
}

// "Staff Software Engineer" USED to be the unresolvable example above: the dictionary had
// no value for a software generalist. It now resolves to the software_engineering
// catch-all category, so the role-field check becomes evaluable too — the full weight is
// on the table, and a CV that never claims software work at all loses real points instead
// of the vacancy being scored on title text alone.
func TestTitleCategoryGenericSoftwareEngineerNowResolvesTheFullWeight(t *testing.T) {
	c := titleCategory("Staff Software Engineer", "Staff Software Engineer at Acme, five years in Go.")

	if !c.Available {
		t.Fatalf("category is unavailable: %q", c.Reason)
	}
	if c.Weight != WeightTitle {
		t.Errorf("weight = %d, want the full %d — software_engineering now resolves, so the role-field check is evaluable", c.Weight, WeightTitle)
	}
	if c.Earned != c.Weight {
		t.Errorf("earned = %d of weight %d: exact title plus a CV that states software work should earn everything", c.Earned, c.Weight)
	}
}

// Every category reports a weight, available or not: the panel and the wire contract treat
// it as always present, and a category reporting 0 reads as one worth nothing rather than
// one that could not be scored.
func TestTitleCategoryReportsItsNominalWeightWhenUnavailable(t *testing.T) {
	c := titleCategory("", "Data Engineer at Acme")

	if c.Available {
		t.Fatal("expected an unavailable category")
	}
	if c.Weight != WeightTitle {
		t.Errorf("weight = %d, want the nominal %d", c.Weight, WeightTitle)
	}
}
