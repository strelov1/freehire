package cvmatch_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/cvmatch"
)

func fullInput() cvmatch.Input {
	return cvmatch.Input{
		CVText:    "JANE DOE\nSenior Data Engineer at Acme\nBuilt data engineering pipelines in Python on Kubernetes.",
		CVSkills:  []string{"python", "kubernetes"},
		JobTitle:  "Senior Data Engineer",
		JobSkills: []string{"python", "kubernetes", "terraform"},
		Requirements: []cvmatch.Requirement{
			{Text: "Proficiency in Python", Priority: "required"},
			{Text: "Experience with Terraform", Priority: "preferred", CachedStatus: "missing-gap"},
			{Text: "Strong communication skills", Priority: "required", CachedStatus: "covered"},
		},
		HasAnalysis: true,
	}
}

func categoryByID(t *testing.T, s cvmatch.Score, id string) cvmatch.Category {
	t.Helper()
	for _, c := range s.Categories {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("score carries no %q category", id)
	return cvmatch.Category{}
}

func TestComputeScoresAllFourCategories(t *testing.T) {
	s := cvmatch.Compute(fullInput())

	if len(s.Categories) != 4 {
		t.Fatalf("categories = %d, want 4", len(s.Categories))
	}
	if len(s.Contributing) != 4 {
		t.Errorf("contributing = %v, want all four", s.Contributing)
	}
	if s.Overall <= 0 || s.Overall > 100 {
		t.Errorf("overall = %d, want a score in 1..100", s.Overall)
	}
	if !slices.Equal(s.MissingSkills, []string{"terraform"}) {
		t.Errorf("missing skills = %v, want [terraform]", s.MissingSkills)
	}
	if len(s.Requirements) != 3 {
		t.Errorf("requirement checks = %d, want 3", len(s.Requirements))
	}
}

// The categories always travel in one order, so the panel's rows do not reshuffle between
// two reads of the same CV.
func TestComputeOrdersCategoriesByWeight(t *testing.T) {
	s := cvmatch.Compute(fullInput())

	want := []string{cvmatch.CategoryRequirements, cvmatch.CategoryKeyword, cvmatch.CategoryTitle, cvmatch.CategorySeniority}
	var got []string
	for _, c := range s.Categories {
		got = append(got, c.ID)
	}
	if !slices.Equal(got, want) {
		t.Errorf("category order = %v, want %v", got, want)
	}
}

// Editing the document moves the score. This is the property the whole panel exists for.
func TestComputeFollowsTheDocument(t *testing.T) {
	before := cvmatch.Compute(fullInput())

	after := fullInput()
	after.CVText += "\nBuilt infrastructure with Terraform."
	after.CVSkills = append(after.CVSkills, "terraform")
	improved := cvmatch.Compute(after)

	if improved.Overall <= before.Overall {
		t.Errorf("adding a required skill moved the score from %d to %d; it must rise", before.Overall, improved.Overall)
	}
}

// No cached fit analysis: the heaviest category drops out and the rest still score, out of
// their own combined weight.
func TestComputeWithoutAnalysisScoresTheRemainingThree(t *testing.T) {
	in := fullInput()
	in.HasAnalysis = false
	in.Requirements = nil

	s := cvmatch.Compute(in)

	reqs := categoryByID(t, s, cvmatch.CategoryRequirements)
	if reqs.Available {
		t.Error("requirements coverage must be unavailable without a cached analysis")
	}
	if slices.Contains(s.Contributing, cvmatch.CategoryRequirements) {
		t.Errorf("contributing = %v, must exclude requirements coverage", s.Contributing)
	}
	if len(s.Contributing) != 3 {
		t.Errorf("contributing = %v, want the other three", s.Contributing)
	}
	if s.Overall == 0 {
		t.Error("the remaining three categories must still produce a score")
	}
}

// A vacancy nothing resolves against yields no score at all rather than a zero the
// candidate did not earn. The caller renders the absence.
func TestComputeWithNothingResolvableYieldsNoScore(t *testing.T) {
	s := cvmatch.Compute(cvmatch.Input{
		CVText:   "Pastry chef with fifteen years in Michelin kitchens.",
		JobTitle: "",
	})

	if len(s.Contributing) != 0 {
		t.Errorf("contributing = %v, want empty", s.Contributing)
	}
	if s.Overall != 0 {
		t.Errorf("overall = %d, want 0", s.Overall)
	}
	if len(s.Categories) != 4 {
		t.Errorf("categories = %d, want all four reported with their reasons", len(s.Categories))
	}
	for _, c := range s.Categories {
		if c.Reason == "" {
			t.Errorf("unavailable category %q carries no reason", c.ID)
		}
	}
}

// Pure and repeatable: the same input scores the same twice, with no map iteration or
// clock leaking into the result.
func TestComputeIsDeterministic(t *testing.T) {
	first := cvmatch.Compute(fullInput())
	second := cvmatch.Compute(fullInput())

	if first.Overall != second.Overall {
		t.Errorf("overall differed between runs: %d then %d", first.Overall, second.Overall)
	}
	if !reflect.DeepEqual(first, second) {
		t.Errorf("the same input scored differently twice:\n%+v\n%+v", first, second)
	}
}
