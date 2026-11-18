package pipeline

import (
	"reflect"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

func TestNormalizeJobParsesGeographyFromLocation(t *testing.T) {
	e := sources.CompanyEntry{Company: "Acme", Provider: "greenhouse", Board: "acme"}

	geoJob, err := normalizeJob(e, sources.Job{ExternalID: "1", Title: "Dev", Company: "Acme", Location: "Remote - Germany"}, nil, false)
	if err != nil {
		t.Fatalf("normalizeJob: %v", err)
	}
	geo := geoJob.Fields()
	if !reflect.DeepEqual(geo.Countries, []string{"de"}) || !reflect.DeepEqual(geo.Regions, []string{"eu"}) {
		t.Errorf("geography = %v/%v, want [de]/[eu]", geo.Countries, geo.Regions)
	}

	// A bare "Remote" resolves no country and no region: it states nothing about where
	// it is open to, so it stays in the "region not specified" bucket, not global (its
	// remoteness stays on WorkMode; see location.Parse).
	bareJob, err := normalizeJob(e, sources.Job{ExternalID: "2", Title: "Dev", Company: "Acme", Location: "Remote"}, nil, false)
	if err != nil {
		t.Fatalf("normalizeJob: %v", err)
	}
	bare := bareJob.Fields()
	if len(bare.Countries) != 0 || len(bare.Regions) != 0 {
		t.Errorf("bare remote geography = %v/%v, want []/[]", bare.Countries, bare.Regions)
	}
}

func TestNormalizeJobPrefersAdapterWorkModeOverParser(t *testing.T) {
	e := sources.CompanyEntry{Company: "Acme", Provider: "greenhouse", Board: "acme"}

	// The adapter states hybrid structurally; the location text would parse as
	// remote. The structured signal wins.
	structured, err := normalizeJob(e, sources.Job{ExternalID: "1", Title: "Dev", Company: "Acme", Location: "Remote", WorkMode: "hybrid"}, nil, false)
	if err != nil {
		t.Fatalf("normalizeJob: %v", err)
	}
	if structured.Fields().WorkMode != "hybrid" {
		t.Errorf("WorkMode = %q, want hybrid (adapter structured wins over parser)", structured.Fields().WorkMode)
	}

	// No structured signal: the parser fills from the location text.
	parsed, err := normalizeJob(e, sources.Job{ExternalID: "2", Title: "Dev", Company: "Acme", Location: "Remote"}, nil, false)
	if err != nil {
		t.Fatalf("normalizeJob: %v", err)
	}
	if parsed.Fields().WorkMode != "remote" {
		t.Errorf("WorkMode = %q, want remote (parser fallback)", parsed.Fields().WorkMode)
	}
}

// TestNormalizeJobPassesIsTechHint pins that an adapter's structured
// "confirmed technical" signal (see issue #2601 — Profession's dedicated IT
// boards) reaches jobderive and therefore is_tech, even for a title neither
// the tech nor the non-tech dictionary resolves.
func TestNormalizeJobPassesIsTechHint(t *testing.T) {
	e := sources.CompanyEntry{Company: "Acme", Provider: "profession", Board: "itdev"}

	hinted, err := normalizeJob(e, sources.Job{ExternalID: "1", Title: "Yard Coordinator", Company: "Acme", IsTechHint: true}, nil, false)
	if err != nil {
		t.Fatalf("normalizeJob: %v", err)
	}
	if got := hinted.Fields().IsTech; got == nil || !*got {
		t.Errorf("IsTech = %v, want true (adapter's structured IsTechHint wins)", got)
	}

	unhinted, err := normalizeJob(e, sources.Job{ExternalID: "2", Title: "Yard Coordinator", Company: "Acme"}, nil, false)
	if err != nil {
		t.Fatalf("normalizeJob: %v", err)
	}
	if got := unhinted.Fields().IsTech; got != nil {
		t.Errorf("IsTech = %v, want nil (no hint, dictionary resolves neither)", *got)
	}
}

func TestNormalizeJobDerivesSkills(t *testing.T) {
	dj, err := normalizeJob(
		sources.CompanyEntry{Provider: "greenhouse", Board: "acme"},
		sources.Job{
			Title: "Backend Engineer", Company: "Acme", ExternalID: "1",
			Description: "<p>Build services in Golang with PostgreSQL and Kubernetes.</p>",
		},
		nil, false,
	)
	if err != nil {
		t.Fatalf("normalizeJob: %v", err)
	}
	want := []string{"go", "kubernetes", "postgresql"}
	if got := dj.Fields().Skills; !reflect.DeepEqual(got, want) {
		t.Fatalf("Skills = %#v, want %#v", got, want)
	}
}

func TestNormalizeJobDerivesClassification(t *testing.T) {
	dj, err := normalizeJob(
		sources.CompanyEntry{Provider: "greenhouse", Board: "acme", Company: "Acme"},
		sources.Job{ExternalID: "1", Title: "Senior Backend Engineer", Description: "x"},
		nil, false,
	)
	if err != nil {
		t.Fatalf("normalizeJob: %v", err)
	}
	f := dj.Fields()
	if f.Seniority != "senior" {
		t.Errorf("Seniority = %q, want senior", f.Seniority)
	}
	if f.Category != "backend" {
		t.Errorf("Category = %q, want backend", f.Category)
	}
}

// A neutral display label shared by many boards must not become their company key.
func TestNormalizeJobCompanyKeyKeepsSlugPerBoard(t *testing.T) {
	e := sources.CompanyEntry{Provider: "paycor", Board: "b"}
	a, err := normalizeJob(e, sources.Job{ExternalID: "1", Title: "Dev", Company: "Employer via Paycor", CompanyKey: "boardaaa"}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	b, err := normalizeJob(e, sources.Job{ExternalID: "1", Title: "Dev", Company: "Employer via Paycor", CompanyKey: "boardbbb"}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if a.Fields().Company != "Employer via Paycor" || a.Fields().CompanySlug == b.Fields().CompanySlug ||
		a.Fields().CompanySlug != "boardaaa" {
		t.Errorf("company %q slugs %q / %q", a.Fields().Company, a.Fields().CompanySlug, b.Fields().CompanySlug)
	}
}
