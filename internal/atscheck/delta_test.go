package atscheck

import "testing"

// report builds a Report from (id, label, score) triples, with Overall as their sum —
// the shape Score produces, without running the scorer.
func report(cats ...ScoreCategory) Report {
	overall := 0
	for _, c := range cats {
		overall += c.Score
	}
	return Report{Overall: overall, Categories: cats}
}

func cat(id, label string, score int) ScoreCategory {
	return ScoreCategory{ID: id, Label: label, Score: score, Max: 30}
}

func changeByID(t *testing.T, d Delta, id string) CategoryChange {
	t.Helper()
	for _, c := range d.Categories {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("no category %q in %+v", id, d.Categories)
	return CategoryChange{}
}

func TestCompareCategoryChangeIsTailoredMinusBase(t *testing.T) {
	base := report(cat("keyword_strength", "Keyword Strength", 10), cat("format_compliance", "Format Compliance", 20))
	tailored := report(cat("keyword_strength", "Keyword Strength", 18), cat("format_compliance", "Format Compliance", 15))

	d := Compare(base, tailored)

	kw := changeByID(t, d, "keyword_strength")
	if kw.Base != 10 || kw.Tailored != 18 || kw.Change != 8 {
		t.Errorf("keyword = {base %d tailored %d change %d}, want {10 18 8}", kw.Base, kw.Tailored, kw.Change)
	}
	format := changeByID(t, d, "format_compliance")
	if format.Change != -5 {
		t.Errorf("format change = %d, want -5", format.Change)
	}
	if d.Base != 30 || d.Tailored != 33 || d.Change != 3 {
		t.Errorf("overall = {base %d tailored %d change %d}, want {30 33 3}", d.Base, d.Tailored, d.Change)
	}
}

func TestCompareCarriesCategoryLabels(t *testing.T) {
	base := report(cat("content_quality", "Content Quality", 5))
	tailored := report(cat("content_quality", "Content Quality", 5))

	if got := changeByID(t, Compare(base, tailored), "content_quality").Label; got != "Content Quality" {
		t.Errorf("label = %q, want %q", got, "Content Quality")
	}
}

func TestCompareIdenticalReportsAreZeroAndNotRegressed(t *testing.T) {
	r := report(cat("keyword_strength", "Keyword Strength", 12), cat("length_density", "Length & Density", 8))

	d := Compare(r, r)

	if d.Change != 0 {
		t.Errorf("overall change = %d, want 0", d.Change)
	}
	if len(d.Categories) != 2 {
		t.Fatalf("categories = %d (%+v), want both reported even when nothing moved", len(d.Categories), d.Categories)
	}
	for _, c := range d.Categories {
		if c.Change != 0 {
			t.Errorf("category %s change = %d, want 0", c.ID, c.Change)
		}
	}
	if d.Regressed {
		t.Error("Regressed = true, want false for an unchanged CV")
	}
	if d.WorstCategory != "" {
		t.Errorf("WorstCategory = %q, want empty when nothing regressed", d.WorstCategory)
	}
}

func TestCompareLowerOverallIsRegressionNamingItsWorstCategory(t *testing.T) {
	base := report(cat("keyword_strength", "Keyword Strength", 20), cat("format_compliance", "Format Compliance", 20))
	tailored := report(cat("keyword_strength", "Keyword Strength", 18), cat("format_compliance", "Format Compliance", 5))

	d := Compare(base, tailored)

	if !d.Regressed {
		t.Error("Regressed = false, want true when the tailored overall is lower")
	}
	if d.WorstCategory != "format_compliance" {
		t.Errorf("WorstCategory = %q, want format_compliance (fell 15 vs 2)", d.WorstCategory)
	}
}

func TestCompareEqualOverallIsNotRegression(t *testing.T) {
	base := report(cat("keyword_strength", "Keyword Strength", 20), cat("format_compliance", "Format Compliance", 10))
	tailored := report(cat("keyword_strength", "Keyword Strength", 25), cat("format_compliance", "Format Compliance", 5))

	d := Compare(base, tailored)

	if d.Regressed {
		t.Error("Regressed = true, want false when the overall held despite a category falling")
	}
	if d.WorstCategory != "" {
		t.Errorf("WorstCategory = %q, want empty when the overall did not fall", d.WorstCategory)
	}
}

func TestCompareWorstCategoryTieTakesTheEarlierReportedCategory(t *testing.T) {
	base := report(cat("keyword_strength", "Keyword Strength", 20), cat("format_compliance", "Format Compliance", 20))
	tailored := report(cat("keyword_strength", "Keyword Strength", 17), cat("format_compliance", "Format Compliance", 17))

	d := Compare(base, tailored)

	if d.WorstCategory != "keyword_strength" {
		t.Errorf("WorstCategory = %q, want keyword_strength — the earlier category wins an equal drop", d.WorstCategory)
	}
}

func TestCompareReportsOnlyCategoriesPresentOnBothSides(t *testing.T) {
	base := report(cat("keyword_strength", "Keyword Strength", 10))
	tailored := report(cat("keyword_strength", "Keyword Strength", 12), cat("content_quality", "Content Quality", 9))

	d := Compare(base, tailored)

	if len(d.Categories) != 1 {
		t.Fatalf("categories = %d (%+v), want 1 — a category on one side only is not a difference", len(d.Categories), d.Categories)
	}
	if d.Categories[0].ID != "keyword_strength" {
		t.Errorf("category = %q, want keyword_strength", d.Categories[0].ID)
	}
}

func TestCompareKeepsTheTailoredReportsCategoryOrder(t *testing.T) {
	base := report(cat("length_density", "Length & Density", 5), cat("keyword_strength", "Keyword Strength", 5))
	tailored := report(cat("keyword_strength", "Keyword Strength", 6), cat("length_density", "Length & Density", 6))

	d := Compare(base, tailored)

	if len(d.Categories) != 2 || d.Categories[0].ID != "keyword_strength" || d.Categories[1].ID != "length_density" {
		t.Errorf("order = %+v, want the tailored report's order (keyword_strength, length_density)", d.Categories)
	}
}

// A row that reports only a number tells the candidate what changed and never why. The
// checks behind the score already exist; the delta carries them so the panel can expand a
// category into them.
func TestCompareCarriesTheTailoredSidesLineItems(t *testing.T) {
	base := report(cat("keyword_strength", "Keyword Strength", 10))
	tailored := report(cat("keyword_strength", "Keyword Strength", 12))
	base.Categories[0].Items = []LineItem{{Points: 10, Text: "before", Status: StatusWarn}}
	tailored.Categories[0].Items = []LineItem{{Points: 12, Text: "after", Status: StatusPass}}

	d := Compare(base, tailored)

	if len(d.Categories) != 1 {
		t.Fatalf("categories = %d, want 1", len(d.Categories))
	}
	got := d.Categories[0].Items
	if len(got) != 1 {
		t.Fatalf("items = %+v, want the tailored side's one item", got)
	}
	// The candidate is editing the tailored copy; a before/after list of individual checks
	// is a diff nobody asked for.
	if got[0].Text != "after" {
		t.Errorf("item = %q, want the tailored side's item, not the base's", got[0].Text)
	}
}
