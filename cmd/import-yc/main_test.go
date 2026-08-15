package main

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/ycdir"
)

// The directory's own industry words must not reach the column: they are a second
// vocabulary, and mixing two is the defect internal/industrytag exists to end.
func TestRecordToParamsCanonicalizesIndustries(t *testing.T) {
	got := recordToParams(ycdir.Record{
		Slug: "acme",
		Name: "Acme",
		// "Artificial Intelligence" merges into "ai", "B2B" is not an industry and
		// is dropped, and the two spellings of fintech collapse to one value.
		Industries: []string{"Artificial Intelligence", "B2B", "FinTech", "Fintech"},
	})

	want := []string{"ai", "fintech"}
	if !reflect.DeepEqual(got.Industries, want) {
		t.Errorf("Industries = %q, want %q", got.Industries, want)
	}
}

type fakeStore struct {
	exists    map[string]bool
	jobCounts map[string]int32
	calls     []db.UpsertYCCompanyParams
}

func (f *fakeStore) ListCompanySlugs(context.Context) ([]string, error) {
	var slugs []string
	for slug, ok := range f.exists {
		if ok {
			slugs = append(slugs, slug)
		}
	}
	return slugs, nil
}

func (f *fakeStore) CompanyJobCountBySlug(_ context.Context, slug string) (int32, error) {
	return f.jobCounts[slug], nil
}

func (f *fakeStore) UpsertYCCompany(_ context.Context, p db.UpsertYCCompanyParams) error {
	f.calls = append(f.calls, p)
	return nil
}

func TestLoad(t *testing.T) {
	entries := []ycdir.Entry{
		{Name: "Stripe", OneLiner: "Payments", Industry: "Fintech", Subindustry: "Fintech -> Payments", TeamSize: 8000, Batch: "Summer 2009", Status: "Public", Stage: "Growth", TopCompany: true},
		{Name: "New Co", Batch: "Winter 2024", Status: "Active"},
		{Name: "Meta", FormerNames: []string{"Facebook"}, Batch: "Summer 2005", Status: "Public"}, // current absent, former exists
		{Name: "Benchmark", TeamSize: 7, Batch: "Winter 2023", Status: "Active"},                  // homonym: tiny YC vs big non-YC company
		{Name: "   "}, // blank → skipped
	}
	// "meta" slug absent, but its former name "facebook" exists → enriches facebook.
	// "benchmark" exists with 1326 jobs but the YC entry has 7 employees → collision.
	fs := &fakeStore{
		exists:    map[string]bool{"stripe": true, "new-co": false, "meta": false, "facebook": true, "benchmark": true},
		jobCounts: map[string]int32{"stripe": 543, "facebook": 600, "benchmark": 1326},
	}

	stats, err := load(context.Background(), fs, entries)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if stats.matched != 2 || stats.inserted != 1 || stats.skipped != 1 || stats.collisions != 1 {
		t.Errorf("stats = %+v, want matched2 inserted1 skipped1 collisions1", stats)
	}
	if len(fs.calls) != 3 {
		t.Fatalf("UpsertYCCompany called %d times, want 3 (benchmark collision not written)", len(fs.calls))
	}
	for _, c := range fs.calls {
		if c.Slug == "benchmark" {
			t.Error("benchmark (homonym collision) was enriched, want skipped")
		}
	}

	// Meta resolves to the existing "facebook" via former name — no new row.
	meta := fs.calls[2]
	if meta.Slug != "facebook" {
		t.Errorf("meta resolved slug = %q, want facebook (former-name match)", meta.Slug)
	}

	stripe := fs.calls[0]
	if stripe.Slug != "stripe" || stripe.Tagline.String != "Payments" {
		t.Errorf("stripe slug/tagline = %q/%q", stripe.Slug, stripe.Tagline.String)
	}
	if !reflect.DeepEqual(stripe.YcBatch, []string{"Summer 2009"}) || !reflect.DeepEqual(stripe.YcStatus, []string{"Public"}) {
		t.Errorf("stripe yc facets = %v/%v", stripe.YcBatch, stripe.YcStatus)
	}
	if !reflect.DeepEqual(stripe.YcStage, []string{"Growth"}) || !reflect.DeepEqual(stripe.YcFlags, []string{"top_company"}) {
		t.Errorf("stripe yc_stage/yc_flags = %v/%v", stripe.YcStage, stripe.YcFlags)
	}
	if stripe.Industries == nil {
		t.Error("industries is nil, want non-nil for the NOT NULL column")
	}
	// subindustry is forwarded as the clean YC leaf (nullable text).
	if !stripe.Subindustry.Valid || stripe.Subindustry.String != "Payments" {
		t.Errorf("stripe subindustry = %+v, want valid %q", stripe.Subindustry, "Payments")
	}
	var info map[string]any
	if err := json.Unmarshal(stripe.CompanyInfo, &info); err != nil {
		t.Fatalf("company_info not JSON: %v", err)
	}

	// New Co: no batch text beyond the single value, non-nil arrays.
	newco := fs.calls[1]
	if newco.Slug != "new-co" || !reflect.DeepEqual(newco.YcStatus, []string{"Active"}) {
		t.Errorf("newco slug/status = %q/%v", newco.Slug, newco.YcStatus)
	}
	if newco.YcBatch == nil || newco.Industries == nil {
		t.Error("newco arrays must be non-nil (NOT NULL columns)")
	}
	if newco.Subindustry.Valid {
		t.Errorf("newco subindustry = %+v, want NULL (no subindustry given)", newco.Subindustry)
	}
}

// TestResolveTarget exercises the in-memory slug resolution directly (no store):
// current-name slug wins first, a former-name slug wins if the current one is
// absent, and an entry with no match at all resolves to its own current-name
// slug as a new reference row.
func TestResolveTarget(t *testing.T) {
	existing := map[string]bool{"stripe": true, "facebook": true}

	if slug, matched := resolveTarget(existing, ycdir.Record{Slug: "stripe"}); slug != "stripe" || !matched {
		t.Errorf("resolveTarget(stripe) = %q, %v; want stripe, true", slug, matched)
	}
	if slug, matched := resolveTarget(existing, ycdir.Record{Slug: "meta", FormerSlugs: []string{"facebook"}}); slug != "facebook" || !matched {
		t.Errorf("resolveTarget(meta, former facebook) = %q, %v; want facebook, true", slug, matched)
	}
	if slug, matched := resolveTarget(existing, ycdir.Record{Slug: "new-co"}); slug != "new-co" || matched {
		t.Errorf("resolveTarget(new-co) = %q, %v; want new-co, false", slug, matched)
	}
}
