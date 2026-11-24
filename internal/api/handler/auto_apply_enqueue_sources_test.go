package handler

import (
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/job/jobview"
)

// TestAutoApplyEnqueueSourcesCoverTheFacetProviders pins the relationship between the two
// provider lists that layering keeps apart (jobview is layer 5 and cannot see handler).
// They are deliberately different sets, not two copies of one:
//
//   - autoApplyEnqueueSources: an attempt may be QUEUED. Every entry here is tailored and
//     reviewed, whatever it can do afterwards.
//   - jobview.AutoApplyProviders: the "auto-apply available" facet, where a fill may be
//     SUBMITTED. atsapply's own test holds it equal to fillProviders ∪ browserUseProviders.
//
// Every provider the facet advertises must be queueable, or the filter offers a posting whose
// button can never start an attempt. The reverse is wider by exactly recruitee: it is queued
// and reviewed, then parks because no submit path drives it.
//
// Without this test a provider added to the facet but not here, or removed from here while
// still advertised, passes every check in both packages.
func TestAutoApplyEnqueueSourcesCoverTheFacetProviders(t *testing.T) {
	var unqueueable []string
	for provider := range jobview.AutoApplyProviders {
		if !autoApplyEnqueueSources[provider] {
			unqueueable = append(unqueueable, provider)
		}
	}
	if len(unqueueable) > 0 {
		slices.Sort(unqueueable)
		t.Errorf("the auto-apply facet advertises providers that cannot be queued: %v", unqueueable)
	}

	var notAdvertised []string
	for provider := range autoApplyEnqueueSources {
		if !jobview.AutoApplyProviders[provider] {
			notAdvertised = append(notAdvertised, provider)
		}
	}
	slices.Sort(notAdvertised)
	// An exact set, not a subset: a second queueable-but-unsubmittable provider is a product
	// decision to make on purpose (and to document), not one to drift into.
	if want := []string{"recruitee"}; !slices.Equal(notAdvertised, want) {
		t.Errorf("queueable but absent from the auto-apply facet = %v, want exactly %v", notAdvertised, want)
	}
}
