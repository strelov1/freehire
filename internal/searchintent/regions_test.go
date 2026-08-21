package searchintent

import (
	"slices"
	"testing"
)

// Regions are disjoint areas — the UK is its own region, not part of eu — so choosing
// one already answers "which area". An exclusion on top can then only strip the roles
// that span BOTH areas, which is not what "somewhere in Europe but not the UK" asks
// for: a pan-European role is exactly what that person wants.
//
// The prompt says so too, but it says so about half the time. This is the deterministic
// half.

func TestResolveDropsAnExcludedRegionWhenAnotherRegionIsChosen(t *testing.T) {
	got, err := intent{
		Facets:  map[string][]string{"regions": {"eu"}},
		Exclude: map[string][]string{"regions": {"uk"}},
	}.resolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !slices.Equal(got.Facets["regions"], []string{"eu"}) {
		t.Fatalf("regions = %v, want [eu]", got.Facets["regions"])
	}
	if len(got.Exclude["regions"]) != 0 {
		t.Fatalf("excluded regions = %v, want none — choosing Europe already leaves the UK out",
			got.Exclude["regions"])
	}
}

// With no region chosen there is nothing to choose instead, so the exclusion is the
// only way to say it and must survive.
func TestResolveKeepsAnExcludedRegionWhenNoRegionIsChosen(t *testing.T) {
	got, err := intent{
		Facets:  map[string][]string{"work_mode": {"remote"}},
		Exclude: map[string][]string{"regions": {"north_america"}},
	}.resolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !slices.Equal(got.Exclude["regions"], []string{"north_america"}) {
		t.Fatalf("excluded regions = %v, want [north_america] — nothing positive says it", got.Exclude["regions"])
	}
}

// A country is INSIDE a region, so choosing the region says nothing about the country:
// "somewhere in Europe but not Germany" needs the exclusion, and the rule above must
// not reach it.
func TestResolveKeepsAnExcludedCountryUnderAChosenRegion(t *testing.T) {
	got, err := intent{
		Facets:  map[string][]string{"regions": {"eu"}},
		Exclude: map[string][]string{"countries": {"de"}},
	}.resolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !slices.Equal(got.Exclude["countries"], []string{"de"}) {
		t.Fatalf("excluded countries = %v, want [de] — Germany is inside the chosen region", got.Exclude["countries"])
	}
}

// Skills are requirements, not places: "Go but not PHP" is a real, non-redundant ask,
// and the region rule must not generalise to it.
func TestResolveKeepsAnExcludedSkillBesideAnIncludedOne(t *testing.T) {
	got, err := intent{
		Facets:  map[string][]string{"skills": {"go"}},
		Exclude: map[string][]string{"skills": {"php"}},
	}.resolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !slices.Equal(got.Exclude["skills"], []string{"php"}) {
		t.Fatalf("excluded skills = %v, want [php]", got.Exclude["skills"])
	}
}
