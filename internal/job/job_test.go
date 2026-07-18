package job_test

import (
	"reflect"
	"testing"

	"github.com/strelov1/freehire/internal/job"
	"github.com/strelov1/freehire/internal/normalize"
)

// New is the single construction door: it runs the deterministic derivation
// internally, so a constructed Job always carries facets consistent with its
// source fields. A caller never touches the facet fields.
func TestNew_DerivesFacetsFromDraft(t *testing.T) {
	j, err := job.New(job.Draft{
		Source:      "manual",
		ExternalID:  "https://acme.example/jobs/1",
		Title:       "Senior Go Developer",
		Company:     "Acme",
		Location:    "Remote - Germany",
		Description: "We use Golang and PostgreSQL.",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	f := j.Fields()

	// Identity is preserved verbatim.
	if f.Source != "manual" || f.ExternalID != "https://acme.example/jobs/1" {
		t.Errorf("identity = %q/%q", f.Source, f.ExternalID)
	}
	// Slugs are minted deterministically from the identity.
	wantSlug := normalize.JobSlug("Senior Go Developer", "Acme", "manual", "https://acme.example/jobs/1")
	if f.PublicSlug != wantSlug {
		t.Errorf("PublicSlug = %q, want %q", f.PublicSlug, wantSlug)
	}
	if f.CompanySlug != normalize.Slug("Acme") {
		t.Errorf("CompanySlug = %q, want %q", f.CompanySlug, normalize.Slug("Acme"))
	}
	// Facets are derived from the dictionaries — the caller supplied none.
	if len(f.Countries) == 0 || f.Countries[0] != "de" {
		t.Errorf("Countries = %v, want [de ...]", f.Countries)
	}
	if !reflect.DeepEqual(f.Skills, []string{"go", "postgresql"}) {
		t.Errorf("Skills = %v, want [go postgresql]", f.Skills)
	}
	if f.WorkMode != "remote" {
		t.Errorf("WorkMode = %q, want remote", f.WorkMode)
	}
}

// A freshly constructed Job is open and unenriched: no lifecycle or enrichment
// state until the write/enrich paths set it.
func TestNew_FreshJobIsOpenAndUnenriched(t *testing.T) {
	j, err := job.New(job.Draft{Source: "manual", ExternalID: "1", Title: "Engineer", Company: "Acme"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !j.IsOpen() {
		t.Error("fresh job should be open")
	}
	if f := j.Fields(); f.EnrichmentVersion != 0 {
		t.Errorf("fresh job EnrichmentVersion = %d, want 0", f.EnrichmentVersion)
	}
}

// Facets depend on content, never on which write path constructed the job: a
// Telegram-extracted posting and a board-ingested posting with the same title,
// description, and location resolve identical dictionary facets (only the slugs,
// minted from identity, differ). This is the deterministic-facets guarantee that a
// single construction door delivers — the tg-extract inline-derive divergence is
// now unrepresentable.
func TestNew_FacetsIndependentOfWritePath(t *testing.T) {
	content := job.Draft{
		Title:       "Senior Go Developer",
		Company:     "Acme",
		Location:    "Remote - Germany",
		Description: "We use Golang and Kubernetes.",
	}
	tg := content
	tg.Source, tg.ExternalID = "telegram", "chan/1/0"
	board := content
	board.Source, board.ExternalID = "greenhouse", "acme:42"

	tj, err := job.New(tg)
	if err != nil {
		t.Fatalf("New(tg): %v", err)
	}
	bj, err := job.New(board)
	if err != nil {
		t.Fatalf("New(board): %v", err)
	}
	tf, bf := tj.Fields(), bj.Fields()

	if !reflect.DeepEqual(tf.Countries, bf.Countries) || !reflect.DeepEqual(tf.Regions, bf.Regions) ||
		!reflect.DeepEqual(tf.Cities, bf.Cities) || tf.WorkMode != bf.WorkMode ||
		!reflect.DeepEqual(tf.Skills, bf.Skills) || tf.Seniority != bf.Seniority || tf.Category != bf.Category {
		t.Errorf("facets diverged between write paths:\n tg    = %+v\n board = %+v", tf, bf)
	}
	// Slugs are minted from identity, so they legitimately differ.
	if tf.PublicSlug == bf.PublicSlug {
		t.Errorf("public slugs should differ by identity, both = %q", tf.PublicSlug)
	}
}

// The factory rejects an identity-less draft: source and external id together are
// the dedup key, and a title-less posting is not a job.
func TestNew_RejectsMissingIdentity(t *testing.T) {
	cases := map[string]job.Draft{
		"no source":      {ExternalID: "1", Title: "Engineer"},
		"no external id": {Source: "manual", Title: "Engineer"},
		"no title":       {Source: "manual", ExternalID: "1"},
	}
	for name, d := range cases {
		if _, err := job.New(d); err == nil {
			t.Errorf("%s: expected error, got nil", name)
		}
	}
}

// An explicit region/city on the draft is authoritative: it overrides what the location
// dictionary would derive, while an unsupplied facet still derives (see jobderive).
func TestNew_ExplicitRegionCityOverrideDerivation(t *testing.T) {
	j, err := job.New(job.Draft{
		Source:      "manual",
		ExternalID:  "https://acme.example/jobs/1",
		Title:       "Senior Go Developer",
		Company:     "Acme",
		Location:    "Remote - Germany",
		Description: "We use Golang.",
		Regions:     []string{"north_america"},
		Cities:      []string{"Austin"},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	f := j.Fields()
	if !reflect.DeepEqual(f.Regions, []string{"north_america"}) {
		t.Errorf("Regions = %v, want [north_america] (explicit wins)", f.Regions)
	}
	if !reflect.DeepEqual(f.Cities, []string{"Austin"}) {
		t.Errorf("Cities = %v, want [Austin] (explicit wins)", f.Cities)
	}
}

// A manual salary supplied on the draft is carried verbatim onto the Job as a base
// field — it is authoritative, never derived — and is absent by default.
func TestNew_CarriesManualSalary(t *testing.T) {
	min, max := 90000, 120000
	j, err := job.New(job.Draft{
		Source:       "manual",
		ExternalID:   "https://acme.example/jobs/1",
		Title:        "Senior Go Developer",
		Company:      "Acme",
		ManualSalary: &job.Salary{Min: &min, Max: &max, Currency: "EUR", Period: "year"},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	f := j.Fields()
	if f.ManualSalary == nil {
		t.Fatal("ManualSalary = nil, want a value")
	}
	if f.ManualSalary.Min == nil || *f.ManualSalary.Min != 90000 || f.ManualSalary.Max == nil || *f.ManualSalary.Max != 120000 {
		t.Errorf("ManualSalary range = %v/%v, want 90000/120000", f.ManualSalary.Min, f.ManualSalary.Max)
	}
	if f.ManualSalary.Currency != "EUR" || f.ManualSalary.Period != "year" {
		t.Errorf("ManualSalary currency/period = %q/%q, want EUR/year", f.ManualSalary.Currency, f.ManualSalary.Period)
	}
}

func TestNew_NoManualSalaryByDefault(t *testing.T) {
	j, err := job.New(job.Draft{Source: "manual", ExternalID: "u", Title: "Go Dev", Company: "Acme"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if j.Fields().ManualSalary != nil {
		t.Errorf("ManualSalary = %v, want nil (none supplied)", j.Fields().ManualSalary)
	}
}
