package collections

import (
	"reflect"
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/normalize"
)

func TestRegistry_HasV1Set(t *testing.T) {
	if _, ok := Lookup("yc"); !ok {
		t.Error("registry missing yc")
	}
	if _, ok := Lookup("bigtech"); !ok {
		t.Error("registry missing bigtech")
	}
	if _, ok := Lookup("does-not-exist"); ok {
		t.Error("Lookup returned ok for unknown slug")
	}
}

func TestRegistry_LookupReturnsEntry(t *testing.T) {
	c, ok := Lookup("yc")
	if !ok {
		t.Fatal("yc not found")
	}
	if c.Slug != "yc" || c.Title == "" || c.Description == "" {
		t.Errorf("yc entry incomplete: %+v", c)
	}
}

func TestSlugs_MatchesRegistry(t *testing.T) {
	got := Slugs()
	if len(got) != len(All) {
		t.Fatalf("Slugs() len = %d, want %d", len(got), len(All))
	}
	set := make(map[string]struct{}, len(got))
	for _, s := range got {
		set[s] = struct{}{}
	}
	for _, c := range All {
		if _, ok := set[c.Slug]; !ok {
			t.Errorf("Slugs() missing %q", c.Slug)
		}
	}
}

func TestHandListSlugs_AreCanonical(t *testing.T) {
	// Every hand-list collection (bigtech, mag7, ai, …) must hold canonical slugs
	// (idempotent under normalization), so the list matches our company slugs
	// without surprises. Dataset-backed collections (yc, unicorn, …) carry no Slugs.
	for _, c := range All {
		if len(c.Slugs) == 0 {
			continue
		}
		for _, s := range c.Slugs {
			if got := normalize.Slug(s); got != s {
				t.Errorf("collection %q slug %q is not canonical (normalizes to %q)", c.Slug, s, got)
			}
		}
	}
}

func TestMatch_SplitsPresentAndAbsentDedupedSorted(t *testing.T) {
	existing := map[string]struct{}{"stripe": {}, "airbnb": {}}
	// "Stripe" and "stripe " both normalize to stripe (dup); "Airbnb" matches;
	// "Unknown Co" does not.
	matched, unmatched := Match([]string{"Stripe", "stripe", "Airbnb", "Unknown Co"}, existing)

	if !reflect.DeepEqual(matched, []string{"airbnb", "stripe"}) {
		t.Errorf("matched = %#v, want [airbnb stripe] (deduped, sorted)", matched)
	}
	if !reflect.DeepEqual(unmatched, []string{"Unknown Co"}) {
		t.Errorf("unmatched = %#v, want [Unknown Co]", unmatched)
	}
}

func TestReconcile(t *testing.T) {
	managed := []string{"yc", "bigtech"}
	cases := []struct {
		name    string
		current []string
		want    []string
		out     []string
	}{
		{"adds a managed tag", []string{}, []string{"yc"}, []string{"yc"}},
		{"drops a managed tag no longer wanted", []string{"yc"}, []string{}, []string{}},
		{"swaps managed tags", []string{"yc"}, []string{"bigtech"}, []string{"bigtech"}},
		{"preserves an unmanaged tag", []string{"custom", "yc"}, []string{"bigtech"}, []string{"bigtech", "custom"}},
		{"deduplicates and sorts", []string{"yc", "yc"}, []string{"bigtech", "yc"}, []string{"bigtech", "yc"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Reconcile(tc.current, managed, tc.want)
			if !reflect.DeepEqual(got, tc.out) {
				t.Errorf("Reconcile(%v, managed, %v) = %#v, want %#v", tc.current, tc.want, got, tc.out)
			}
		})
	}
}

func TestParseYC_ExtractsNames(t *testing.T) {
	payload := []byte(`[
		{"id": 1, "name": "Stripe", "slug": "stripe", "website": "https://stripe.com", "batch": "S09"},
		{"id": 2, "name": "Airbnb", "slug": "airbnb", "website": "https://airbnb.com", "batch": "W09"}
	]`)
	names, err := ParseYC(payload)
	if err != nil {
		t.Fatalf("ParseYC: %v", err)
	}
	if !reflect.DeepEqual(names, []string{"Stripe", "Airbnb"}) {
		t.Errorf("names = %#v, want [Stripe Airbnb]", names)
	}
}

func TestParseYC_RejectsGarbage(t *testing.T) {
	if _, err := ParseYC([]byte("not json")); err == nil {
		t.Error("ParseYC accepted invalid JSON")
	}
}

func TestParseCompanyCSV_ExtractsCompanyColumn(t *testing.T) {
	// Company is not the first column; the parser must locate it by header, not
	// index, and tolerate quoted fields with commas (the Investors column).
	csv := `Updated at,Company,Last Valuation,Investors
"x",Stripe,95,"[""Sequoia"",""a16z""]"
"x",Canva,40,"[""Blackbird""]"
"x",,0,"[]"`
	names, err := ParseCompanyCSV([]byte(csv))
	if err != nil {
		t.Fatalf("ParseCompanyCSV: %v", err)
	}
	if !reflect.DeepEqual(names, []string{"Stripe", "Canva"}) { // empty name skipped
		t.Errorf("names = %#v, want [Stripe Canva]", names)
	}
}

func TestParseCompanyCSV_RequiresCompanyColumn(t *testing.T) {
	if _, err := ParseCompanyCSV([]byte("Name,Valuation\nStripe,95")); err == nil {
		t.Error("ParseCompanyCSV accepted a CSV without a Company column")
	}
}

func TestParseTechstarsCSV_SemicolonNameColumn(t *testing.T) {
	// Techstars CSV is semicolon-separated with the company in a "name" column.
	csv := "name;urls;description\n" +
		"Sentry;https://sentry.io;Error monitoring\n" +
		"DigitalOcean;https://do.com;Cloud\n" +
		";;empty name skipped"
	names, err := ParseTechstarsCSV([]byte(csv))
	if err != nil {
		t.Fatalf("ParseTechstarsCSV: %v", err)
	}
	if !reflect.DeepEqual(names, []string{"Sentry", "DigitalOcean"}) {
		t.Errorf("names = %#v, want [Sentry DigitalOcean]", names)
	}
}

func TestParseEUStartups_ExtractsNameField(t *testing.T) {
	// icp-radar uses a capitalised "Name" field.
	payload := []byte(`[{"Name":"Revolut","Country":"UK"},{"Name":"Spotify","Country":"Sweden"},{"Name":""}]`)
	names, err := ParseEUStartups(payload)
	if err != nil {
		t.Fatalf("ParseEUStartups: %v", err)
	}
	if !reflect.DeepEqual(names, []string{"Revolut", "Spotify"}) {
		t.Errorf("names = %#v, want [Revolut Spotify]", names)
	}
}

func TestRegistry_HasUnicorn(t *testing.T) {
	c, ok := Lookup("unicorn")
	if !ok || c.Dataset == nil {
		t.Fatalf("unicorn collection missing or has no dataset: %+v ok=%v", c, ok)
	}
}

func TestRetiredSlugs_AreNotLiveCollections(t *testing.T) {
	// A retired slug must be absent from All (else it is a live collection, not
	// retired) — the invariant that lets import-collections manage-then-strip it.
	live := make(map[string]struct{})
	for _, s := range Slugs() {
		live[s] = struct{}{}
	}
	for _, r := range RetiredSlugs {
		if _, ok := live[r]; ok {
			t.Errorf("retired slug %q is still a live collection in All", r)
		}
	}
	// russian-roots was renamed to eastern-roots; it must be retired so its stale
	// tags get cleaned up.
	if !slices.Contains(RetiredSlugs, "russian-roots") {
		t.Error("russian-roots missing from RetiredSlugs after rename to eastern-roots")
	}
}

func TestParseSlugList_SkipsBlanksAndComments(t *testing.T) {
	data := []byte("# header comment\n\nabbyy\n  jetbrains  \n# mid comment\nrevolut\n")
	got, err := ParseSlugList(data)
	if err != nil {
		t.Fatalf("ParseSlugList: %v", err)
	}
	if !reflect.DeepEqual(got, []string{"abbyy", "jetbrains", "revolut"}) {
		t.Errorf("got = %#v, want [abbyy jetbrains revolut]", got)
	}
}

func TestEasternRoots_EmbeddedDatasetResolves(t *testing.T) {
	c, ok := Lookup("eastern-roots")
	if !ok || c.Dataset == nil {
		t.Fatalf("eastern-roots collection missing or has no dataset: %+v ok=%v", c, ok)
	}
	if len(c.Dataset.Data) == 0 {
		t.Fatal("eastern-roots dataset has no embedded data")
	}
	names, err := c.Dataset.Parse(c.Dataset.Data)
	if err != nil {
		t.Fatalf("parse embedded eastern-roots: %v", err)
	}
	if len(names) < 50 {
		t.Errorf("eastern-roots slugs = %d, want a substantial list", len(names))
	}
	// The embedded slugs must be canonical (Match normalizes them, but a
	// non-canonical entry signals a bad edit to the committed file).
	for _, s := range names {
		if got := normalize.Slug(s); got != s {
			t.Errorf("non-canonical slug %q (normalizes to %q)", s, got)
		}
	}
}
