package talentnetwork

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/platform/db"
)

// A membership wide enough that a count can be wrong in an interesting way: three backend
// people so a value clears the floor, and one of everything else so a value does not.
func countableMembers(base time.Time) []db.ListTalentNetworkMembersRow {
	backend := func(skills string) string {
		return `{"total_years":8,"skills":[` + skills + `],
		  "experience":[{"title":"Senior Backend Engineer","current":true}]}`
	}
	return []db.ListTalentNetworkMembersRow{
		memberRow("backend-aaaa", "Europe/Berlin", "berlin", backend(`"Go","PostgreSQL"`), base),
		memberRow("backend-bbbb", "Europe/Lisbon", "lisbon", backend(`"Go","Kubernetes"`), base.Add(-time.Hour)),
		memberRow("backend-cccc", "America/New_York", "new-york", backend(`"Go","Kubernetes"`), base.Add(-2*time.Hour)),
		memberRow("frontend-dddd", "Europe/Berlin", "berlin", frontendCV, base.Add(-3*time.Hour)),
	}
}

func newCountableCatalogue(t *testing.T, base time.Time) *Catalogue {
	t.Helper()
	store := &fakeCatalogueStore{rows: countableMembers(base)}
	return NewCatalogue(store, time.Minute, func() time.Time { return base })
}

func TestFacets_CountsEveryValuePresent(t *testing.T) {
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	c := newCountableCatalogue(t, base)

	got, err := c.Facets(context.Background(), Query{})
	if err != nil {
		t.Fatalf("Facets: %v", err)
	}
	if got.Total != 4 {
		t.Errorf("total = %d, want 4", got.Total)
	}
	if n := got.Facets["categories"]["backend"]; n != 3 {
		t.Errorf("backend = %d, want 3", n)
	}
	if n := got.Facets["skills"]["go"]; n != 3 {
		t.Errorf("go = %d, want 3", n)
	}
	if n := got.Facets["tz"]["Europe"]; n != 3 {
		t.Errorf("Europe = %d, want 3", n)
	}
}

// A value nobody carries is absent, not zero. A pane offering "Rust (0)" cannot be told
// apart from one offering a skill whose members have all left.
func TestFacets_OmitsValuesNobodyCarries(t *testing.T) {
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	c := newCountableCatalogue(t, base)

	got, _ := c.Facets(context.Background(), Query{})
	if _, present := got.Facets["skills"]["rust"]; present {
		t.Error("a skill nobody carries was reported")
	}
	if _, present := got.Facets["categories"]["ml_ai"]; present {
		t.Error("a category nobody carries was reported")
	}
}

// THE trap. Counting a facet under its own selection makes every other value in the pane
// read zero, so a visitor can never add a second value and the control silently becomes
// single-select. The fix — drop that facet's own selection before counting it — is
// invisible from outside, which is why it gets its own test.
func TestFacets_AFacetIsCountedWithoutItsOwnSelection(t *testing.T) {
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	c := newCountableCatalogue(t, base)

	got, err := c.Facets(context.Background(), Query{Skills: []string{"go"}})
	if err != nil {
		t.Fatalf("Facets: %v", err)
	}
	// Two members carry Kubernetes, so its count clears the floor and a real number is
	// the assertion. A value below the floor would report zero for a different reason,
	// and this test must not pass for that one.
	if n := got.Facets["skills"]["kubernetes"]; n != 2 {
		t.Errorf("kubernetes = %d under a `go` selection, want 2 — the skills pane just became single-select", n)
	}
	if n := got.Facets["skills"]["go"]; n != 3 {
		t.Errorf("go = %d under its own selection, want the unfiltered 3", n)
	}
}

// Every OTHER filter still applies while a facet is counted, or the counts describe a
// catalogue the visitor is not looking at.
func TestFacets_OtherFiltersStillNarrow(t *testing.T) {
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	c := newCountableCatalogue(t, base)

	got, _ := c.Facets(context.Background(), Query{Skills: []string{"go"}, TimezoneRegions: []string{"Europe"}})
	// Two of the three Go members are in Europe, so the skills pane counts within Europe.
	if n := got.Facets["skills"]["go"]; n != 2 {
		t.Errorf("go = %d under a Europe filter, want 2", n)
	}
	// And the timezone pane counts within the Go selection: three members are in Europe,
	// but only two of those carry Go.
	if n := got.Facets["tz"]["Europe"]; n != 2 {
		t.Errorf("Europe = %d under a go filter, want 2 — the skills selection must still narrow it", n)
	}
}

// The number is withheld below the floor; the OPTION is not. Hiding it would be the worse
// answer — the member is in the catalogue either way and the list already shows them, so
// removing the value only makes the filter unusable.
func TestFacets_WithholdsANumberThatWouldNameSomebody(t *testing.T) {
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	c := newCountableCatalogue(t, base)

	got, _ := c.Facets(context.Background(), Query{})
	// Exactly one member is a frontend one.
	n, present := got.Facets["categories"]["frontend"]
	if !present {
		t.Fatal("the value was dropped entirely — the filter is now unusable for it")
	}
	if n != CountWithheld {
		t.Errorf("frontend = %d, want CountWithheld (%d) — the number is withheld below the floor, the option is not", n, CountWithheld)
	}
	// And it must NOT be zero: zero already means "nobody carries this", and a pane that
	// received both as 0 would render the person who exists exactly like the value nobody
	// has.
	if n == 0 {
		t.Error("withheld reported as 0, which is indistinguishable from a value nobody carries")
	}
}

// Two counting paths over one snapshot is exactly the pair that drifts, so one test drives
// both and compares.
func TestFacets_TotalAgreesWithTheList(t *testing.T) {
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	c := newCountableCatalogue(t, base)

	for _, q := range []Query{
		{},
		{Skills: []string{"go"}},
		{TimezoneRegions: []string{"Europe"}},
		{Categories: []string{"backend"}, MinYears: 5},
	} {
		page, err := c.List(context.Background(), q)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		facets, err := c.Facets(context.Background(), q)
		if err != nil {
			t.Fatalf("Facets: %v", err)
		}
		if page.Total != facets.Total {
			t.Errorf("query %+v: list says %d, facets say %d", q, page.Total, facets.Total)
		}
	}
}

// Every facet the query filters on is a facet the panes can COUNT, and vice versa. The two
// lists are read by different callers — FacetParams by the browser (through the generated
// contract) and facetOf by Facets — and a param in one alone fails silently in both
// directions: a filter with no counts, or counts for something nobody can select.
func TestFacetParamsAndCountableFacetsAreTheSameSet(t *testing.T) {
	t.Parallel()

	for _, param := range FacetParams() {
		if _, ok := facetOf[param]; !ok {
			t.Errorf("%q is filterable but not countable — its pane would show no numbers", param)
		}
	}
	for param := range facetOf {
		if !slices.Contains(FacetParams(), param) {
			t.Errorf("%q is countable but not in FacetParams — the browser would never offer it", param)
		}
	}
}
