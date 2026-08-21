package searchintent

import (
	"slices"
	"strings"
	"testing"
)

// A saved profile is already written in the filter's own vocabulary: specializations
// are category values, skills are canonical tags, the location block holds work modes
// and ISO codes. Asking a model to translate it would be paying to guess at something
// already stated exactly — and giving it the chance to guess wrong.

func TestFromProfileMapsWhatIsAlreadyCanonical(t *testing.T) {
	got := FromProfile(Profile{
		Specializations: []string{"backend"},
		Skills:          []string{"go", "kubernetes"},
		ExcludedSkills:  []string{"php"},
		WorkModes:       []string{"remote"},
		RemoteFrom:      []string{"eu", "pt"},
	})
	if !slices.Equal(got.Facets["category"], []string{"backend"}) {
		t.Fatalf("category = %v, want [backend]", got.Facets["category"])
	}
	if !slices.Equal(got.Facets["skills"], []string{"go", "kubernetes"}) {
		t.Fatalf("skills = %v", got.Facets["skills"])
	}
	if !slices.Equal(got.Exclude["skills"], []string{"php"}) {
		t.Fatalf("excluded skills = %v, want [php]", got.Exclude["skills"])
	}
	if !slices.Equal(got.Facets["work_mode"], []string{"remote"}) {
		t.Fatalf("work_mode = %v, want [remote]", got.Facets["work_mode"])
	}
	if !slices.Equal(got.Facets["regions"], []string{"eu"}) {
		t.Fatalf("regions = %v, want [eu]", got.Facets["regions"])
	}
	if !slices.Equal(got.Facets["countries"], []string{"pt"}) {
		t.Fatalf("countries = %v, want [pt]", got.Facets["countries"])
	}
}

// Where they live now is not where they want to work. Someone based in Lisbon who is
// open to relocating did not ask for jobs in Portugal, and importing their address as a
// filter would answer the opposite of their question.
func TestFromProfileDoesNotSearchWhereTheyMerelyLive(t *testing.T) {
	got := FromProfile(Profile{
		Skills:     []string{"go"},
		BasedIn:    "Lisbon, pt",
		Relocating: true,
		RelocateTo: []string{"de"},
	})
	if slices.Contains(got.Facets["countries"], "pt") {
		t.Fatalf("countries = %v — their home address is not a search", got.Facets["countries"])
	}
	if !slices.Contains(got.Facets["countries"], "de") {
		t.Fatalf("countries = %v, want the place they said they would move to", got.Facets["countries"])
	}
}

// The profile's own values pass through untouched, but they were stored by an older
// version of a vocabulary that has since changed — so they go through the same
// resolution as a model's proposal, and a value that no longer exists is reported, not
// applied.
func TestFromProfileDropsAValueTheVocabularyNoLongerHas(t *testing.T) {
	got := FromProfile(Profile{Specializations: []string{"retired_category"}, Skills: []string{"go"}})
	if len(got.Facets["category"]) != 0 {
		t.Fatalf("category = %v, want none", got.Facets["category"])
	}
	if !slices.Contains(got.Unresolved, "retired_category") {
		t.Fatalf("unresolved = %v, want it to name the dropped value", got.Unresolved)
	}
}

// The summary is what the dialog shows instead of the raw values, so a profile-built
// result needs one too — composed from what was actually written, not from a model.
func TestFromProfileDescribesItself(t *testing.T) {
	got := FromProfile(Profile{
		Specializations: []string{"backend"},
		Skills:          []string{"go"},
		WorkModes:       []string{"remote"},
	})
	if got.Summary == "" {
		t.Fatal("summary is empty")
	}
	for _, want := range []string{"backend", "go", "remote"} {
		if !strings.Contains(strings.ToLower(got.Summary), want) {
			t.Errorf("summary %q does not mention %q", got.Summary, want)
		}
	}
}

func TestFromProfileWithNothingInItIsEmpty(t *testing.T) {
	if !FromProfile(Profile{}).Empty() {
		t.Fatal("an empty profile built a search")
	}
}
