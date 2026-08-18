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

// TestPlanMerges_JobCountStillDecidesBetweenFixedPoints: the preferences are tie-breaks BEFORE
// job count, not replacements for it. Where neither the fixed-point rule nor the word shape of
// the name discriminates, the bigger spelling still wins.
//
// This once asserted that turnertownsend beats turner-townsend on volume. The employer writes
// two words, so the word-shape rule now takes it the other way — and that is the better url.
func TestPlanMerges_JobCountStillDecidesBetweenFixedPoints(t *testing.T) {
	got := planMerges([]company{
		{Slug: "dollartree", Name: "DollarTree", JobCount: 283},
		{Slug: "dollar-tree", Name: "Dollar Tree", JobCount: 22683},
	}, nil, 0)
	if got[0].Canonical != "dollar-tree" {
		t.Errorf("Canonical = %q, want dollar-tree", got[0].Canonical)
	}

	// Both spellings of one WORD: the count decides and nothing else has an opinion.
	got = planMerges([]company{
		{Slug: "dominos", Name: "Dominos", JobCount: 14396},
		{Slug: "domino-s", Name: "Domino's", JobCount: 1},
	}, nil, 0)
	if got[0].Canonical != "dominos" {
		t.Errorf("Canonical = %q, want dominos", got[0].Canonical)
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

// TestPlanMerges_PrefersTheSpellingTheNameIsWrittenIn decides the spelling class by the shape
// of the NAME, which is the signal job count alone cannot see.
//
// The >=100-job wave elects a squashed canonical over a hyphenated one 73 times out of 162
// spelling merges. Some are right — AT&T really is one word, so `att` beats `at-t` — and some
// are plainly wrong: `accenturefederalservices`, `acehardware`, `westerndigital`. What tells
// them apart is not the slug but whether the employer writes its name with spaces.
//
// This is also why "prefer the more hyphenated slug" failed and this does not: that rule read
// the slug, where a stray apostrophe looks exactly like a word break.
func TestPlanMerges_PrefersTheSpellingTheNameIsWrittenIn(t *testing.T) {
	t.Run("a multi-word name keeps its word breaks", func(t *testing.T) {
		got := planMerges([]company{
			{Slug: "westerndigital", Name: "WesternDigital", JobCount: 400},
			{Slug: "western-digital", Name: "Western Digital", JobCount: 126},
		}, nil, 0)
		if got[0].Canonical != "western-digital" {
			t.Errorf("Canonical = %q, want western-digital — the employer writes two words",
				got[0].Canonical)
		}
	})

	t.Run("no member is multi-word, so the count decides", func(t *testing.T) {
		// The counterexample that killed "prefer hyphens": the break in `domino-s` comes from
		// an apostrophe, not a space, and neither name has a space at all.
		got := planMerges([]company{
			{Slug: "dominos", Name: "Dominos", JobCount: 14396},
			{Slug: "domino-s", Name: "Domino's", JobCount: 1},
		}, nil, 0)
		if got[0].Canonical != "dominos" {
			t.Errorf("Canonical = %q, want dominos", got[0].Canonical)
		}
	})

	t.Run("every member is multi-word, so the count decides", func(t *testing.T) {
		// The other counterexample: both names carry spaces, so the shape says nothing and
		// the count correctly picks the un-corrupted spelling.
		got := planMerges([]company{
			{Slug: "alfa-bank", Name: "Alfa Bank", JobCount: 1617},
			{Slug: "al-fa-bank", Name: "Al Fa Bank", JobCount: 20},
		}, nil, 0)
		if got[0].Canonical != "alfa-bank" {
			t.Errorf("Canonical = %q, want alfa-bank", got[0].Canonical)
		}
	})

	t.Run("a hyphen in the name is a word break too", func(t *testing.T) {
		// Kimberly-Clark writes itself with a hyphen, which is as much a word break as a
		// space. Counting only spaces made it a one-word name and elected `kimberlyclark`.
		got := planMerges([]company{
			{Slug: "kimberlyclark", Name: "KimberlyClark", JobCount: 300},
			{Slug: "kimberly-clark", Name: "Kimberly-Clark", JobCount: 40},
		}, nil, 0)
		if got[0].Canonical != "kimberly-clark" {
			t.Errorf("Canonical = %q, want kimberly-clark", got[0].Canonical)
		}
	})

	t.Run("an apostrophe is NOT a word break", func(t *testing.T) {
		// The distinction the slug cannot make: Brink's is one word, and `brinks` beats
		// `brink-s` exactly as `dominos` beats `domino-s`.
		got := planMerges([]company{
			{Slug: "brinks", Name: "Brinks", JobCount: 200},
			{Slug: "brink-s", Name: "Brink's", JobCount: 10},
		}, nil, 0)
		if got[0].Canonical != "brinks" {
			t.Errorf("Canonical = %q, want brinks", got[0].Canonical)
		}
	})

	t.Run("a legal form is not a word that makes a name multi-word", func(t *testing.T) {
		// "Ace Hardware Corporation" is two words plus a form; it must not outrank a plain
		// "Ace Hardware", and both must beat the squashed spelling.
		got := planMerges([]company{
			{Slug: "acehardware", Name: "AceHardware", JobCount: 500},
			{Slug: "ace-hardware", Name: "Ace Hardware", JobCount: 90},
		}, nil, 0)
		if got[0].Canonical != "ace-hardware" {
			t.Errorf("Canonical = %q, want ace-hardware", got[0].Canonical)
		}
	})
}

// TestPlanMerges_MultiWordNameWinsEvenWhenOnlyAFormedRowHasIt closes the last gap the prod dry
// runs found, and it is a big one: 2,811 of 7,375 retiring slugs across the full catalogue.
//
// "Public Storage" only exists in the catalogue as `public-storage-inc`, a row carrying a form.
// Preferring a member that is ALREADY a fixed point dropped that row from consideration before
// its word shape could be read, leaving the squashed `publicstorage` to win by default.
//
// Deriving the canon from the elected member's NAME fixes it and needs no extra rule: the
// derived slug is a fixed point by construction, so the fixed-point preference disappears.
func TestPlanMerges_MultiWordNameWinsEvenWhenOnlyAFormedRowHasIt(t *testing.T) {
	got := planMerges([]company{
		{Slug: "publicstorage", Name: "PublicStorage", JobCount: 300},
		{Slug: "public-storage-inc", Name: "Public Storage Inc", JobCount: 40},
	}, nil, 0)

	if got[0].Canonical != "public-storage" {
		t.Errorf("Canonical = %q, want public-storage — the employer writes two words, and a "+
			"corporate form on the row that says so is not a reason to ignore it",
			got[0].Canonical)
	}
	if len(got[0].Aliases) != 2 {
		t.Errorf("got %d aliases, want 2 — no existing row holds the canon", len(got[0].Aliases))
	}
}

// TestPlanMerges_SeesASlugWhoseIndexCounterIsZero is the bug that stranded 8,375 rows on
// `jpmorganchase` after wave 1 had merged it.
//
// The planner read companies.job_count, which counts the postings the SEARCH INDEX holds — not
// the rows this worker rewrites. A retired slug drops to 0 in the index while its unmoved rows
// stay in the table, so filtering on that counter made exactly the leftovers of a merge
// invisible: the better the merge worked, the more reliably its remainder hid.
//
// The query now reads jobs, so a slug with rows and a zero counter is planned like any other.
// This test pins the planning side of it: a group whose members report 0 open jobs must still
// merge.
func TestPlanMerges_SeesASlugWhoseIndexCounterIsZero(t *testing.T) {
	got := planMerges([]company{
		{Slug: "jp-morgan-chase", Name: "JP Morgan Chase", JobCount: 0},
		{Slug: "jpmorganchase", Name: "JPMorganChase", JobCount: 0},
	}, nil, 0)

	if len(got) != 1 {
		t.Fatalf("planned %d merges, want 1 — a slug the search index has forgotten still has "+
			"rows in the table, and those rows are what this worker exists to move", len(got))
	}
	if got[0].Canonical != "jp-morgan-chase" {
		t.Errorf("Canonical = %q, want jp-morgan-chase", got[0].Canonical)
	}
}
