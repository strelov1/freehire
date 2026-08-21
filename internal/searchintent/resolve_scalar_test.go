package searchintent

import "testing"

func resolveIntent(t *testing.T, in intent) Result {
	t.Helper()
	got, err := in.resolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	return got
}

func TestResolveKeepsSalaryFloor(t *testing.T) {
	got := resolveIntent(t, intent{SalaryMin: ptr(120000)})
	if got.Scalars.SalaryMin == nil || *got.Scalars.SalaryMin != 120000 {
		t.Fatalf("salary floor = %v, want 120000", got.Scalars.SalaryMin)
	}
}

func TestResolveDropsNonPositiveSalaryFloor(t *testing.T) {
	got := resolveIntent(t, intent{SalaryMin: ptr(0)})
	if got.Scalars.SalaryMin != nil {
		t.Fatalf("salary floor = %v, want none — a floor of zero filters nothing", *got.Scalars.SalaryMin)
	}
}

func TestResolveDropsAbsurdSalaryFloor(t *testing.T) {
	got := resolveIntent(t, intent{SalaryMin: ptr(999999999)})
	if got.Scalars.SalaryMin != nil {
		t.Fatalf("salary floor = %v, want none", *got.Scalars.SalaryMin)
	}
	if len(got.Unresolved) == 0 {
		t.Fatal("unresolved is empty — a dropped bound must be reported like any other drop")
	}
}

func TestResolveKeepsFreshnessPreset(t *testing.T) {
	got := resolveIntent(t, intent{PostedWithinDays: ptr(7)})
	if got.Scalars.PostedWithinDays == nil || *got.Scalars.PostedWithinDays != 7 {
		t.Fatalf("freshness = %v, want 7", got.Scalars.PostedWithinDays)
	}
}

// "Posted in the last five days" is a real request even though the slider has no such
// stop, and the sidebar names an off-preset bound honestly. Snapping it to a preset
// would answer a question the caller did not ask.
func TestResolveKeepsOffPresetFreshness(t *testing.T) {
	got := resolveIntent(t, intent{PostedWithinDays: ptr(5)})
	if got.Scalars.PostedWithinDays == nil || *got.Scalars.PostedWithinDays != 5 {
		t.Fatalf("freshness = %v, want 5 exactly", got.Scalars.PostedWithinDays)
	}
}

func TestResolveDropsAbsurdFreshness(t *testing.T) {
	got := resolveIntent(t, intent{PostedWithinDays: ptr(100000)})
	if got.Scalars.PostedWithinDays != nil {
		t.Fatalf("freshness = %v, want none", *got.Scalars.PostedWithinDays)
	}
	if len(got.Unresolved) == 0 {
		t.Fatal("unresolved is empty — a dropped bound must be reported")
	}
}

// Zero is the entry-level filter — postings that ask for no prior experience — so it
// is a bound like any other, not an unset one.
func TestResolveKeepsZeroExperienceCeiling(t *testing.T) {
	got := resolveIntent(t, intent{ExperienceYearsMax: ptr(0)})
	if got.Scalars.ExperienceYearsMax == nil || *got.Scalars.ExperienceYearsMax != 0 {
		t.Fatalf("experience ceiling = %v, want 0", got.Scalars.ExperienceYearsMax)
	}
}

func TestResolveDropsAbsurdExperienceCeiling(t *testing.T) {
	got := resolveIntent(t, intent{ExperienceYearsMax: ptr(400)})
	if got.Scalars.ExperienceYearsMax != nil {
		t.Fatalf("experience ceiling = %v, want none", *got.Scalars.ExperienceYearsMax)
	}
}

func TestResolveKeepsVisaFlag(t *testing.T) {
	got := resolveIntent(t, intent{VisaSponsorship: true})
	if !got.Scalars.VisaSponsorship {
		t.Fatal("visa flag lost")
	}
}

func TestResolveKeepsFreeTextQuery(t *testing.T) {
	got := resolveIntent(t, intent{Query: "  climate modelling  "})
	if got.Query != "climate modelling" {
		t.Fatalf("query = %q, want %q", got.Query, "climate modelling")
	}
}

func TestResolveCarriesSummary(t *testing.T) {
	got := resolveIntent(t, intent{Summary: "Senior Go backend roles in Portugal."})
	if got.Summary != "Senior Go backend roles in Portugal." {
		t.Fatalf("summary = %q, want it carried through", got.Summary)
	}
}

// An interpretation that resolved nothing must be distinguishable from one that
// resolved something, so the dialog can say so rather than applying an empty filter
// that reads as "the whole catalogue matches you".
func TestResolveReportsWhenNothingResolved(t *testing.T) {
	got := resolveIntent(t, intent{Facets: map[string][]string{"skills": {"blockchain-adjacent"}}})
	if !got.Empty() {
		t.Fatal("Empty() = false, want true — nothing resolved")
	}
	if resolveIntent(t, intent{Facets: map[string][]string{"skills": {"Golang"}}}).Empty() {
		t.Fatal("Empty() = true, want false — a skill resolved")
	}
	if resolveIntent(t, intent{Query: "climate"}).Empty() {
		t.Fatal("Empty() = true, want false — a free-text query alone is still a search")
	}
	if resolveIntent(t, intent{PostedWithinDays: ptr(7)}).Empty() {
		t.Fatal("Empty() = true, want false — a freshness bound alone is still a search")
	}
}

func ptr[T any](v T) *T { return &v }
