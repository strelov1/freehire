package main

import (
	"slices"
	"testing"
)

func TestMatchingProvidersMatchesBareAndDashedMembers(t *testing.T) {
	providers := []string{"whatjobs", "whatjobs-de", "whatjobs-uk", "himalayas", "whatjobsomething"}
	got := matchingProviders(providers, []string{"whatjobs"})
	want := []string{"whatjobs", "whatjobs-de", "whatjobs-uk"}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("matchingProviders() = %v, want %v", got, want)
	}
}

func TestMatchingProvidersEmptyOnNoMatch(t *testing.T) {
	got := matchingProviders([]string{"himalayas", "jobicy"}, []string{"whatjobs"})
	if len(got) != 0 {
		t.Fatalf("matchingProviders() = %v, want empty", got)
	}
}
