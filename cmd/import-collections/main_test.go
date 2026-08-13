package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"sort"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/collections"
	"github.com/strelov1/freehire/internal/db"
)

// plan matches each collection's candidates against existing companies and emits a
// write only for companies whose managed-tag set actually changes, preserving any
// unmanaged tags. `google` is the known bigtech member and `acme-startup` a yc-only
// match, so the test does not depend on the exact hand list beyond google being in it.
func TestPlan(t *testing.T) {
	rows := []db.ListCompanyCollectionsRow{
		{Slug: "google", Collections: []string{}},               // bigtech (hand list)
		{Slug: "acme-startup", Collections: []string{"custom"}}, // yc match, unmanaged tag preserved
		{Slug: "nytimes", Collections: []string{}},              // matches nothing → no write
		{Slug: "oldyc", Collections: []string{"yc"}},            // no longer matched → yc dropped
	}
	resolved := map[string][]collections.Record{
		"yc":      slugRecords([]string{"Acme Startup", "Unknown Co"}), // "Acme Startup" → acme-startup; "Unknown Co" → none
		"bigtech": slugRecords(collections.BigTechSlugs),
		"unicorn": nil,
	}

	got := plan(rows, resolved)

	writeBySlug := map[string][]string{}
	for _, w := range got.writes {
		writeBySlug[w.Slug] = w.Collections
	}

	if c := writeBySlug["google"]; !reflect.DeepEqual(c, []string{"bigtech"}) {
		t.Errorf("google write = %#v, want [bigtech]", c)
	}
	if c := writeBySlug["acme-startup"]; !reflect.DeepEqual(c, []string{"custom", "yc"}) {
		t.Errorf("acme-startup write = %#v, want [custom yc]", c)
	}
	if c, ok := writeBySlug["oldyc"]; !ok || len(c) != 0 {
		t.Errorf("oldyc write = %#v (ok=%v), want [] (yc dropped)", c, ok)
	}
	if _, ok := writeBySlug["nytimes"]; ok {
		t.Errorf("nytimes should not be rewritten (no managed match), got %v", writeBySlug["nytimes"])
	}

	if s := got.stats["yc"]; s.Matched != 1 || s.Unmatched != 1 {
		t.Errorf("yc stats = %+v, want {matched:1 unmatched:1}", s)
	}
	if s := got.stats["bigtech"]; s.Matched != 1 { // only google (of the rows) is in the hand list
		t.Errorf("bigtech matched = %d, want 1", s.Matched)
	}
}

// A company keeps an unmanaged tag through reconciliation even when it gains a
// managed one.
func TestPlan_PreservesUnmanagedTag(t *testing.T) {
	rows := []db.ListCompanyCollectionsRow{
		{Slug: "google", Collections: []string{"custom"}},
	}
	// google gains bigtech from the hand list; no yc/unicorn candidates.
	got := plan(rows, map[string][]collections.Record{"bigtech": slugRecords(collections.BigTechSlugs)})
	if len(got.writes) != 1 {
		t.Fatalf("writes = %d, want 1", len(got.writes))
	}
	c := got.writes[0].Collections
	sort.Strings(c)
	if !reflect.DeepEqual(c, []string{"bigtech", "custom"}) {
		t.Errorf("collections = %#v, want [bigtech custom]", c)
	}
}

// credentialRows is a catalogue shaped for the gate tests: a UK-headquartered
// single-token company, a multinational that merely hires in the UK, and a
// two-token British company.
func credentialRows() []db.ListCompanyCollectionsRow {
	return []db.ListCompanyCollectionsRow{
		{Slug: "monzo", Countries: []string{"GB"}, HqCountry: pgtype.Text{String: "GB", Valid: true}},
		{Slug: "apple", Countries: []string{"GB", "US"}, HqCountry: pgtype.Text{String: "US", Valid: true}},
		{Slug: "acme-robotics", Countries: []string{"GB"}},
	}
}

func tagsFor(p planResult, slug string) []string {
	for _, w := range p.writes {
		if w.Slug == slug {
			return w.Collections
		}
	}
	return nil
}

func TestPlan_CredentialMatchesThroughTheLegalSuffix(t *testing.T) {
	// "ACME ROBOTICS LIMITED" must reach acme-robotics, which plain normalize.Slug
	// would never do.
	resolved := map[string][]collections.Record{
		"uk-skilled-worker-sponsor": {
			{Name: "ACME ROBOTICS LIMITED", Meta: map[string]string{"town": "London", "route": "Skilled Worker"}},
		},
	}
	got := plan(credentialRows(), resolved)
	if c := tagsFor(got, "acme-robotics"); !slices.Contains(c, "uk-skilled-worker-sponsor") {
		t.Errorf("acme-robotics tags = %v, want the UK credential", c)
	}
}

func TestPlan_CredentialAppliesTheSingleTokenHeadquartersRule(t *testing.T) {
	resolved := map[string][]collections.Record{
		"uk-skilled-worker-sponsor": {
			{Name: "MONZO LTD", Meta: map[string]string{"town": "London", "route": "Skilled Worker"}},
			{Name: "APPLE LTD", Meta: map[string]string{"town": "Liverpool", "route": "Skilled Worker"}},
		},
	}
	got := plan(credentialRows(), resolved)
	if c := tagsFor(got, "monzo"); !slices.Contains(c, "uk-skilled-worker-sponsor") {
		t.Errorf("monzo tags = %v, want the UK credential", c)
	}
	if c := tagsFor(got, "apple"); slices.Contains(c, "uk-skilled-worker-sponsor") {
		t.Errorf("apple earned a credential from an unrelated APPLE LTD: %v", c)
	}
}

func TestPlan_CredentialNeedsAWorkRoute(t *testing.T) {
	resolved := map[string][]collections.Record{
		"uk-skilled-worker-sponsor": {
			{Name: "ACME ROBOTICS LTD", Meta: map[string]string{"town": "London", "route": "Temporary Worker - Seasonal Worker"}},
		},
	}
	got := plan(credentialRows(), resolved)
	if c := tagsFor(got, "acme-robotics"); slices.Contains(c, "uk-skilled-worker-sponsor") {
		t.Errorf("a seasonal-worker licence earned the skilled-worker credential: %v", c)
	}
}

func TestPlan_CredentialTakesTheWorkRouteAmongSeveral(t *testing.T) {
	// One organisation, several routes: the work route must win even when a
	// temporary one is listed first.
	resolved := map[string][]collections.Record{
		"uk-skilled-worker-sponsor": {
			{Name: "ACME ROBOTICS LTD", Meta: map[string]string{"town": "London", "route": "Temporary Worker - Creative Worker"}},
			{Name: "ACME ROBOTICS LTD", Meta: map[string]string{"town": "London", "route": "Skilled Worker"}},
		},
	}
	got := plan(credentialRows(), resolved)
	if c := tagsFor(got, "acme-robotics"); !slices.Contains(c, "uk-skilled-worker-sponsor") {
		t.Errorf("acme-robotics tags = %v, want the UK credential", c)
	}
}

func TestPlan_CredentialDropsAnAmbiguousName(t *testing.T) {
	// Two organisations in different towns share a normalized name: it identifies
	// neither, so nobody is tagged.
	resolved := map[string][]collections.Record{
		"uk-skilled-worker-sponsor": {
			{Name: "ACME ROBOTICS LTD", Meta: map[string]string{"town": "London", "route": "Skilled Worker"}},
			{Name: "Acme Robotics Limited", Meta: map[string]string{"town": "Leeds", "route": "Skilled Worker"}},
		},
	}
	got := plan(credentialRows(), resolved)
	if c := tagsFor(got, "acme-robotics"); slices.Contains(c, "uk-skilled-worker-sponsor") {
		t.Errorf("an ambiguous register name was granted: %v", c)
	}
	if s := got.stats["uk-skilled-worker-sponsor"]; s.Ambiguous == 0 {
		t.Error("the ambiguity guard did not report what it dropped")
	}
}

func TestPlan_EditorialMatchingIsUnchangedByTheSuffixStrip(t *testing.T) {
	// Editorial collections keep matching on normalize.Slug. A dataset name carrying
	// a legal form must NOT be stripped into a different company's slug.
	rows := []db.ListCompanyCollectionsRow{{Slug: "acme-robotics-limited"}}
	got := plan(rows, map[string][]collections.Record{"yc": slugRecords([]string{"Acme Robotics Limited"})})
	if c := tagsFor(got, "acme-robotics-limited"); !slices.Contains(c, "yc") {
		t.Errorf("editorial match changed shape: %v", c)
	}
}

func TestPlan_CountsGatedOutCandidates(t *testing.T) {
	resolved := map[string][]collections.Record{
		"uk-skilled-worker-sponsor": {
			{Name: "APPLE LTD", Meta: map[string]string{"town": "Liverpool", "route": "Skilled Worker"}},
		},
	}
	got := plan(credentialRows(), resolved)
	if s := got.stats["uk-skilled-worker-sponsor"]; s.Gated != 1 {
		t.Errorf("gated = %d, want 1 (apple matched by name but failed the gate)", s.Gated)
	}
}

func TestResolveOne_TreatsAZeroRecordParseAsFailure(t *testing.T) {
	// A source that has changed shape parses to zero rows just as convincingly as a
	// genuinely empty register. Letting it through would reconcile the tag off every
	// company, so it must fail like a fetch failure does.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()

	c := collections.Collection{
		Slug:    "x",
		Kind:    collections.KindEditorial,
		Dataset: &collections.Dataset{URL: srv.URL, Parse: func([]byte) ([]collections.Record, error) { return nil, nil }},
	}
	if _, err := resolveOne(context.Background(), srv.Client(), c); err == nil {
		t.Error("resolveOne accepted a dataset that parsed to no records")
	}
}

func TestResolveOne_FetchesFromTheDatasetResolver(t *testing.T) {
	var fetched string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetched = r.URL.Path
		_, _ = w.Write([]byte("payload"))
	}))
	defer srv.Close()

	c := collections.Collection{
		Slug: "x",
		Kind: collections.KindEditorial,
		Dataset: &collections.Dataset{
			ResolveURL: func(context.Context, *http.Client) (string, error) { return srv.URL + "/resolved.csv", nil },
			Parse:      func([]byte) ([]collections.Record, error) { return []collections.Record{{Name: "Acme"}}, nil },
		},
	}
	got, err := resolveOne(context.Background(), srv.Client(), c)
	if err != nil {
		t.Fatalf("resolveOne: %v", err)
	}
	if fetched != "/resolved.csv" {
		t.Errorf("fetched %q, want the resolver's URL", fetched)
	}
	if len(got) != 1 {
		t.Errorf("records = %+v, want one", got)
	}
}

func TestResolveOne_UsesASelfFetchingDatasetsRecords(t *testing.T) {
	// A paginated directory cannot be expressed as one URL, so it fetches itself. The
	// worker must route it the same as any other dataset — including through the
	// zero-record guard below.
	c := collections.Collection{
		Slug: "a16z-portfolio",
		Kind: collections.KindBacker,
		Dataset: &collections.Dataset{
			Records: func(context.Context, *http.Client) ([]collections.Record, error) {
				return []collections.Record{{Name: "Anduril Industries"}}, nil
			},
		},
	}
	got, err := resolveOne(context.Background(), http.DefaultClient, c)
	if err != nil {
		t.Fatalf("resolveOne: %v", err)
	}
	if len(got) != 1 || got[0].Name != "Anduril Industries" {
		t.Errorf("records = %+v, want the directory's one record", got)
	}
}

func TestResolveOne_ASelfFetchingDatasetThatReadsNothingIsAFailure(t *testing.T) {
	// Same reasoning as every other source: zero records parses just as cleanly as a
	// genuinely empty directory would, and would reconcile the tag off every company.
	c := collections.Collection{
		Slug: "a16z-speedrun",
		Kind: collections.KindBacker,
		Dataset: &collections.Dataset{
			Records: func(context.Context, *http.Client) ([]collections.Record, error) { return nil, nil },
		},
	}
	if _, err := resolveOne(context.Background(), http.DefaultClient, c); err == nil {
		t.Error("an empty directory read was accepted as an empty membership")
	}
}

func TestResolveOne_PropagatesASelfFetchingDatasetFailure(t *testing.T) {
	c := collections.Collection{
		Slug: "a16z-portfolio",
		Kind: collections.KindBacker,
		Dataset: &collections.Dataset{
			Records: func(context.Context, *http.Client) ([]collections.Record, error) {
				return nil, errors.New("page 2: status 502")
			},
		},
	}
	if _, err := resolveOne(context.Background(), http.DefaultClient, c); err == nil {
		t.Error("resolveOne swallowed a directory fetch failure")
	}
}

func TestResolveOne_PropagatesAResolverFailure(t *testing.T) {
	c := collections.Collection{
		Slug: "x",
		Kind: collections.KindCredential,
		Dataset: &collections.Dataset{
			ResolveURL: func(context.Context, *http.Client) (string, error) { return "", errors.New("no csv link") },
			Parse:      func([]byte) ([]collections.Record, error) { return nil, nil },
		},
	}
	if _, err := resolveOne(context.Background(), http.DefaultClient, c); err == nil {
		t.Error("resolveOne swallowed a resolver failure")
	}
}

func TestPlan_ReportsHQCountryCoverage(t *testing.T) {
	// The dry run is read for this number: the single-token credential rule is only
	// as good as hq_country's coverage.
	got := plan(credentialRows(), nil)
	if got.withHQCountry != 2 {
		t.Errorf("withHQCountry = %d, want 2 of 3 rows", got.withHQCountry)
	}
}

func TestPlan_FlagsACollapseAsUnsafe(t *testing.T) {
	// A source that parses fine but matches nobody is the failure the fetch and
	// empty-parse aborts cannot see: GOV.UK renaming a route value leaves 142k rows
	// intact, gates every one of them out, and Reconcile then strips the credential
	// from every company that held it. Exit 0, no error, tag gone.
	rows := []db.ListCompanyCollectionsRow{
		{Slug: "monzo", Collections: []string{"uk-skilled-worker-sponsor"}, Countries: []string{"GB"}, HqCountry: pgtype.Text{String: "GB", Valid: true}},
		{Slug: "acme-robotics", Collections: []string{"uk-skilled-worker-sponsor"}, Countries: []string{"GB"}},
	}
	// The register still parses, but every row is on a route the gate refuses.
	resolved := map[string][]collections.Record{
		"uk-skilled-worker-sponsor": {
			{Name: "MONZO LTD", Meta: map[string]string{"town": "London", "route": "WORKER: SKILLED"}},
			{Name: "ACME ROBOTICS LTD", Meta: map[string]string{"town": "London", "route": "WORKER: SKILLED"}},
		},
	}
	got := plan(rows, resolved)
	if len(got.collapsed) == 0 {
		t.Fatal("a tag losing every one of its holders was not flagged")
	}
	if c := got.collapsed[0]; c.slug != "uk-skilled-worker-sponsor" || c.had != 2 || c.keeps != 0 {
		t.Errorf("collapse = %+v, want uk-skilled-worker-sponsor had=2 keeps=0", c)
	}
}

func TestPlan_DoesNotFlagAHealthyRun(t *testing.T) {
	rows := []db.ListCompanyCollectionsRow{
		{Slug: "monzo", Collections: []string{"uk-skilled-worker-sponsor"}, Countries: []string{"GB"}, HqCountry: pgtype.Text{String: "GB", Valid: true}},
		{Slug: "acme-robotics", Collections: []string{"uk-skilled-worker-sponsor"}, Countries: []string{"GB"}},
	}
	resolved := map[string][]collections.Record{
		"uk-skilled-worker-sponsor": {
			{Name: "MONZO LTD", Meta: map[string]string{"town": "London", "route": "Skilled Worker"}},
			{Name: "ACME ROBOTICS LTD", Meta: map[string]string{"town": "London", "route": "Skilled Worker"}},
		},
	}
	if got := plan(rows, resolved); len(got.collapsed) != 0 {
		t.Errorf("a healthy run was flagged as a collapse: %+v", got.collapsed)
	}
}

func TestPlan_DoesNotFlagATagNobodyHeld(t *testing.T) {
	// First run of a new credential: nothing held it, so losing nothing is not a
	// collapse. Without this the guard would block every new collection's first run.
	rows := []db.ListCompanyCollectionsRow{{Slug: "monzo", Countries: []string{"US"}}}
	got := plan(rows, map[string][]collections.Record{
		"uk-skilled-worker-sponsor": {{Name: "SOMEONE ELSE LTD", Meta: map[string]string{"route": "Skilled Worker"}}},
	})
	if len(got.collapsed) != 0 {
		t.Errorf("a tag nobody held was flagged: %+v", got.collapsed)
	}
}

func TestPlan_FlagsAMajorityLossNotJustATotalOne(t *testing.T) {
	// A truncated upstream snapshot is a partial wipe with no error either.
	rows := make([]db.ListCompanyCollectionsRow, 0, 10)
	for _, s := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"} {
		rows = append(rows, db.ListCompanyCollectionsRow{
			Slug: s, Collections: []string{"uk-skilled-worker-sponsor"},
			Countries: []string{"GB"}, HqCountry: pgtype.Text{String: "GB", Valid: true},
		})
	}
	// Only two of the ten survive the truncated register.
	resolved := map[string][]collections.Record{"uk-skilled-worker-sponsor": {
		{Name: "a", Meta: map[string]string{"town": "London", "route": "Skilled Worker"}},
		{Name: "b", Meta: map[string]string{"town": "Leeds", "route": "Skilled Worker"}},
	}}
	if got := plan(rows, resolved); len(got.collapsed) == 0 {
		t.Error("losing 8 of 10 holders was not flagged")
	}
}

func TestClientFor_UsesDirectWhenNoProxyConfigured(t *testing.T) {
	direct := &http.Client{}
	if got := clientFor("us-h1b-sponsor", direct, nil); got != direct {
		t.Error("clientFor did not fall back to direct when no proxy client was built")
	}
}

func TestClientFor_UsesProxiedForAnAllowlistedSlug(t *testing.T) {
	direct, proxied := &http.Client{}, &http.Client{}
	if got := clientFor("us-h1b-sponsor", direct, proxied); got != proxied {
		t.Error("clientFor did not route the allowlisted slug through the proxy")
	}
}

func TestClientFor_UsesDirectForANonAllowlistedSlug(t *testing.T) {
	// uk-skilled-worker-sponsor has never needed the proxy; a proxy configured for
	// the US register must not silently reroute an unrelated collection's egress.
	direct, proxied := &http.Client{}, &http.Client{}
	if got := clientFor("uk-skilled-worker-sponsor", direct, proxied); got != direct {
		t.Error("clientFor rerouted a non-allowlisted slug through the proxy")
	}
}

func TestProxiedClient_ReturnsNilWhenSOURCES_PROXY_URLIsUnset(t *testing.T) {
	t.Setenv("SOURCES_PROXY_URL", "")
	got, err := proxiedClient(fetchTimeout)
	if err != nil {
		t.Fatalf("proxiedClient: %v", err)
	}
	if got != nil {
		t.Error("proxiedClient returned a non-nil client with SOURCES_PROXY_URL unset")
	}
}

func TestProxiedClient_BuildsAClientWhenSOURCES_PROXY_URLIsSet(t *testing.T) {
	t.Setenv("SOURCES_PROXY_URL", "http://user:pass@proxy.example:8080")
	got, err := proxiedClient(fetchTimeout)
	if err != nil {
		t.Fatalf("proxiedClient: %v", err)
	}
	if got == nil {
		t.Error("proxiedClient returned nil with SOURCES_PROXY_URL set")
	}
}

func TestProxiedClient_ErrorsOnAnUnparseableSOURCES_PROXY_URL(t *testing.T) {
	t.Setenv("SOURCES_PROXY_URL", "not a url")
	if _, err := proxiedClient(fetchTimeout); err == nil {
		t.Error("proxiedClient accepted an invalid SOURCES_PROXY_URL")
	}
}
