package recentfeed

import (
	"fmt"
	"testing"
)

func TestGroup_SingleJobProducesSingleEntry(t *testing.T) {
	entries := Group([]Posting{
		{Title: "Senior Backend Engineer", CompanyName: "Acme", JobSlug: "acme-sbe"},
	})
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1: %+v", len(entries), entries)
	}
	e := entries[0]
	if e.Kind != KindSingle {
		t.Errorf("Kind = %q, want %q", e.Kind, KindSingle)
	}
	if e.Title != "Senior Backend Engineer" || e.CompanyName != "Acme" || e.JobSlug != "acme-sbe" {
		t.Errorf("unexpected entry: %+v", e)
	}
	if e.Count != 0 {
		t.Errorf("Count = %d on a single entry, want 0 (unused)", e.Count)
	}
}

func TestGroup_BelowThresholdClusterYieldsOneSingleEntryEach(t *testing.T) {
	postings := make([]Posting, AggregationThreshold-1)
	for i := range postings {
		postings[i] = Posting{
			Title:       "Senior Backend Engineer",
			CompanyName: "Company",
			JobSlug:     "slug",
		}
	}
	entries := Group(postings)
	if len(entries) != len(postings) {
		t.Fatalf("got %d entries, want %d (one per posting, below threshold)", len(entries), len(postings))
	}
	for _, e := range entries {
		if e.Kind != KindSingle {
			t.Errorf("Kind = %q, want %q for a below-threshold cluster", e.Kind, KindSingle)
		}
	}
}

func TestGroup_AtThresholdClusterYieldsOneAggregateEntry(t *testing.T) {
	postings := make([]Posting, AggregationThreshold)
	for i := range postings {
		postings[i] = Posting{
			Title:       "Senior Backend Engineer",
			CompanyName: "Company",
			JobSlug:     "slug",
		}
	}
	entries := Group(postings)
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1 aggregated entry: %+v", len(entries), entries)
	}
	e := entries[0]
	if e.Kind != KindAggregate {
		t.Errorf("Kind = %q, want %q", e.Kind, KindAggregate)
	}
	if e.Count != AggregationThreshold {
		t.Errorf("Count = %d, want %d", e.Count, AggregationThreshold)
	}
	if e.Title != "Senior Backend Engineer" {
		t.Errorf("Title = %q, want the representative posting's title", e.Title)
	}
	if e.CompanyName == "" {
		t.Error("CompanyName is empty on an aggregate entry, want a representative company name")
	}
}

func TestGroup_DifferentRolesStayInSeparateBuckets(t *testing.T) {
	entries := Group([]Posting{
		{Title: "Senior Backend Engineer", CompanyName: "A", JobSlug: "a"},
		{Title: "Senior Frontend Engineer", CompanyName: "B", JobSlug: "b"},
	})
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2 (distinct roles must not merge): %+v", len(entries), entries)
	}
}

func TestGroup_CosmeticTitleVariationStillClusters(t *testing.T) {
	postings := make([]Posting, AggregationThreshold)
	for i := range postings {
		title := "Senior Backend Engineer"
		if i%2 == 0 {
			title = "senior backend engineer, Remote"
		}
		postings[i] = Posting{Title: title, CompanyName: "Company", JobSlug: "slug"}
	}
	entries := Group(postings)
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1 (cosmetic title variants must cluster together): %+v", len(entries), entries)
	}
	if entries[0].Kind != KindAggregate {
		t.Errorf("Kind = %q, want %q", entries[0].Kind, KindAggregate)
	}
}

func TestGroup_EmptyBatchProducesNoEntries(t *testing.T) {
	if entries := Group(nil); len(entries) != 0 {
		t.Errorf("got %d entries for an empty batch, want 0", len(entries))
	}
}

// One company posting many DIFFERENT roles at once (e.g. a mass hiring push) must
// not flood the feed with one card per posting — it should collapse the same way a
// same-role burst across companies does, just keyed the other way round.
func TestGroup_CompanyBurstOfDifferentRolesAggregates(t *testing.T) {
	postings := make([]Posting, AggregationThreshold)
	for i := range postings {
		postings[i] = Posting{
			Title:       fmt.Sprintf("Role %d", i),
			CompanyName: "Amyx, Inc.",
			CompanySlug: "amyx",
			JobSlug:     fmt.Sprintf("slug-%d", i),
		}
	}
	entries := Group(postings)
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1 company-aggregated entry: %+v", len(entries), entries)
	}
	e := entries[0]
	if e.Kind != KindCompanyAggregate {
		t.Errorf("Kind = %q, want %q", e.Kind, KindCompanyAggregate)
	}
	if e.CompanyName != "Amyx, Inc." {
		t.Errorf("CompanyName = %q, want %q", e.CompanyName, "Amyx, Inc.")
	}
	if e.Count != AggregationThreshold {
		t.Errorf("Count = %d, want %d", e.Count, AggregationThreshold)
	}
}

func TestGroup_BelowThresholdCompanyBurstStaysIndividual(t *testing.T) {
	postings := make([]Posting, AggregationThreshold-1)
	for i := range postings {
		postings[i] = Posting{
			Title:       fmt.Sprintf("Role %d", i),
			CompanyName: "Amyx, Inc.",
			CompanySlug: "amyx",
			JobSlug:     fmt.Sprintf("slug-%d", i),
		}
	}
	entries := Group(postings)
	if len(entries) != len(postings) {
		t.Fatalf("got %d entries, want %d (one per posting, below threshold)", len(entries), len(postings))
	}
	for _, e := range entries {
		if e.Kind != KindSingle {
			t.Errorf("Kind = %q, want %q for a below-threshold company burst", e.Kind, KindSingle)
		}
	}
}

// The company key is CompanySlug, not the free-text CompanyName — two spellings
// of one employer ("Acme, Inc." vs "Acme Inc", the exact reason
// company_slug/company_slug_aliases exist — see docs/agents/company-identity.md)
// must still collapse into one card, and a genuinely different company sharing
// no slug must never merge with it just because of a coincidental name.
func TestGroup_CompanyBurstCollapsesDespiteNameSpellingVariants(t *testing.T) {
	postings := make([]Posting, AggregationThreshold)
	for i := range postings {
		name := "Acme, Inc."
		if i%2 == 0 {
			name = "Acme Inc"
		}
		postings[i] = Posting{
			Title:       fmt.Sprintf("Role %d", i),
			CompanyName: name,
			CompanySlug: "acme",
			JobSlug:     fmt.Sprintf("slug-%d", i),
		}
	}
	entries := Group(postings)
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1 (same company_slug must collapse despite differing display names): %+v", len(entries), entries)
	}
	if entries[0].Kind != KindCompanyAggregate {
		t.Errorf("Kind = %q, want %q", entries[0].Kind, KindCompanyAggregate)
	}
}

// A burst that already qualifies as a role-aggregate (same role, many companies)
// must not also be counted toward company aggregation — each posting belongs to
// exactly one entry.
func TestGroup_RoleAggregateTakesPrecedenceOverCompanyAggregate(t *testing.T) {
	postings := make([]Posting, AggregationThreshold)
	for i := range postings {
		postings[i] = Posting{
			Title:       "Senior Backend Engineer",
			CompanyName: fmt.Sprintf("Company %d", i),
			JobSlug:     fmt.Sprintf("slug-%d", i),
		}
	}
	entries := Group(postings)
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1 role-aggregated entry: %+v", len(entries), entries)
	}
	if entries[0].Kind != KindAggregate {
		t.Errorf("Kind = %q, want %q", entries[0].Kind, KindAggregate)
	}
}
