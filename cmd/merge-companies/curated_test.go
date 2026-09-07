package main

import (
	"testing"

	"github.com/strelov1/freehire/internal/dict/normalize"
)

// TestCuratedAliasesAreWellFormed is the guard that replaces reviewing the list by eye. Every
// property here is one a wrong entry would break silently: the plan would still print, --apply
// would still write, and the damage would be a public URL pointing at the wrong employer.
func TestCuratedAliasesAreWellFormed(t *testing.T) {
	canons := curatedCanons(curatedAliases)

	for aliasSlug, canon := range curatedAliases {
		if aliasSlug == "" || canon == "" {
			t.Errorf("empty entry %q -> %q", aliasSlug, canon)
			continue
		}
		if aliasSlug == canon {
			// The database CHECK would reject this too, but at 3am mid-wave rather than in CI.
			t.Errorf("%q retires into itself", aliasSlug)
		}
		// A chain (a -> b, b -> c) would leave `a` pointing at a slug that no longer holds
		// jobs. The registry stores one hop and resolves one hop, so the second is never taken.
		if _, chained := curatedAliases[canon]; chained {
			t.Errorf("%q -> %q, but %q is itself retiring: the registry resolves one hop, so "+
				"%q would land on a slug with no jobs", aliasSlug, canon, canon, aliasSlug)
		}
		// A canon has to survive the slug rule unchanged. Every future posting from the
		// surviving board derives its slug through CompanySlug, so a canon the rule would
		// rewrite could never be reached by an ordinary crawl — the company would depend on
		// an alias row forever to name itself. The same requirement is why electCanonical
		// derives the canon from a name rather than reusing a stored slug.
		if normalize.CompanySlug(canon) != canon {
			t.Errorf("canonical %q is not a fixed point of CompanySlug: every future posting "+
				"derives a different slug, so the company could never name itself", canon)
		}
		// A slug that is a canon for one entry and an alias in another is the same chain seen
		// from the other end, and it makes the group membership depend on map order.
		if canons[aliasSlug] {
			t.Errorf("%q is both retiring and canonical", aliasSlug)
		}
	}
}

// exadel is the catalogue shape the curated list was built for: one employer running two
// Greenhouse boards that disagree about the name, plus two stray artefacts. No two of these
// four names fold to the same CompanyKey, which is exactly why the rule cannot reach them.
func exadelCompanies() []company {
	return []company{
		{Slug: "exadel", Name: "Exadel", JobCount: 99},
		{Slug: "exadel-inc-website", Name: "Exadel Inc (Website)", JobCount: 50},
		{Slug: "exadel-1", Name: "Exadel 1", JobCount: 2},
		{Slug: "exadelinc", Name: "exadelinc", JobCount: 1},
	}
}

var exadelCurated = map[string]string{
	"exadel-inc-website": "exadel",
	"exadel-1":           "exadel",
	"exadelinc":          "exadel",
}

func TestPlanMerges_CuratedGroupOverridesTheFold(t *testing.T) {
	got := planMerges(exadelCompanies(), nil, 0, exadelCurated)

	if len(got) != 1 {
		t.Fatalf("planned %d merges, want 1 — the four names fold four different ways, so "+
			"only the curated list can group them", len(got))
	}
	if got[0].Canonical != "exadel" {
		t.Errorf("Canonical = %q, want exadel", got[0].Canonical)
	}
	if len(got[0].Aliases) != 3 {
		t.Fatalf("retiring %d slugs, want 3", len(got[0].Aliases))
	}
	// The canon carries the most jobs here, but that is incidental: the list decides, not the
	// count. See TestPlanMerges_CuratedCanonWinsAgainstTheJobCount.
	for _, a := range got[0].Aliases {
		if a.Slug == "exadel" {
			t.Error("the canonical slug is retiring into itself")
		}
	}
}

// TestPlanMerges_CuratedAliasCarriesItsOwnFold is the whole reason folded_key moved onto the
// alias. Ingest resolves by folded_key, so each retiring spelling has to record the fold its
// OWN name produces — otherwise the next crawl of that board mints the duplicate again while
// the merge reports success.
func TestPlanMerges_CuratedAliasCarriesItsOwnFold(t *testing.T) {
	got := planMerges(exadelCompanies(), nil, 0, exadelCurated)
	if len(got) != 1 {
		t.Fatalf("planned %d merges, want 1", len(got))
	}

	want := map[string]string{
		"exadel-inc-website": "exadelincwebsite",
		"exadel-1":           "exadel1",
		"exadelinc":          "exadelinc",
	}
	for _, a := range got[0].Aliases {
		if w, ok := want[a.Slug]; !ok {
			t.Errorf("unexpected alias %q", a.Slug)
		} else if a.FoldedKey != w {
			t.Errorf("alias %q folded_key = %q, want %q", a.Slug, a.FoldedKey, w)
		}
	}
}

// TestPlanMerges_CuratedCanonWinsAgainstTheJobCount: the list is a judgement and outranks the
// election. Sopra Steria is the real case — the artefact board carries 1336 open jobs against
// the real company's 337, so an election would retire the company into its own artefact.
func TestPlanMerges_CuratedCanonWinsAgainstTheJobCount(t *testing.T) {
	got := planMerges([]company{
		{Slug: "sopra-steria", Name: "Sopra Steria", JobCount: 337},
		{Slug: "soprasteria1", Name: "SopraSteria1", JobCount: 1336},
	}, nil, 0, map[string]string{"soprasteria1": "sopra-steria"})

	if len(got) != 1 {
		t.Fatalf("planned %d merges, want 1", len(got))
	}
	if got[0].Canonical != "sopra-steria" {
		t.Errorf("Canonical = %q, want sopra-steria — the curated list decides, not the count",
			got[0].Canonical)
	}
}

// TestPlanMerges_CuratedGroupOfOneStillMerges: a curated entry whose canon holds no catalogue
// row is still a merge. A slug no row holds yet is a legitimate canon (the reconcile after the
// re-key creates it), and judging a curated group by member count would drop exactly the entry
// somebody took the trouble to write down.
func TestPlanMerges_CuratedGroupOfOneStillMerges(t *testing.T) {
	got := planMerges([]company{
		{Slug: "acme-2", Name: "Acme 2", JobCount: 7},
	}, nil, 0, map[string]string{"acme-2": "acme"})

	if len(got) != 1 {
		t.Fatalf("planned %d merges, want 1", len(got))
	}
	if got[0].Canonical != "acme" || len(got[0].Aliases) != 1 {
		t.Errorf("got canonical %q with %d aliases, want acme with 1",
			got[0].Canonical, len(got[0].Aliases))
	}
}

// TestPlanMerges_CuratedCanonLeavesItsFoldedGroup: the canon must join its curated group and
// not stay in the folded one it would otherwise share. Left behind, the curated members would
// have no one to merge into — a plan that prints and moves nothing.
func TestPlanMerges_CuratedCanonLeavesItsFoldedGroup(t *testing.T) {
	got := planMerges([]company{
		{Slug: "acme", Name: "Acme", JobCount: 50},
		{Slug: "acme-inc", Name: "Acme Inc", JobCount: 4}, // folds onto acme by the rule
		{Slug: "acme-2", Name: "Acme 2", JobCount: 7},     // reaches acme only via the list
	}, nil, 0, map[string]string{"acme-2": "acme"})

	if len(got) != 1 {
		t.Fatalf("planned %d merges, want 1 — the folded member and the curated one belong to "+
			"the same employer and must not split into two groups", len(got))
	}
	if got[0].Canonical != "acme" {
		t.Errorf("Canonical = %q, want acme", got[0].Canonical)
	}
	if len(got[0].Aliases) != 2 {
		t.Errorf("retiring %d slugs, want 2 (acme-inc by the rule, acme-2 by the list)",
			len(got[0].Aliases))
	}
}

// TestPlanMerges_WithoutCuratedListNothingChanges pins that the list is additive: an ordinary
// folded group plans exactly as it did before, and its alias records the group's own fold.
func TestPlanMerges_WithoutCuratedListNothingChanges(t *testing.T) {
	got := planMerges([]company{
		{Slug: "dollar-tree", Name: "Dollar Tree", JobCount: 22683},
		{Slug: "dollartree", Name: "DollarTree", JobCount: 283},
	}, nil, 0, nil)

	if len(got) != 1 {
		t.Fatalf("planned %d merges, want 1", len(got))
	}
	if got[0].Canonical != "dollar-tree" {
		t.Errorf("Canonical = %q, want dollar-tree", got[0].Canonical)
	}
	if len(got[0].Aliases) != 1 || got[0].Aliases[0].FoldedKey != "dollartree" {
		t.Errorf("aliases = %+v, want one carrying folded_key dollartree", got[0].Aliases)
	}
}
