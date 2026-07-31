package cvmatch

import (
	"slices"
	"testing"
)

// The corroboration trap this rule exists for: read on its own, "5+ years of Go" carries no
// technical term to resolve the ambiguous "Go" against, so tagging the line in isolation
// yields nothing. Asking instead which of the VACANCY's skills the line names reads it
// correctly, because the vacancy's skills were resolved from its whole description.
func TestRequirementSkillsReadAnAmbiguousTermTheVacancyStates(t *testing.T) {
	got := requirementSkills("5+ years of Go", []string{"go", "kubernetes"})

	if !slices.Equal(got, []string{"go"}) {
		t.Errorf("requirementSkills = %v, want [go]", got)
	}
}

// A requirement is never attributed a skill the vacancy does not carry, so this category
// and Keyword Match draw from one set and cannot disagree about what the role asks for.
func TestRequirementSkillsNeverLeaveTheVacancysSet(t *testing.T) {
	got := requirementSkills("Experience with Kubernetes and Terraform", []string{"kubernetes"})

	if !slices.Equal(got, []string{"kubernetes"}) {
		t.Errorf("requirementSkills = %v, want [kubernetes] — terraform is not this vacancy's skill", got)
	}
}

func TestRequirementSkillsAreEmptyForProse(t *testing.T) {
	if got := requirementSkills("Strong communication skills", []string{"go"}); len(got) != 0 {
		t.Errorf("requirementSkills = %v, want empty", got)
	}
}

// A multi-word skill has to be found as a phrase; a word-by-word walk would miss it.
func TestRequirementSkillsResolveMultiWordSkills(t *testing.T) {
	got := requirementSkills("Experience building distributed systems", []string{"distributed-systems", "go"})

	if !slices.Equal(got, []string{"distributed-systems"}) {
		t.Errorf("requirementSkills = %v, want [distributed-systems]", got)
	}
}

func reqs() []Requirement {
	return []Requirement{
		{Text: "5+ years of Go", Priority: "required", CachedStatus: "missing-gap"},
		{Text: "Experience with Kubernetes", Priority: "preferred", CachedStatus: "covered"},
		{Text: "Strong communication skills", Priority: "required", CachedStatus: "covered"},
	}
}

// The cached status was determined against the base profile. The document is what is being
// edited, so the status follows the document — even when the two disagree.
func TestRequirementsCategoryFollowsTheDocumentNotTheCache(t *testing.T) {
	c, checks := requirementsCategory(reqs(), true, []string{"go"}, []string{"go", "kubernetes"})

	if !c.Available {
		t.Fatalf("category is unavailable: %q", c.Reason)
	}
	byText := map[string]RequirementCheck{}
	for _, r := range checks {
		byText[r.Text] = r
	}
	// The cache said this was a gap; the tailored document now states Go.
	if got := byText["5+ years of Go"]; got.Coverage != Covered {
		t.Errorf("Go requirement = %q, want covered — the document states it", got.Coverage)
	}
	// The cache said this was covered; the document does not mention Kubernetes.
	if got := byText["Experience with Kubernetes"]; got.Coverage != Missing {
		t.Errorf("Kubernetes requirement = %q, want missing — the document does not state it", got.Coverage)
	}
}

// A re-derived requirement must not carry a stale verdict beside a live one; an
// unverifiable one must, because the cached status is all anyone has for it.
func TestRequirementsCategoryCarriesTheCachedStatusOnlyWhenUnverifiable(t *testing.T) {
	_, checks := requirementsCategory(reqs(), true, []string{"go"}, []string{"go", "kubernetes"})

	for _, r := range checks {
		switch r.Coverage {
		case Unverifiable:
			if r.CachedStatus == "" {
				t.Errorf("unverifiable requirement %q dropped its cached status", r.Text)
			}
		default:
			if r.CachedStatus != "" {
				t.Errorf("re-derived requirement %q carried a cached status %q", r.Text, r.CachedStatus)
			}
		}
	}
}

// An unverifiable requirement leaves the category's own denominator — the recursive half of
// the rule. One of two checkable requirements covered is half of 40, not a third of it.
func TestRequirementsCategoryExcludesUnverifiableFromItsDenominator(t *testing.T) {
	c, _ := requirementsCategory([]Requirement{
		{Text: "5+ years of Go", Priority: "required"},
		{Text: "Experience with Kubernetes", Priority: "required"},
		{Text: "Strong communication skills", Priority: "required"},
	}, true, []string{"go"}, []string{"go", "kubernetes"})

	if c.Earned != WeightRequirements/2 {
		t.Errorf("earned = %d, want %d — the unverifiable requirement must leave the denominator",
			c.Earned, WeightRequirements/2)
	}
}

// A must-have carries more than a nice-to-have, so a CV covering the requirements the
// employer called required outscores one covering the same count of preferred ones.
func TestRequirementsCategoryWeighsRequiredAbovePreferred(t *testing.T) {
	list := []Requirement{
		{Text: "Experience with Kubernetes", Priority: "required"},
		{Text: "Proficiency in Python", Priority: "preferred"},
	}
	jobSkills := []string{"kubernetes", "python"}

	coversRequired, _ := requirementsCategory(list, true, []string{"kubernetes"}, jobSkills)
	coversPreferred, _ := requirementsCategory(list, true, []string{"python"}, jobSkills)

	if coversRequired.Earned <= coversPreferred.Earned {
		t.Errorf("covering the required requirement scored %d, the preferred one %d: required must weigh more",
			coversRequired.Earned, coversPreferred.Earned)
	}
}

// Partial coverage of a requirement naming several skills is not coverage.
func TestRequirementsCategoryNeedsEverySkillARequirementNames(t *testing.T) {
	_, checks := requirementsCategory(
		[]Requirement{{Text: "Experience with Kubernetes and Terraform", Priority: "required"}},
		true, []string{"kubernetes"}, []string{"kubernetes", "terraform"},
	)

	if len(checks) != 1 {
		t.Fatalf("checks = %d, want 1", len(checks))
	}
	if checks[0].Coverage != Missing {
		t.Errorf("coverage = %q, want missing — only one of the two skills is in the document", checks[0].Coverage)
	}
	if !slices.Equal(checks[0].Missing, []string{"terraform"}) {
		t.Errorf("missing = %v, want [terraform]", checks[0].Missing)
	}
}

// No fit analysis is cached for the pair: the category cannot be evaluated, and says so
// rather than reporting zero coverage of requirements nobody has read.
func TestRequirementsCategoryUnavailableWithoutACachedAnalysis(t *testing.T) {
	c, checks := requirementsCategory(nil, false, []string{"go"}, []string{"go"})

	if c.Available {
		t.Fatal("a pair with no cached analysis must make the category unavailable")
	}
	if c.Reason == "" {
		t.Error("an unavailable category must carry a reason")
	}
	if len(checks) != 0 {
		t.Errorf("checks = %v, want none", checks)
	}
}

// Every requirement unverifiable means there is nothing to divide by.
func TestRequirementsCategoryUnavailableWhenNothingIsCheckable(t *testing.T) {
	c, checks := requirementsCategory([]Requirement{
		{Text: "Strong communication skills", Priority: "required", CachedStatus: "covered"},
		{Text: "A degree in computer science", Priority: "preferred", CachedStatus: "missing-gap"},
	}, true, []string{"go"}, []string{"go"})

	if c.Available {
		t.Fatal("an all-unverifiable ledger must make the category unavailable")
	}
	// The checks still travel: the panel shows them labelled, it just cannot score them.
	if len(checks) != 2 {
		t.Errorf("checks = %d, want 2 — an unscored ledger is still shown", len(checks))
	}
}

// A vacancy that states no requirements at all is a different situation from one whose
// requirements name nothing checkable, and the reason must not describe the wrong one.
func TestRequirementsCategoryDistinguishesAnEmptyLedger(t *testing.T) {
	empty, _ := requirementsCategory(nil, true, []string{"go"}, []string{"go"})
	unreadable, _ := requirementsCategory([]Requirement{
		{Text: "Strong communication skills", Priority: "required"},
	}, true, []string{"go"}, []string{"go"})

	if empty.Available || unreadable.Available {
		t.Fatal("both cases must be unavailable")
	}
	if empty.Reason == unreadable.Reason {
		t.Errorf("both reasons read %q; an empty ledger is not an unreadable one", empty.Reason)
	}
}
