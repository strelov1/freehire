package searchintent

import (
	"context"
	"slices"
	"strings"
	"testing"
)

// "Remote, but not in the USA" is how people actually describe a search, and a filter
// that can only add is a filter that answers a different question. Exclusions run
// through the same dictionaries as inclusions — an unresolved exclusion is the worse
// half of the silent-filter failure, since it hides postings rather than showing too
// many.

func TestResolveExcludesCanonicalisedValues(t *testing.T) {
	got, err := proposal{
		WorkMode: []string{"remote"},
		Exclude:  exclusions{Countries: []string{"United States"}, Skills: []string{"PHP"}},
	}.intent().resolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !slices.Equal(got.Exclude["countries"], []string{"us"}) {
		t.Fatalf("excluded countries = %v, want [us]", got.Exclude["countries"])
	}
	if !slices.Equal(got.Exclude["skills"], []string{"php"}) {
		t.Fatalf("excluded skills = %v, want [php]", got.Exclude["skills"])
	}
	if !slices.Equal(got.Facets["work_mode"], []string{"remote"}) {
		t.Fatalf("work_mode = %v, want [remote]", got.Facets["work_mode"])
	}
}

func TestResolveDropsAndReportsUnresolvableExclusion(t *testing.T) {
	got, err := proposal{Exclude: exclusions{Skills: []string{"bad vibes"}}}.intent().resolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(got.Exclude["skills"]) != 0 {
		t.Fatalf("excluded skills = %v, want none", got.Exclude["skills"])
	}
	if !slices.Contains(got.Unresolved, "bad vibes") {
		t.Fatalf("unresolved = %v, want it to name the dropped exclusion", got.Unresolved)
	}
}

// An exclusion is a search: "anything but PHP" narrows the catalogue even with no
// inclusion beside it.
func TestExclusionAloneIsNotAnEmptyResult(t *testing.T) {
	got, err := proposal{Exclude: exclusions{Skills: []string{"PHP"}}}.intent().resolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.Empty() {
		t.Fatal("Empty() = true, want false — an exclusion is a filter")
	}
}

func TestExclusionsCoverOnlyRealFilters(t *testing.T) {
	for name := range (proposal{}).intent().Exclude {
		if _, ok := facetResolvers[name]; !ok {
			t.Errorf("the model may exclude %q, but nothing resolves it", name)
		}
	}
}

// The profile seed exists so a caller who has already told us what they want does not
// have to type it again. What they told us must reach the model as material, and their
// excluded skills must reach it as exclusions — importing them as wants would build the
// exact search they said they did not want.
func TestProfileSeedReachesTheModel(t *testing.T) {
	got, err := userPrompt(Request{Profile: &Profile{
		Specializations: []string{"backend"},
		Skills:          []string{"go", "kubernetes"},
		ExcludedSkills:  []string{"php"},
		Locations:       []string{"Portugal"},
		Headline:        "Backend engineer, 8 years",
	}})
	if err != nil {
		t.Fatalf("userPrompt: %v", err)
	}
	for _, want := range []string{"backend", "kubernetes", "Portugal", "Backend engineer, 8 years"} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt does not carry %q", want)
		}
	}
	if !strings.Contains(strings.ToLower(got), "avoid") && !strings.Contains(strings.ToLower(got), "exclude") {
		t.Errorf("prompt carries the excluded skills without saying they are unwanted:\n%s", got)
	}
}

func TestInterpretRefusesAProfileWithNothingInIt(t *testing.T) {
	in := modelSaying(t, proposal{})
	if _, err := in.Interpret(context.Background(), Request{Profile: &Profile{}}); err == nil {
		t.Fatal("Interpret accepted an empty profile — there is nothing to build a search from")
	}
}

// A refinement must argue with the search that is live, so the model is shown the
// previous result and asked for a complete replacement rather than a diff.
func TestRefinementCarriesThePreviousSearch(t *testing.T) {
	previous := Result{
		Facets:  map[string][]string{"work_mode": {"onsite"}, "countries": {"pt"}},
		Scalars: Scalars{PostedWithinDays: ptr(7)},
	}
	got, err := userPrompt(Request{Text: "actually remote", Previous: &previous})
	if err != nil {
		t.Fatalf("userPrompt: %v", err)
	}
	for _, want := range []string{"onsite", "pt", "7", "actually remote"} {
		if !strings.Contains(got, want) {
			t.Errorf("refinement prompt does not carry %q:\n%s", want, got)
		}
	}
}
