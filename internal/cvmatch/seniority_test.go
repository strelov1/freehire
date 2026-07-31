package cvmatch

import "testing"

func TestSeniorityCategoryExactGradeEarnsFullMarks(t *testing.T) {
	c := seniorityCategory("Senior Data Engineer", "Senior Data Engineer at Acme, 2019–2025.")

	if !c.Available {
		t.Fatalf("category is unavailable: %q", c.Reason)
	}
	if c.Earned != WeightSeniority || c.Weight != WeightSeniority {
		t.Errorf("earned %d of weight %d, want %d of %d", c.Earned, c.Weight, WeightSeniority, WeightSeniority)
	}
}

// One rung apart is a stretch, not a mismatch — a senior applying to a lead role is the
// ordinary case, and scoring it zero would push them to inflate their own title.
func TestSeniorityCategoryOneRungApartEarnsHalfMarks(t *testing.T) {
	c := seniorityCategory("Lead Data Engineer", "Senior Data Engineer at Acme.")

	if !c.Available {
		t.Fatalf("category is unavailable: %q", c.Reason)
	}
	if c.Earned != WeightSeniority/2 {
		t.Errorf("earned = %d, want %d", c.Earned, WeightSeniority/2)
	}
}

func TestSeniorityCategoryTwoRungsApartEarnsNothing(t *testing.T) {
	c := seniorityCategory("Principal Data Engineer", "Junior Data Engineer at Acme.")

	if !c.Available {
		t.Fatalf("category is unavailable: %q", c.Reason)
	}
	if c.Earned != 0 {
		t.Errorf("earned = %d, want 0", c.Earned)
	}
}

// The dictionaries emit nothing for unknowns here as everywhere else: a title with no
// stated grade is not a junior one.
func TestSeniorityCategoryUnavailableWhenTheVacancyStatesNoGrade(t *testing.T) {
	c := seniorityCategory("Data Engineer", "Senior Data Engineer at Acme.")

	if c.Available {
		t.Fatal("a vacancy title stating no grade must make the category unavailable")
	}
	if c.Reason == "" {
		t.Error("an unavailable category must carry a reason")
	}
	if c.Earned != 0 {
		t.Errorf("earned = %d, want 0", c.Earned)
	}
}

func TestSeniorityCategoryUnavailableWhenTheCVStatesNoGrade(t *testing.T) {
	c := seniorityCategory("Senior Data Engineer", "Data Engineer at Acme, built pipelines.")

	if c.Available {
		t.Fatal("a CV stating no grade must make the category unavailable, not a mismatch")
	}
}

// A CV listing a career's worth of titles is read at the grade it reached: the ladder is
// walked highest-first, so the strongest claim in the document wins.
func TestSeniorityCategoryReadsTheHighestGradeTheCVClaims(t *testing.T) {
	c := seniorityCategory("Staff Data Engineer",
		"Junior Data Engineer 2015–2018\nSenior Data Engineer 2018–2022\nStaff Data Engineer 2022–2025")

	if c.Earned != WeightSeniority {
		t.Errorf("earned = %d, want full marks for the staff grade the CV reached", c.Earned)
	}
}
