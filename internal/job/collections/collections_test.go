package collections

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/dict/normalize"
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
			if got := normalize.CompanySlug(s); got != s {
				t.Errorf("collection %q slug %q is not canonical (normalizes to %q)", c.Slug, s, got)
			}
		}
	}
}

func TestMembers_SplitsPresentAndAbsentDedupedSorted(t *testing.T) {
	companies := map[string]Company{"stripe": {Slug: "stripe"}, "airbnb": {Slug: "airbnb"}}
	// "Stripe" and "stripe" both normalize to stripe (dup); "Airbnb" matches;
	// "Unknown Co" does not.
	c := Collection{Slug: "x", Kind: KindEditorial}
	matched, stat := c.Members([]Record{{Name: "Stripe"}, {Name: "stripe"}, {Name: "Airbnb"}, {Name: "Unknown Co"}}, companies)

	if !reflect.DeepEqual(matched, []string{"airbnb", "stripe"}) {
		t.Errorf("matched = %#v, want [airbnb stripe] (deduped, sorted)", matched)
	}
	if !reflect.DeepEqual(stat.UnmatchedNames, []string{"Unknown Co"}) {
		t.Errorf("unmatched = %#v, want [Unknown Co]", stat.UnmatchedNames)
	}
	if stat.Gated != 0 || stat.Ambiguous != 0 {
		t.Errorf("an editorial collection reported gate/ambiguity counts: %+v", stat)
	}
}

func TestMembers_EditorialMatchingUsesTheCatalogueSlugRule(t *testing.T) {
	// This once asserted the opposite: the suffix strip belonged to credentials only, and an
	// editorial dataset naming "Acme Robotics Limited" had to land on acme-robotics-limited
	// rather than on "a different company called acme-robotics".
	//
	// Those are no longer two companies. The catalogue keys by normalize.CompanySlug, so
	// acme-robotics-limited is a slug ingest cannot produce — matching on it finds nothing,
	// and an editorial collection that finds nothing reports no error, it just holds fewer
	// companies than it names.
	companies := map[string]Company{"acme-robotics": {Slug: "acme-robotics"}}
	c := Collection{Slug: "x", Kind: KindEditorial}
	matched, _ := c.Members([]Record{{Name: "Acme Robotics Limited"}}, companies)
	if !reflect.DeepEqual(matched, []string{"acme-robotics"}) {
		t.Errorf("editorial match = %#v, want [acme-robotics]", matched)
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

func TestNames_AdaptsANameParserToRecords(t *testing.T) {
	got, err := names(func([]byte) ([]string, error) { return []string{"Acme", "Globex"}, nil })(nil)
	if err != nil {
		t.Fatalf("names adapter returned error: %v", err)
	}
	want := []Record{{Name: "Acme"}, {Name: "Globex"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("names adapter = %+v, want %+v", got, want)
	}
}

func TestNames_PropagatesParserError(t *testing.T) {
	_, err := names(func([]byte) ([]string, error) { return nil, errBoom })(nil)
	if err == nil {
		t.Error("names adapter swallowed the parser error")
	}
}

func TestRecord_CarriesSourceMetadata(t *testing.T) {
	r := Record{Name: "Acme Robotics Ltd", Meta: map[string]string{"route": "Skilled Worker"}}
	if r.Meta["route"] != "Skilled Worker" {
		t.Errorf("record metadata lost: %+v", r)
	}
}

func TestRegisterSlug_StripsTrailingLegalSuffix(t *testing.T) {
	cases := []struct{ in, want string }{
		// normalize.Slug alone yields acme-robotics-limited, which never matches our
		// acme-robotics — the whole reason this step exists.
		{"ACME ROBOTICS LIMITED", "acme-robotics"},
		{"Acme Robotics Ltd", "acme-robotics"},
		{"Acme Robotics Ltd.", "acme-robotics"},
		{"Monzo Bank PLC", "monzo-bank"},
		{"Foo Bar LLP", "foo-bar"},
		// The strip repeats, so a compound form comes off whole: CIC, then Co.
		{"Community Co CIC", "community"},
		{"Booking B.V.", "booking"},
		{"Adyen N.V.", "adyen"},
		{"Adyen NV", "adyen"},
		{"Acme Robotics Inc", "acme-robotics"},
		{"Acme Robotics Inc.", "acme-robotics"},
		{"Acme Robotics Incorporated", "acme-robotics"},
		{"Acme Robotics Corp", "acme-robotics"},
		{"Acme Robotics Corp.", "acme-robotics"},
		{"Acme Robotics Corporation", "acme-robotics"},
		{"Acme Robotics LLC", "acme-robotics"},
		// No suffix to strip.
		{"Monzo", "monzo"},
		{"Deliveroo", "deliveroo"},
	}
	for _, tc := range cases {
		if got := RegisterSlug(tc.in); got != tc.want {
			t.Errorf("RegisterSlug(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRegisterSlug_StripsCo(t *testing.T) {
	// "Co" was once excluded on the theory that it collides with ordinary short words
	// where Inc/Corp/LLC do not. The catalogue does not bear that out: all 297 companies
	// whose slug ends in "-co" are "& Co." forms (Tiffany & Co., Levi Strauss & Co.,
	// JPMorgan Chase & Co.), the strip is trailing-only so an interior collision cannot
	// arise, and stripping it is what merges jpmorgan-chase-co into jp-morgan-chase.
	//
	// The test that matters is not whether a token looks dangerous but whether stripping
	// it lands on a DIFFERENT existing employer. Measured on prod 2026-08-17 over the
	// tokens this list added: of the 25 largest such merges, 25 are correct.
	if got := RegisterSlug("Acme Robotics Co"); got != "acme-robotics" {
		t.Errorf("RegisterSlug(Acme Robotics Co) = %q, want acme-robotics", got)
	}
}

func TestRegisterSlug_OnlyStripsAtTheEnd(t *testing.T) {
	// "Limited" leading the name is part of the name, not a legal form — even
	// though the trailing "Inc" is now a recognised suffix and does strip.
	if got := RegisterSlug("LIMITED BRANDS INC"); got != "limited-brands" {
		t.Errorf("RegisterSlug(LIMITED BRANDS INC) = %q, want limited-brands", got)
	}
	if got := RegisterSlug("Limited Brands"); got != "limited-brands" {
		t.Errorf("RegisterSlug(Limited Brands) = %q, want limited-brands", got)
	}
}

func TestRegisterSlug_NeverReducesANameToNothing(t *testing.T) {
	// A register row that is nothing but a legal form must not normalize to the empty
	// slug — an empty slug would match nothing, but stripping to it hides a bad row.
	for _, in := range []string{"Limited", "LTD", "B.V."} {
		if got := RegisterSlug(in); got == "" {
			t.Errorf("RegisterSlug(%q) stripped the whole name away", in)
		}
	}
}

func TestRegisterSlug_StripsOnlyOneSuffix(t *testing.T) {
	// Chained forms are rare and ambiguous; strip one and stop rather than peel a
	// name down to a fragment.
	if got := RegisterSlug("Acme Holdings Ltd"); got != "acme-holdings" {
		t.Errorf("RegisterSlug(Acme Holdings Ltd) = %q, want acme-holdings", got)
	}
}

func TestGate_NilAdmitsEveryNameMatch(t *testing.T) {
	c := Collection{Slug: "x", Kind: KindEditorial}
	if !c.Admits(Company{Slug: "acme"}, Record{Name: "Acme"}) {
		t.Error("a gateless collection rejected a name match")
	}
}

func TestGate_DecidesWhenSet(t *testing.T) {
	c := Collection{
		Slug: "x",
		Kind: KindCredential,
		Gate: func(co Company, r Record) bool { return r.Meta["route"] == "Skilled Worker" },
	}
	if c.Admits(Company{Slug: "acme"}, Record{Meta: map[string]string{"route": "Skilled Worker"}}) != true {
		t.Error("gate rejected a record it should admit")
	}
	if c.Admits(Company{Slug: "acme"}, Record{Meta: map[string]string{"route": "Seasonal Worker"}}) != false {
		t.Error("gate admitted a record it should reject")
	}
}

func TestCompany_CarriesTheAttributesAGateNeeds(t *testing.T) {
	co := Company{Slug: "acme", Countries: []string{"GB", "US"}, HQCountry: "GB"}
	if co.Slug != "acme" || co.HQCountry != "GB" || len(co.Countries) != 2 {
		t.Errorf("company shape lost detail: %+v", co)
	}
}

func TestDataset_SourceIsExactlyOneOfURLDataResolverRecords(t *testing.T) {
	cases := []struct {
		name string
		ds   Dataset
		ok   bool
	}{
		{"url only", Dataset{URL: "https://x/y.json"}, true},
		{"data only", Dataset{Data: []byte("x")}, true},
		{"resolver only", Dataset{ResolveURL: func(context.Context, *http.Client) (string, error) { return "", nil }}, true},
		{"records only", Dataset{Records: func(context.Context, *http.Client) ([]Record, error) { return nil, nil }}, true},
		{"none", Dataset{}, false},
		{"url and data", Dataset{URL: "https://x", Data: []byte("x")}, false},
		{"url and resolver", Dataset{URL: "https://x", ResolveURL: func(context.Context, *http.Client) (string, error) { return "", nil }}, false},
		{"url and records", Dataset{URL: "https://x", Records: func(context.Context, *http.Client) ([]Record, error) { return nil, nil }}, false},
		{"data and records", Dataset{Data: []byte("x"), Records: func(context.Context, *http.Client) ([]Record, error) { return nil, nil }}, false},
		{"resolver and records", Dataset{
			ResolveURL: func(context.Context, *http.Client) (string, error) { return "", nil },
			Records:    func(context.Context, *http.Client) ([]Record, error) { return nil, nil },
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.ds.Valid(); got != tc.ok {
				t.Errorf("Valid() = %v, want %v", got, tc.ok)
			}
		})
	}
}

func TestRegistry_EveryDatasetDeclaresExactlyOneSource(t *testing.T) {
	for _, c := range All {
		if c.Dataset != nil && !c.Dataset.Valid() {
			t.Errorf("collection %q dataset declares no single source", c.Slug)
		}
	}
}

func TestRegistry_EveryEntryDeclaresAKind(t *testing.T) {
	// The zero Kind is deliberately invalid: kind decides which filter group a tag
	// renders in, so a forgotten one must fail here rather than silently defaulting.
	for _, c := range All {
		switch c.Kind {
		case KindEditorial, KindCredential, KindBacker:
		default:
			t.Errorf("collection %q declares no valid kind (got %q)", c.Slug, c.Kind)
		}
	}
}

func TestRegistry_CuratedThemesAreEditorial(t *testing.T) {
	// A theme we assembled ourselves — by size, valuation, geography or sector. No
	// outside body selected these members, which is what separates them from the
	// backers (see TestRegistry_BackersNameWhoSelectedTheCompany).
	for _, slug := range []string{
		"european", "ai", "mag7",
		"bigtech", "unicorn", "fortune500", "eastern-roots", "ai-native",
	} {
		c, ok := Lookup(slug)
		if !ok {
			t.Errorf("registry missing %q", slug)
			continue
		}
		if c.Kind != KindEditorial {
			t.Errorf("collection %q kind = %q, want %q", slug, c.Kind, KindEditorial)
		}
	}
}

func TestRegistry_BackersNameWhoSelectedTheCompany(t *testing.T) {
	// An accelerator or fund that picked the company is a different sort of tag from a
	// theme we curate ourselves: it names an outside selector, so it renders as that
	// brand's mark rather than as one more filter chip.
	for _, slug := range []string{"yc", "techstars", "a16z-portfolio", "a16z-speedrun"} {
		c, ok := Lookup(slug)
		if !ok {
			t.Errorf("registry missing %q", slug)
			continue
		}
		if c.Kind != KindBacker {
			t.Errorf("collection %q kind = %q, want %q", slug, c.Kind, KindBacker)
		}
		if c.Title == "" || c.Description == "" {
			t.Errorf("collection %q missing display copy: %+v", slug, c)
		}
	}
}

func TestRegistry_TheTwoA16zTagsAreSeparateCollections(t *testing.T) {
	// Being held in the fund's portfolio and having been selected into its
	// accelerator are different facts of different strength. Merging them would
	// present a seed-stage cohort company and OpenAI as carrying the same signal.
	portfolio, ok := Lookup("a16z-portfolio")
	if !ok {
		t.Fatal("a16z-portfolio missing")
	}
	speedrun, ok := Lookup("a16z-speedrun")
	if !ok {
		t.Fatal("a16z-speedrun missing")
	}
	for _, c := range []Collection{portfolio, speedrun} {
		if c.Dataset == nil || c.Dataset.Records == nil {
			t.Errorf("%s does not source its membership from the directory: %+v", c.Slug, c.Dataset)
			continue
		}
		if !c.Dataset.Valid() {
			t.Errorf("%s declares more than one payload source", c.Slug)
		}
	}
	if portfolio.Title == speedrun.Title {
		t.Errorf("both a16z tags share the title %q — a reader cannot tell them apart", portfolio.Title)
	}
}

func TestRegistry_HasUnicorn(t *testing.T) {
	c, ok := Lookup("unicorn")
	if !ok || c.Dataset == nil {
		t.Fatalf("unicorn collection missing or has no dataset: %+v ok=%v", c, ok)
	}
}

func TestRegistry_HasAINative(t *testing.T) {
	c, ok := Lookup("ai-native")
	if !ok || len(c.Slugs) == 0 {
		t.Fatalf("ai-native collection missing or has no member slugs: %+v ok=%v", c, ok)
	}
	if c.Title == "" || c.Description == "" {
		t.Errorf("ai-native collection missing display copy: %+v", c)
	}
	// Members are the canonical slugs of our ingested companies (matched by
	// normalize.Slug), so the forms that differ from the brand name must be present.
	want := []string{"deepgram", "openrouter-ai", "trmlabs", "qdrant-tech"}
	set := make(map[string]struct{}, len(c.Slugs))
	for _, s := range c.Slugs {
		set[s] = struct{}{}
	}
	for _, w := range want {
		if _, ok := set[w]; !ok {
			t.Errorf("ai-native slugs missing %q", w)
		}
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
	records, err := c.Dataset.Parse(c.Dataset.Data)
	if err != nil {
		t.Fatalf("parse embedded eastern-roots: %v", err)
	}
	if len(records) < 50 {
		t.Errorf("eastern-roots slugs = %d, want a substantial list", len(records))
	}
	// The embedded slugs must be canonical (Match normalizes them, but a
	// non-canonical entry signals a bad edit to the committed file).
	for _, r := range records {
		if got := normalize.Slug(r.Name); got != r.Name {
			t.Errorf("non-canonical slug %q (normalizes to %q)", r.Name, got)
		}
	}
}

// errBoom is a sentinel for the adapter's error-propagation test.
var errBoom = errors.New("boom")

// TestHandListSlugsAreCompanySlugStable guards the curated lists against the slug rule they
// silently depend on.
//
// Editorial collections match by exact slug against the catalogue, and the catalogue now keys
// companies by normalize.CompanySlug. An entry written in a spelling that rule would never
// produce — one still carrying a corporate form — matches nothing, and nothing says so: the
// collection just quietly holds fewer companies than it names.
//
// The test is that each entry is a fixed point of the rule. eastern_roots.txt is included
// because it is the same kind of list, only larger and easier to add to carelessly.
func TestHandListSlugsAreCompanySlugStable(t *testing.T) {
	lists := map[string][]string{
		"AICompanySlugs": AICompanySlugs,
		"Mag7Slugs":      Mag7Slugs,
		"BigTechSlugs":   BigTechSlugs,
		"AINativeSlugs":  AINativeSlugs,
		"eastern_roots":  easternRootsSlugs(),
		"indian_roots":   indianRootsSlugs(),
	}
	var checked int
	for name, slugs := range lists {
		for _, slug := range slugs {
			checked++
			if got := normalize.CompanySlug(slug); got != slug {
				t.Errorf("%s: %q is not what the slug rule produces (%q) — the catalogue keys "+
					"companies by normalize.CompanySlug, so this entry matches nothing",
					name, slug, got)
			}
		}
	}
	// A detector that has stopped seeing the lists passes for the same reason a clean list
	// does; count the population so the two are told apart.
	if checked < 100 {
		t.Errorf("checked only %d hand-list slugs, expected the curated lists to be far larger "+
			"— has a list been renamed out from under this test?", checked)
	}
}
