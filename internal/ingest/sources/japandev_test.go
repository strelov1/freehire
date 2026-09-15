package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
)

const japanDevLeaf = "https://japan-dev.com/cdn/sitemaps/sitemap.xml"

func japanDevIndexXML() string {
	return `<?xml version="1.0"?><sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><sitemap><loc>` + japanDevLeaf + `</loc></sitemap></sitemapindex>`
}

func japanDevURLSet(urls ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	for _, u := range urls {
		fmt.Fprintf(&b, "<url><loc>%s</loc></url>", u)
	}
	b.WriteString(`</urlset>`)
	return b.String()
}

type japanDevTestJob struct {
	Title             string
	Slug              string
	Company           string
	Location          string
	Body              string
	ApplicationURL    string
	PublishedAt       string
	UpdatedAt         string
	RemoteLevel       string
	EmploymentType    string
	SeniorityLevel    string
	CandidateLocation string
	SponsorsVisas     string
	JapaneseLevel     string
	EnglishLevel      string
	SalaryMin         int
	SalaryMax         int
	Skills            []string
}

// japanDevPage emits the small subset of Nuxt/devalue's reference-table shape the production
// parser reads. The production page has thousands of cells; what matters is that object fields
// point at top-level table indexes and that the primary object is reached through a semantic
// {"job": <ref>} wrapper rather than by position.
func japanDevPage(j japanDevTestJob) string {
	table := []any{nil, nil}
	add := func(v any) int {
		table = append(table, v)
		return len(table) - 1
	}
	job := map[string]any{}
	put := func(k string, v any) { job[k] = add(v) }
	put("title", j.Title)
	put("slug", j.Slug)
	put("location", j.Location)
	put("raw_content", j.Body)
	if j.ApplicationURL == "" {
		put("application_url", nil)
	} else {
		put("application_url", j.ApplicationURL)
	}
	put("published_at", j.PublishedAt)
	put("updated_at", j.UpdatedAt)
	put("remote_level", j.RemoteLevel)
	put("employment_type", j.EmploymentType)
	put("seniority_level", j.SeniorityLevel)
	put("candidate_location", j.CandidateLocation)
	put("sponsors_visas", j.SponsorsVisas)
	put("japanese_level_enum", j.JapaneseLevel)
	put("english_level_enum", j.EnglishLevel)
	if j.SalaryMin > 0 {
		put("salary_min", j.SalaryMin)
	} else {
		put("salary_min", nil)
	}
	if j.SalaryMax > 0 {
		put("salary_max", j.SalaryMax)
	} else {
		put("salary_max", nil)
	}

	company := map[string]any{"name": add(j.Company)}
	job["company"] = add(company)
	var skillRefs []any
	for _, name := range j.Skills {
		skillRefs = append(skillRefs, add(map[string]any{"name": add(name)}))
	}
	job["skills"] = add(skillRefs)
	table[0] = map[string]any{"job": 1}
	table[1] = job
	b, _ := json.Marshal(table)
	return `<html><body><script type="application/json" id="__NUXT_DATA__">` + string(b) + `</script><div class="job-detail-main-content"><div class="body">` + j.Body + `</div></div></body></html>`
}

func japanDevFixture(jobURL, detail string) *routedHTTP {
	return (&routedHTTP{}).
		route("/cdn/sitemaps/sitemap.xml", japanDevURLSet(jobURL)).
		route("/sitemap.xml", japanDevIndexXML()).
		route(jobURL, detail)
}

func TestJapanDevMarkersAndRegistration(t *testing.T) {
	src := NewJapanDev(&routedHTTP{})
	if src.Provider() != "japandev" {
		t.Fatalf("Provider() = %q", src.Provider())
	}
	if _, ok := src.(HydratingSource); !ok {
		t.Error("japandev must implement HydratingSource")
	}
	if _, ok := src.(boardless); !ok {
		t.Error("japandev must be boardless: one sitemap covers the site")
	}
	if _, ok := src.(aggregator); !ok {
		t.Error("japandev must be an aggregator so first-party ATS copies win dedup")
	}
	if _, ok := src.(fullCatalog); ok {
		t.Error("japandev must not claim fullCatalog: sitemap and live /jobs count differ")
	}
	if _, ok := src.(fullBoardListing); ok {
		t.Error("japandev must not claim fullBoardListing")
	}
	if _, ok := All(nil)["japandev"]; !ok {
		t.Error(`All(nil)["japandev"] missing`)
	}
	if !slices.Contains(AggregatorProviders(Taxonomy()), "japandev") {
		t.Error("japandev missing from AggregatorProviders")
	}
}

func TestJapanDevMapsStructuredPrimaryJob(t *testing.T) {
	const (
		id     = "mujin-senior-software-engineer-wes--fm-uhpwd5"
		jobURL = "https://japan-dev.com/jobs/mujin/" + id
		apply  = "https://jobs.lever.co/mujininc/70b4fe3c/apply?source=japan-dev.com"
	)
	detail := japanDevPage(japanDevTestJob{
		Title: "Senior Software Engineer (WES & Fleet Management)", Slug: id, Company: "Mujin",
		Location: "Tokyo", Body: "<p>Build robotics software in production.</p>", ApplicationURL: apply,
		PublishedAt: "2026-05-25T17:00:27.000Z", RemoteLevel: "remote_level_none",
		EmploymentType: "employment_type_full_time", SeniorityLevel: "seniority_level_mid_level",
		CandidateLocation: "candidate_location_anywhere", SponsorsVisas: "sponsors_visas_yes",
		JapaneseLevel: "japanese_level_not_required", EnglishLevel: "english_level_business_level",
		SalaryMin: 8000000, SalaryMax: 15000000, Skills: []string{"Engineering", "C++", "Python"},
	})
	jobs, err := NewJapanDev(japanDevFixture(jobURL, detail)).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != id || j.Title != "Senior Software Engineer (WES & Fleet Management)" || j.Company != "Mujin" {
		t.Errorf("identity = %q / %q / %q", j.ExternalID, j.Title, j.Company)
	}
	if j.URL != apply {
		t.Errorf("URL = %q, want official ATS %q", j.URL, apply)
	}
	if j.Location != "Tokyo" || !slices.Equal(j.Countries, []string{"jp"}) {
		t.Errorf("geography = %q / %v", j.Location, j.Countries)
	}
	if j.WorkMode != "onsite" || j.Remote {
		t.Errorf("work mode = %q Remote=%v", j.WorkMode, j.Remote)
	}
	if j.EmploymentType != "full_time" || j.Seniority != "middle" || !j.IsTechHint {
		t.Errorf("facets employment=%q seniority=%q isTech=%v", j.EmploymentType, j.Seniority, j.IsTechHint)
	}
	if j.SalaryMin == nil || *j.SalaryMin != 8000000 || j.SalaryMax == nil || *j.SalaryMax != 15000000 || j.SalaryCurrency != "JPY" || j.SalaryPeriod != "year" {
		t.Errorf("salary = %v..%v %s/%s", j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-05-25" {
		t.Errorf("PostedAt = %v", j.PostedAt)
	}
	for _, want := range []string{"candidates may apply from outside Japan", "Visa sponsorship: available", "Japanese: Not required", "English: Business level", "C++", "Build robotics software"} {
		if !strings.Contains(j.Description, want) {
			t.Errorf("description missing %q: %s", want, j.Description)
		}
	}
}

func TestJapanDevSeenPostingSkipsDetail(t *testing.T) {
	const (
		id     = "known-job-abc123"
		jobURL = "https://japan-dev.com/jobs/acme/" + id
	)
	http := (&routedHTTP{}).
		route("/cdn/sitemaps/sitemap.xml", japanDevURLSet(jobURL)).
		route("/sitemap.xml", japanDevIndexXML()).
		routeErr(jobURL, errors.New("detail must not be requested"))
	jobs, err := NewJapanDev(http).(HydratingSource).FetchNew(context.Background(), CompanyEntry{}, func(got string) bool { return got == id })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != id || !jobs[0].SeenRefresh {
		t.Fatalf("seen result = %+v", jobs)
	}
}

func TestJapanDevMissingOfficialApplyURLFallsBackToDetailPage(t *testing.T) {
	const (
		id     = "email-apply-role-abc123"
		jobURL = "https://japan-dev.com/jobs/acme/" + id
	)
	detail := japanDevPage(japanDevTestJob{Title: "Backend Engineer", Slug: id, Company: "Acme", Location: "Tokyo", Body: "<p>Build APIs.</p>"})
	jobs, err := NewJapanDev(japanDevFixture(jobURL, detail)).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].URL != jobURL {
		t.Fatalf("fallback URL = %+v, want %s", jobs, jobURL)
	}
	if jobs[0].SalaryCurrency != "" || jobs[0].SalaryPeriod != "" {
		t.Errorf("unstated salary invented currency/period: %s/%s", jobs[0].SalaryCurrency, jobs[0].SalaryPeriod)
	}
}

func TestJapanDevFailedNewDetailIsDeferred(t *testing.T) {
	const (
		badID   = "bad-role-abc123"
		goodID  = "good-role-def456"
		badURL  = "https://japan-dev.com/jobs/acme/" + badID
		goodURL = "https://japan-dev.com/jobs/acme/" + goodID
	)
	http := (&routedHTTP{}).
		route("/cdn/sitemaps/sitemap.xml", japanDevURLSet(badURL, goodURL)).
		route("/sitemap.xml", japanDevIndexXML()).
		routeErr(badURL, errors.New("temporary refusal")).
		route(goodURL, japanDevPage(japanDevTestJob{Title: "Platform Engineer", Slug: goodID, Company: "Acme", Location: "Tokyo", Body: "<p>Operate systems.</p>"}))
	jobs, err := NewJapanDev(http).(HydratingSource).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != goodID {
		t.Fatalf("jobs = %+v, want only the readable posting", jobs)
	}
}

func TestJapanDevRejectsSitemapShapeDrift(t *testing.T) {
	http := (&routedHTTP{}).route("/sitemap.xml", `<?xml version="1.0"?><sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><sitemap><loc>https://japan-dev.com/other.xml</loc></sitemap></sitemapindex>`)
	if jobs, err := NewJapanDev(http).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatalf("shape drift returned %d jobs without an error", len(jobs))
	}
}

func TestJapanDevJobIDOnlyAcceptsCanonicalPostingURLs(t *testing.T) {
	if got := japanDevJobID("https://japan-dev.com/jobs/mujin/role-123"); got != "role-123" {
		t.Errorf("id = %q", got)
	}
	for _, raw := range []string{
		"https://japan-dev.com/jobs",
		"https://japan-dev.com/jobs/apply-from-abroad",
		"https://evil.example/jobs/mujin/role-123",
		"http://japan-dev.com/jobs/mujin/role-123",
	} {
		if got := japanDevJobID(raw); got != "" {
			t.Errorf("japanDevJobID(%q) = %q, want empty", raw, got)
		}
	}
}
func TestJapanDevWorldwideRemoteDoesNotInventAWorkplaceCountry(t *testing.T) {
	d := japanDevNuxtJob{Location: "Remote", RemoteLevel: "remote_level_full_worldwide"}
	if got := japanDevCountries(d); len(got) != 0 {
		t.Errorf("Countries = %v, want empty for worldwide remote", got)
	}
	d.RemoteLevel = "remote_level_full_japan"
	if got := japanDevCountries(d); !slices.Equal(got, []string{"jp"}) {
		t.Errorf("Japan-scoped remote Countries = %v, want [jp]", got)
	}
}
