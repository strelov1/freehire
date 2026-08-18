package main

import (
	"reflect"
	"testing"
)

func TestPlanMerges_ElectsTheVariantWithTheMostJobs(t *testing.T) {
	// The real counterexamples from prod. Hyphens mark the corrupted spelling about as often
	// as the correct one, so "prefer the more readable slug" elects backwards; job count does
	// not.
	got := planMerges([]company{
		{Slug: "dominos", Name: "Dominos", JobCount: 14396},
		{Slug: "domino-s", Name: "Domino's", JobCount: 1},
		{Slug: "alfa-bank", Name: "Alfa Bank", JobCount: 1617},
		{Slug: "al-fa-bank", Name: "Al Fa Bank", JobCount: 20},
	}, nil, 0)

	canon := map[string]string{}
	for _, m := range got {
		for _, a := range m.Aliases {
			canon[a.Slug] = m.Canonical
		}
	}
	want := map[string]string{"domino-s": "dominos", "al-fa-bank": "alfa-bank"}
	if !reflect.DeepEqual(canon, want) {
		t.Errorf("elected %v, want %v", canon, want)
	}
}

func TestPlanMerges_LabelsWhyEachAliasRetires(t *testing.T) {
	// reason drives reversal: a legal-form merge is a pure rule the write path now applies on
	// its own, a spelling merge is a judgement only this election can make. Undoing one class
	// without the other is impossible if they are not told apart when recorded.
	got := planMerges([]company{
		{Slug: "ringcentral", Name: "RingCentral", JobCount: 66},
		{Slug: "ringcentral-inc", Name: "RingCentral, Inc.", JobCount: 2},
		{Slug: "dollar-tree", Name: "Dollar Tree", JobCount: 22683},
		{Slug: "dollartree", Name: "DollarTree", JobCount: 283},
	}, nil, 0)

	reasons := map[string]string{}
	for _, m := range got {
		for _, a := range m.Aliases {
			reasons[a.Slug] = a.Reason
		}
	}
	want := map[string]string{"ringcentral-inc": reasonLegalForm, "dollartree": reasonSpelling}
	if !reflect.DeepEqual(reasons, want) {
		t.Errorf("reasons = %v, want %v", reasons, want)
	}
}

func TestPlanMerges_RespectsAFrozenCanon(t *testing.T) {
	// Once a slug has been elected canonical it stays canonical, even when a later wave finds
	// a bigger variant. The alternative moves a URL that has already been 301-ing and indexed.
	got := planMerges([]company{
		{Slug: "acme", Name: "Acme", JobCount: 3},
		{Slug: "acme-inc", Name: "Acme Inc", JobCount: 900},
	}, map[string]bool{"acme": true}, 0)

	if len(got) != 1 {
		t.Fatalf("planned %d merges, want 1", len(got))
	}
	if got[0].Canonical != "acme" {
		t.Errorf("Canonical = %q, want acme — the frozen canon wins over the job count", got[0].Canonical)
	}
}

func TestPlanMerges_MinJobsBoundsTheWave(t *testing.T) {
	companies := []company{
		{Slug: "big", Name: "Big", JobCount: 900},
		{Slug: "big-inc", Name: "Big Inc", JobCount: 200},
		{Slug: "small", Name: "Small", JobCount: 2},
		{Slug: "small-inc", Name: "Small Inc", JobCount: 1},
	}
	got := planMerges(companies, nil, 1000)
	if len(got) != 1 || got[0].Canonical != "big" {
		t.Fatalf("planned %v, want only the group whose combined jobs reach 1000", got)
	}
}

func TestPlanMerges_IgnoresACompanyWithNoTwin(t *testing.T) {
	if got := planMerges([]company{{Slug: "solo", Name: "Solo", JobCount: 5}}, nil, 0); len(got) != 0 {
		t.Errorf("planned %v, want nothing — a company with no other spelling is not a merge", got)
	}
}

func TestPlanMerges_IsDeterministic(t *testing.T) {
	// A dry run a human reviewed must be the run that --apply then performs. Ties break on the
	// slug so the plan does not depend on map iteration order.
	companies := []company{
		{Slug: "tie-a", Name: "Tie A", JobCount: 7},
		{Slug: "tiea", Name: "TieA", JobCount: 7},
	}
	first := planMerges(companies, nil, 0)
	for range 20 {
		if !reflect.DeepEqual(planMerges(companies, nil, 0), first) {
			t.Fatal("planMerges is not deterministic across runs")
		}
	}
}
