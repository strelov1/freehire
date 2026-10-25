package sources

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"golang.org/x/time/rate"
)

// nlFixture reads what the Dutch public boards served on 2026-10-25: each sitemap trimmed to a few
// entries and each posting page trimmed to its <title>, its JobPosting script and, for
// werkenvoornederland, the page's sections and analytics script. The recruiter contact section
// is replaced with a placeholder so the fixture carries no real phone number.
func nlFixture(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return string(b)
}

func newTestWerkenVoorNederland(t *testing.T) (werkenVoorNederland, *routedHTTP) {
	fake := (&routedHTTP{}).
		route("/sitemap-vacatures.xml", nlFixture(t, "werkenvoornederland/sitemap.xml")).
		route("-CJIB-2026-9570", nlFixture(t, "werkenvoornederland/detail_cjib.html")).
		route("-KM-2026-2837", nlFixture(t, "werkenvoornederland/detail_km.html")).
		route("-FIN-2026-6979", nlFixture(t, "werkenvoornederland/detail_stage.html")).
		routeErr("-DJI-2026-0001", errors.New("unexpected status 404"))
	s := NewWerkenVoorNederland(fake).(werkenVoorNederland)
	s.limiter = rate.NewLimiter(rate.Inf, 1)
	return s, fake
}

func TestWerkenVoorNederlandProvider(t *testing.T) {
	s, ok := All(nil)["werkenvoornederland"]
	if !ok || s.Provider() != "werkenvoornederland" {
		t.Fatal("All() must register werkenvoornederland")
	}
	if _, ok := s.(HydratingSource); !ok {
		t.Error("werkenvoornederland must be a HydratingSource: the body costs a page request")
	}
	if _, ok := s.(boardless); !ok {
		t.Error("werkenvoornederland must be boardless: one sitemap names every posting")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("werkenvoornederland must be an aggregator: the organisation is read per posting")
	}
}

func TestWerkenVoorNederlandHydratesUnseenPostings(t *testing.T) {
	s, _ := newTestWerkenVoorNederland(t)
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{Provider: "werkenvoornederland"}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	got := byExternalID(jobs)
	if len(jobs) != 3 {
		t.Fatalf("jobs = %d, want the 3 open postings (the closed one is a 404, /bewaard is no posting)", len(jobs))
	}

	k8s := got["CJIB-2026-9570"]
	if k8s.Title != "Kubernetes Software Platform Engineer" || k8s.Company != "Centraal Justitieel Incassobureau" || k8s.Location != "Leeuwarden" {
		t.Errorf("CJIB = %q %q %q", k8s.Title, k8s.Company, k8s.Location)
	}
	if !k8s.IsTechHint {
		t.Error("CJIB: vakgebied ICT must set IsTechHint")
	}
	if k8s.EmploymentType != "" {
		t.Errorf("CJIB EmploymentType = %q: TEMPORARY is a fixed-term appointment, not contract work", k8s.EmploymentType)
	}
	if k8s.SalaryMin == nil || *k8s.SalaryMin != 4818 || k8s.SalaryMax == nil || *k8s.SalaryMax != 7094 ||
		k8s.SalaryPeriod != "month" || k8s.SalaryCurrency != "EUR" {
		t.Errorf("CJIB salary = %v-%v %s/%s", k8s.SalaryMin, k8s.SalaryMax, k8s.SalaryCurrency, k8s.SalaryPeriod)
	}
	if len(k8s.Countries) != 1 || k8s.Countries[0] != "nl" || k8s.PostedAt == nil || k8s.PostedAt.Format("2006-01-02") != "2026-09-09" {
		t.Errorf("CJIB countries/posted = %v %v", k8s.Countries, k8s.PostedAt)
	}
	for _, want := range []string{"<h2>Dit ga je doen</h2>", "Go (Golang)", "<h2>Dit vragen wij</h2>", "<h2>Hier kom je te werken</h2>"} {
		if !strings.Contains(k8s.Description, want) {
			t.Errorf("CJIB description lacks %q", want)
		}
	}
	if strings.Contains(k8s.Description, "06-12345678") || strings.Contains(k8s.Description, "Stel gerust je vraag") {
		t.Error("CJIB description must not copy the recruiter contact section")
	}

	km := got["KM-2026-2837"]
	if km.IsTechHint || km.Company != "Ministerie van Defensie" || km.EmploymentType != "" {
		t.Errorf("KM tech/company/type = %v %q %q, want no hint, Defensie, no type for FULL_TIME+PART_TIME", km.IsTechHint, km.Company, km.EmploymentType)
	}

	stage := got["FIN-2026-6979"]
	if stage.EmploymentType != "internship" || !stage.IsTechHint {
		t.Errorf("FIN stage type/tech = %q %v, want internship and ICT", stage.EmploymentType, stage.IsTechHint)
	}
	if stage.SalaryMin == nil || *stage.SalaryMin != 800 || stage.SalaryMax == nil || *stage.SalaryMax != 800 {
		t.Errorf("FIN stage salary = %v-%v, want a lone value as both ends", stage.SalaryMin, stage.SalaryMax)
	}
}

func TestWerkenVoorNederlandHydratesMostRecentlyModifiedFirst(t *testing.T) {
	s, _ := newTestWerkenVoorNederland(t)
	s.maxNewPerRun = 1
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{Provider: "werkenvoornederland"}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "CJIB-2026-9570" {
		t.Fatalf("jobs = %+v, want only the most recently modified posting", jobs)
	}
}

func TestWerkenVoorNederlandRefreshesSeenPostingsWithoutFetchingThem(t *testing.T) {
	s, fake := newTestWerkenVoorNederland(t)
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{Provider: "werkenvoornederland"}, func(string) bool { return true })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 4 || fake.calls != 1 {
		t.Fatalf("jobs/requests = %d/%d, want 4 refreshes from the one sitemap request", len(jobs), fake.calls)
	}
	for _, j := range jobs {
		if !j.SeenRefresh {
			t.Errorf("job %s must be a SeenRefresh", j.ExternalID)
		}
	}
}

func TestWerkenVoorNederlandSitemapFailureFailsTheCrawl(t *testing.T) {
	s := NewWerkenVoorNederland(&routedHTTP{}).(werkenVoorNederland)
	s.limiter = rate.NewLimiter(rate.Inf, 1)
	if _, err := s.Fetch(context.Background(), CompanyEntry{Provider: "werkenvoornederland"}); err == nil {
		t.Fatal("a sitemap that cannot be read must fail the crawl")
	}
}

func TestWerkenVoorNederlandDropsTheStreetFromTheLocality(t *testing.T) {
	page := `{"@type":"JobPosting","title":"Jurist","hiringOrganization":{"name":"Immigratie- en Naturalisatiedienst"},` +
		`"jobLocation":{"@type":"Place","address":{"addressLocality":"Den Haag (Rijnstraat 8)","addressCountry":"NL"}}}`
	j, ok := wvnDetail(ldPage(page), listedPosting{id: "IND-2026-1644"})
	if !ok || j.Location != "Den Haag" {
		t.Fatalf("location = %q (ok=%v), want the city without the office's street", j.Location, ok)
	}
}

func TestWerkenVoorNederlandPostingCode(t *testing.T) {
	for raw, want := range map[string]string{
		"https://www.werkenvoornederland.nl/vacatures/kubernetes-software-platform-engineer-CJIB-2026-9570": "CJIB-2026-9570",
		"https://www.werkenvoornederland.nl/vacatures/adviseur-data-en-ai-cio-487383-DEF2644-2005-6416":     "DEF2644-2005-6416",
		"https://www.werkenvoornederland.nl/vacatures/bewaard":                                              "",
		"https://www.werkenvoornederland.nl/organisaties/cjib-2026-9570":                                    "",
	} {
		if got := firstSubmatch(wvnPostingCode, raw); got != want {
			t.Errorf("code(%q) = %q, want %q", raw, got, want)
		}
	}
}
