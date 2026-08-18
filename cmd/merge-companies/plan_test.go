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

// TestPlanMerges_CanonicalIsAFixedPointOfTheSlugRule guards against electing a canonical slug
// the rule itself would never produce.
//
// Found in the first prod dry run: `danaher-corporation` outweighed `danaher` (714 open jobs),
// so pure job count elected it — and the catalogue's canonical url for the employer became the
// one carrying a corporate form, with the better-known slug 301ing INTO it. That inverts the
// change: the whole point is that the key does not carry the form.
//
// A slug still keying an employer under a form is also unstable. Every new posting derives the
// stripped slug, so the canon would depend forever on an alias row to reach itself.
func TestPlanMerges_CanonicalIsAFixedPointOfTheSlugRule(t *testing.T) {
	got := planMerges([]company{
		{Slug: "danaher-corporation", Name: "Danaher Corporation", JobCount: 900},
		{Slug: "danaher", Name: "Danaher", JobCount: 714},
	}, nil, 0)

	if len(got) != 1 {
		t.Fatalf("planned %d merges, want 1", len(got))
	}
	if got[0].Canonical != "danaher" {
		t.Errorf("Canonical = %q, want danaher — a slug carrying a corporate form is not a "+
			"canonical the rule can reproduce, whatever its job count", got[0].Canonical)
	}
}

// TestPlanMerges_JobCountStillDecidesBetweenFixedPoints: the fixed-point preference is a
// tie-break BEFORE job count, not a replacement for it. Both spellings here are ones the rule
// produces, so the bigger one still wins — including when it is the uglier of the two.
func TestPlanMerges_JobCountStillDecidesBetweenFixedPoints(t *testing.T) {
	got := planMerges([]company{
		{Slug: "dollartree", Name: "DollarTree", JobCount: 283},
		{Slug: "dollar-tree", Name: "Dollar Tree", JobCount: 22683},
	}, nil, 0)
	if got[0].Canonical != "dollar-tree" {
		t.Errorf("Canonical = %q, want dollar-tree", got[0].Canonical)
	}

	got = planMerges([]company{
		{Slug: "turner-townsend", Name: "Turner Townsend", JobCount: 16},
		{Slug: "turnertownsend", Name: "TurnerTownsend", JobCount: 400},
	}, nil, 0)
	if got[0].Canonical != "turnertownsend" {
		t.Errorf("Canonical = %q, want turnertownsend — both are fixed points, so the count "+
			"decides even though the hyphenated one reads better", got[0].Canonical)
	}
}

// TestPlanMerges_FrozenCanonWinsEvenIfItCarriesAForm: a canon already elected has been
// redirecting and indexing. Moving it would cost more than the tidier slug is worth.
func TestPlanMerges_FrozenCanonWinsEvenIfItCarriesAForm(t *testing.T) {
	got := planMerges([]company{
		{Slug: "acme-inc", Name: "Acme Inc", JobCount: 5},
		{Slug: "acme", Name: "Acme", JobCount: 900},
	}, map[string]bool{"acme-inc": true}, 0)
	if got[0].Canonical != "acme-inc" {
		t.Errorf("Canonical = %q, want acme-inc (frozen)", got[0].Canonical)
	}
}

// TestPlanMerges_FallsBackToTheDerivedSlug covers the group where NOTHING is a fixed point.
//
// The >=100-job wave surfaced four: carnival-corporation, dcs-corporation, quess-corp-limited,
// avaron-pte-ltd. Every member carried a form, so "the biggest fixed point" found none and the
// election fell back to the biggest member — leaving a canonical url with the form still on it,
// which is the outcome the fixed-point rule exists to prevent.
//
// The right canon is the one the rule itself yields, whether or not a company row holds it yet:
// that is the slug every future posting derives, and the reconcile creates the row.
func TestPlanMerges_FallsBackToTheDerivedSlug(t *testing.T) {
	got := planMerges([]company{
		{Slug: "carnival-corporation", Name: "Carnival Corporation", JobCount: 300},
		{Slug: "carnival-corporation-plc", Name: "Carnival Corporation plc", JobCount: 40},
	}, nil, 0)

	if len(got) != 1 {
		t.Fatalf("planned %d merges, want 1", len(got))
	}
	if got[0].Canonical != "carnival" {
		t.Errorf("Canonical = %q, want carnival — with no member the rule can reproduce, the "+
			"canon is what the rule yields, not the least bad row that happens to exist",
			got[0].Canonical)
	}
	// Both rows retire into it, including the one the election started from.
	if len(got[0].Aliases) != 2 {
		t.Errorf("got %d aliases, want 2 — every existing slug retires when none of them is "+
			"the canon", len(got[0].Aliases))
	}
}
