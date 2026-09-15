package sources

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestJoppyProvider(t *testing.T) {
	if got := NewJoppy(nil).Provider(); got != "joppy" {
		t.Errorf("Provider() = %q, want %q", got, "joppy")
	}
}

func TestJoppyIsBoardlessAggregator(t *testing.T) {
	src := NewJoppy(nil)
	if _, ok := src.(boardless); !ok {
		t.Error("joppy should implement boardless")
	}
	if _, ok := src.(aggregator); !ok {
		t.Error("joppy should implement aggregator")
	}
}

func TestJoppyConfigValidateAcceptsEmptyBoard(t *testing.T) {
	cfg := Config{Sources: []CompanyEntry{{Provider: "joppy", Company: "Joppy"}}}
	registry := map[string]Source{"joppy": NewJoppy(nil)}
	if err := cfg.Validate(registry); err != nil {
		t.Errorf("Validate() = %v, want nil for boardless entry with empty board", err)
	}
}

// joppySitemapXML builds a sitemap.directory.xml fixture: one <url> per given loc.
func joppySitemapXML(locs ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><urlset>`)
	for _, l := range locs {
		b.WriteString(`<url><loc>` + l + `</loc><lastmod>2026-09-10T08:00:00.000Z</lastmod></url>`)
	}
	b.WriteString(`</urlset>`)
	return b.String()
}

// joppyCompanyPage builds a company page's __NEXT_DATA__ fixture embedding the given raw job
// JSON objects (already-serialized) under company.jobs.
func joppyCompanyPage(name, slug string, jobsJSON ...string) string {
	return `<html><body><script id="__NEXT_DATA__" type="application/json">` +
		`{"props":{"pageProps":{"company":{"name":"` + name + `","slug":"` + slug + `",` +
		`"jobs":[` + strings.Join(jobsJSON, ",") + `]}}}}` +
		`</script></body></html>`
}

func TestJoppyCompanySlugsFromSitemap(t *testing.T) {
	xml := joppySitemapXML(
		"https://www.joppy.me/companies/seidor",
		"https://www.joppy.me/companies/seidor/7941c018-b7c5-42f2-a92a-bced77649fc4",
		"https://www.joppy.me/companies/seidor/14d16b99-0cdb-4813-8644-dd6f3ccf0d64",
		"https://www.joppy.me/companies/n26",
	)
	fake := (&routedHTTP{}).route("sitemap.directory.xml", xml)
	doc, err := getSitemap(context.Background(), fake, joppySitemapURL)
	if err != nil {
		t.Fatalf("getSitemap() error = %v", err)
	}
	got := joppyCompanySlugs(doc)
	want := []string{"n26", "seidor"} // deduped, sorted
	if !slices.Equal(got, want) {
		t.Errorf("joppyCompanySlugs() = %v, want %v", got, want)
	}
}

func TestJoppyWorkMode(t *testing.T) {
	cases := []struct {
		name                   string
		remote, hybrid, office bool
		want                   string
	}{
		{"remote only", true, false, false, "remote"},
		{"hybrid only", false, true, false, "hybrid"},
		{"onsite only", false, false, true, "onsite"},
		{"remote+hybrid", true, true, false, "hybrid"},
		{"hybrid+office", false, true, true, "hybrid"},
		{"all three", true, true, true, "hybrid"},
		{"remote+office, no hybrid", true, false, true, ""},
		{"none set", false, false, false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := joppyWorkMode(c.remote, c.hybrid, c.office); got != c.want {
				t.Errorf("joppyWorkMode(%v,%v,%v) = %q, want %q", c.remote, c.hybrid, c.office, got, c.want)
			}
		})
	}
}

func TestJoppyLocation(t *testing.T) {
	cases := []struct {
		name    string
		cities  []string
		located string
		want    string
	}{
		{"cities present", []string{"Barcelona, Spain", "Madrid, Spain"}, "España", "Barcelona, Spain; Madrid, Spain"},
		{"cities empty, located fallback", nil, "España", "España"},
		{"both empty", nil, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := joppyLocation(joppyPlace{Cities: c.cities, Located: c.located}); got != c.want {
				t.Errorf("joppyLocation() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestJoppyDescriptionExtras(t *testing.T) {
	job := joppyJob{
		SponsorVisa:      true,
		RelocationPack:   true,
		OnlyEuCandidates: true,
		Skills: []joppySkill{
			{Name: "SAP", IsMandatory: true},
			{Name: "SAP FI", IsMandatory: false},
		},
		Languages: []joppyLanguage{
			{Name: "spanish", Level: 5},
			{Name: "english", Level: 3},
		},
	}
	got := joppyDescriptionExtras(job)
	for _, want := range []string{"SAP", "SAP FI", "spanish", "english", "3", "5"} {
		if !strings.Contains(got, want) {
			t.Errorf("joppyDescriptionExtras() = %q, missing %q", got, want)
		}
	}
	if !strings.Contains(strings.ToLower(got), "visa") {
		t.Errorf("joppyDescriptionExtras() = %q, missing visa sponsorship note", got)
	}
	if !strings.Contains(strings.ToLower(got), "relocation") {
		t.Errorf("joppyDescriptionExtras() = %q, missing relocation note", got)
	}
}

func TestJoppyDescriptionExtrasRendersRealParagraphs(t *testing.T) {
	job := joppyJob{
		SponsorVisa: true,
		Skills:      []joppySkill{{Name: "SAP", IsMandatory: true}},
		Languages:   []joppyLanguage{{Name: "english", Level: 4}},
	}
	got := joppyDescriptionExtras(job)
	if !strings.Contains(got, "<p>") {
		t.Errorf("joppyDescriptionExtras() = %q, want real HTML paragraphs (rendered via {@html} on the frontend)", got)
	}
	if strings.Contains(got, "\n\n") {
		t.Errorf("joppyDescriptionExtras() = %q, contains literal blank-line separators instead of HTML paragraph breaks", got)
	}
}

func TestJoppyDescriptionExtrasOmitsUnsetFacts(t *testing.T) {
	got := joppyDescriptionExtras(joppyJob{})
	for _, unwanted := range []string{"visa", "relocation", "EU"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("joppyDescriptionExtras() = %q, should not mention %q when unset", got, unwanted)
		}
	}
}

const joppySeidorJob1 = `{
  "uid":"7941c018-b7c5-42f2-a92a-bced77649fc4",
  "title":"Consultor/a Senior SAP FI-CO",
  "description":"<p>Great role.</p>",
  "isSalaryPublic":true,
  "onlyEuCandidates":true,
  "sponsorVisa":false,
  "relocationPack":false,
  "skills":[{"name":"SAP","isMandatory":true},{"name":"SAP Implementation","isMandatory":false}],
  "place":{"isRemote":true,"isHybrid":false,"isOffice":false,"located":"Madrid, Barcelona o Zona norte de España","cities":[]},
  "languages":[{"name":"spanish","level":5},{"name":"english","level":3}],
  "salaryMin":40000,"salaryMax":50000
}`

const joppySeidorJob2NonPublicSalary = `{
  "uid":"14d16b99-0cdb-4813-8644-dd6f3ccf0d64",
  "title":"Consultor/a SAP Logística",
  "description":"<p>Second role.</p>",
  "isSalaryPublic":false,
  "onlyEuCandidates":false,
  "sponsorVisa":true,
  "relocationPack":true,
  "skills":[{"name":"Logistics","isMandatory":true}],
  "place":{"isRemote":true,"isHybrid":true,"isOffice":false,"located":"España","cities":["Valencia, Spain","Barcelona, Spain"]},
  "languages":[],
  "salaryMin":45000,"salaryMax":60000
}`

const joppyMissingIdentityJob = `{
  "uid":"",
  "title":"No id, dropped",
  "description":"<p>x</p>",
  "place":{}
}`

func TestJoppyFetch(t *testing.T) {
	sitemap := joppySitemapXML(
		"https://www.joppy.me/companies/seidor",
		"https://www.joppy.me/companies/seidor/7941c018-b7c5-42f2-a92a-bced77649fc4",
		"https://www.joppy.me/companies/seidor/14d16b99-0cdb-4813-8644-dd6f3ccf0d64",
		"https://www.joppy.me/companies/empty-co",
		"https://www.joppy.me/companies/broken-co",
		"https://www.joppy.me/companies/broken-co/deadbeef-0000-0000-0000-000000000000",
	)
	fake := (&routedHTTP{}).
		route("sitemap.directory.xml", sitemap).
		route("/companies/seidor", joppyCompanyPage("SEIDOR", "seidor", joppySeidorJob1, joppySeidorJob2NonPublicSalary, joppyMissingIdentityJob)).
		route("/companies/empty-co", joppyCompanyPage("Empty Co", "empty-co")).
		routeErr("/companies/broken-co", errors.New("boom"))

	jobs, err := NewJoppy(fake).Fetch(context.Background(), CompanyEntry{Provider: "joppy", Company: "Joppy"})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("Fetch() returned %d jobs, want 2 (one company failed, one had none, one identity-less job dropped): %+v", len(jobs), jobs)
	}

	var j1, j2 *Job
	for i := range jobs {
		switch jobs[i].ExternalID {
		case "7941c018-b7c5-42f2-a92a-bced77649fc4":
			j1 = &jobs[i]
		case "14d16b99-0cdb-4813-8644-dd6f3ccf0d64":
			j2 = &jobs[i]
		}
	}
	if j1 == nil || j2 == nil {
		t.Fatalf("expected both seidor jobs present, got %+v", jobs)
	}

	if j1.Company != "SEIDOR" {
		t.Errorf("j1.Company = %q, want SEIDOR", j1.Company)
	}
	if j1.URL != "https://www.joppy.me/companies/seidor/7941c018-b7c5-42f2-a92a-bced77649fc4" {
		t.Errorf("j1.URL = %q", j1.URL)
	}
	if j1.WorkMode != "remote" {
		t.Errorf("j1.WorkMode = %q, want remote", j1.WorkMode)
	}
	if j1.SalaryMin == nil || *j1.SalaryMin != 40000 || j1.SalaryMax == nil || *j1.SalaryMax != 50000 {
		t.Errorf("j1 salary = %v/%v, want 40000/50000", j1.SalaryMin, j1.SalaryMax)
	}
	if j1.SalaryCurrency != "EUR" || j1.SalaryPeriod != "year" {
		t.Errorf("j1 salary currency/period = %q/%q, want EUR/year", j1.SalaryCurrency, j1.SalaryPeriod)
	}
	if !slices.Contains(j1.Skills, "sap") {
		t.Errorf("j1.Skills = %v, want it to contain the canonicalized \"sap\"", j1.Skills)
	}
	// EnglishLevel is deliberately left unset (see design.md): Joppy's own 1-5 scale has no
	// authoritative CEFR equivalence, so the requirement is folded into the description instead.
	if j1.EnglishLevel != "" {
		t.Errorf("j1.EnglishLevel = %q, want unset — no guessed CEFR mapping", j1.EnglishLevel)
	}
	if !strings.Contains(j1.Description, "<p>") {
		t.Errorf("j1.Description = %q, want the appended extras rendered as real HTML paragraphs", j1.Description)
	}

	// j2 has isSalaryPublic=false: salary must NOT be published even though the platform's own
	// payload carries salaryMin/salaryMax internally.
	if j2.SalaryMin != nil || j2.SalaryMax != nil {
		t.Errorf("j2 salary = %v/%v, want unset (isSalaryPublic=false)", j2.SalaryMin, j2.SalaryMax)
	}
	if j2.WorkMode != "hybrid" {
		t.Errorf("j2.WorkMode = %q, want hybrid", j2.WorkMode)
	}
	if j2.Location != "Valencia, Spain; Barcelona, Spain" {
		t.Errorf("j2.Location = %q", j2.Location)
	}
	if !strings.Contains(strings.ToLower(j2.Description), "visa") {
		t.Errorf("j2.Description missing visa sponsorship note: %q", j2.Description)
	}
}

func TestJoppyFetchSitemapFailureAbortsCrawl(t *testing.T) {
	fake := (&routedHTTP{}).routeErr("sitemap.directory.xml", errors.New("sitemap down"))
	_, err := NewJoppy(fake).Fetch(context.Background(), CompanyEntry{Provider: "joppy", Company: "Joppy"})
	if err == nil {
		t.Error("Fetch() error = nil, want error when the sitemap itself cannot be read")
	}
}
