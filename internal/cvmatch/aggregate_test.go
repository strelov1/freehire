package cvmatch

import (
	"slices"
	"testing"
)

func available(id string, earned, weight int) ScoredCategory {
	return ScoredCategory{ID: id, Earned: earned, Weight: weight, Available: true}
}

func unavailable(id string, weight int) ScoredCategory {
	return ScoredCategory{ID: id, Weight: weight, Available: false, Reason: "not evaluable"}
}

// The overall is earned ÷ possible over the AVAILABLE categories, expressed out of 100.
// One formula covers every degraded state, so there is no branch for "some categories are
// missing" to get wrong.
func TestOverallIsEarnedOverAvailableWeight(t *testing.T) {
	s := aggregate([]ScoredCategory{
		unavailable(CategoryRequirements, WeightRequirements),
		available(CategoryKeyword, 20, WeightKeyword),
		available(CategoryTitle, 20, WeightTitle),
		available(CategorySeniority, 5, WeightSeniority),
	})

	// 45 earned of the 60 weight still on the table.
	if s.Overall != 75 {
		t.Errorf("overall = %d, want 75", s.Overall)
	}
	want := []string{CategoryKeyword, CategoryTitle, CategorySeniority}
	if !slices.Equal(s.Contributing, want) {
		t.Errorf("contributing = %v, want %v", s.Contributing, want)
	}
}

// An input we cannot check is not a failure the candidate caused. Scoring it zero at full
// weight would tell them to rewrite a CV that was fine.
func TestUnavailableCategoryIsNotAZero(t *testing.T) {
	cats := []ScoredCategory{
		available(CategoryKeyword, 30, WeightKeyword),
		available(CategoryTitle, 20, WeightTitle),
		available(CategorySeniority, 10, WeightSeniority),
	}
	withUnavailable := aggregate(append(slices.Clone(cats), unavailable(CategoryRequirements, WeightRequirements)))
	withZero := aggregate(append(slices.Clone(cats), available(CategoryRequirements, 0, WeightRequirements)))

	if withUnavailable.Overall <= withZero.Overall {
		t.Errorf("unavailable scored %d, zero-at-full-weight scored %d: unavailable must score strictly higher",
			withUnavailable.Overall, withZero.Overall)
	}
	if withUnavailable.Overall != 100 {
		t.Errorf("three full categories with the fourth unavailable = %d, want 100", withUnavailable.Overall)
	}
}

// With every category available the denominator is 100, so the overall is the plain sum
// and no rounding can drift it.
func TestAllAvailableIsThePlainSum(t *testing.T) {
	s := aggregate([]ScoredCategory{
		available(CategoryRequirements, 31, WeightRequirements),
		available(CategoryKeyword, 17, WeightKeyword),
		available(CategoryTitle, 12, WeightTitle),
		available(CategorySeniority, 5, WeightSeniority),
	})
	if s.Overall != 65 {
		t.Errorf("overall = %d, want 65", s.Overall)
	}
	if len(s.Contributing) != 4 {
		t.Errorf("contributing = %v, want all four", s.Contributing)
	}
}

// Nothing evaluable means no score at all, not a zero. The caller renders the absence.
func TestNoAvailableCategoryYieldsNoScore(t *testing.T) {
	s := aggregate([]ScoredCategory{
		unavailable(CategoryRequirements, WeightRequirements),
		unavailable(CategoryKeyword, WeightKeyword),
		unavailable(CategoryTitle, WeightTitle),
		unavailable(CategorySeniority, WeightSeniority),
	})
	if s.Overall != 0 {
		t.Errorf("overall = %d, want 0", s.Overall)
	}
	if len(s.Contributing) != 0 {
		t.Errorf("contributing = %v, want empty", s.Contributing)
	}
}

func TestOverallRoundsRatherThanTruncates(t *testing.T) {
	// 7 of 10 available weight is 70; 2 of 3 categories' worth (17/30) rounds to 57, not 56.
	s := aggregate([]ScoredCategory{
		available(CategoryKeyword, 17, WeightKeyword),
		unavailable(CategoryRequirements, WeightRequirements),
		unavailable(CategoryTitle, WeightTitle),
		unavailable(CategorySeniority, WeightSeniority),
	})
	if s.Overall != 57 {
		t.Errorf("overall = %d, want 57 (17/30 rounded, not truncated)", s.Overall)
	}
}
