package sources

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/html"
	"golang.org/x/time/rate"
)

// The shapes below are trimmed from live glints.com responses fetched 2026-11-17: the sitemap
// index and job shards (each posting listed twice, once per locale), and a posting page whose
// __NEXT_DATA__ carries the structured fields and whose ld+json carries the HTML body.

const (
	glintsITID    = "96770699-c989-4cea-9c33-ab0d2d12c888"
	glintsOtherID = "d135e40f-d2af-421c-b7ff-c14f24a4dbcc"
	glintsVNID    = "5a1c2e43-0b6f-4d0e-8a57-0a8d0f3a9b11"
	glintsITURL   = "https://glints.com/id/opportunities/jobs/software-developer/" + glintsITID
	glintsVNURL   = "https://glints.com/vn/opportunities/jobs/backend-developer/" + glintsVNID
)

// glintsShard renders a job shard the way the site does: every posting twice, the English locale
// second.
func glintsShard(urls ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	for _, u := range urls {
		en := u
		en = strings.Replace(en, "/opportunities/", "/en/opportunities/", 1)
		fmt.Fprintf(&b, "<url><loc>%s</loc></url><url><loc>%s</loc></url>", u, en)
	}
	b.WriteString(`</urlset>`)
	return b.String()
}

const glintsIndex = `<?xml version="1.0" encoding="UTF-8"?><sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` +
	`<sitemap><loc>https://glints.com/sitemap_static.xml</loc></sitemap>` +
	`<sitemap><loc>https://glints.com/sitemap_job_id_1.xml</loc></sitemap>` +
	`<sitemap><loc>https://glints.com/sitemap_job_vn_1.xml</loc></sitemap>` +
	`<sitemap><loc>https://glints.com/sitemap_sourced_job_id_1.xml</loc></sitemap>` +
	`<sitemap><loc>https://glints.com/sitemap_companies_id_1.xml</loc></sitemap>` +
	`</sitemapindex>`

// glintsPageHTML renders a posting page: the structured state and the JobPosting block. mutate edits
// the state before it is serialised.
func glintsPageHTML(t *testing.T, id string, mutate func(d map[string]any)) string {
	t.Helper()
	data := map[string]any{
		"id":                    id,
		"title":                 "Software Developer",
		"status":                "OPEN",
		"createdAt":             "2026-11-17T09:06:25.911112Z",
		"expiryDate":            "2026-12-18",
		"closedAt":              nil,
		"type":                  "FULL_TIME",
		"workArrangementOption": "HYBRID",
		"isRemote":              false,
		"minYearsOfExperience":  3,
		"shouldShowSalary":      true,
		"company":               map[string]any{"name": "MSBU"},
		"country":               map[string]any{"code": "ID", "name": "Indonesia"},
		"location": map[string]any{
			"formattedName": "Tanah Abang", "level": 4,
			"parents": []any{map[string]any{
				"formattedName": "Jakarta Pusat", "level": 3,
				"parents": []any{map[string]any{
					"formattedName": "DKI Jakarta", "level": 2,
					"parents": []any{map[string]any{"formattedName": "Indonesia", "level": 1}},
				}},
			}},
		},
		"salaries": []any{map[string]any{
			"salaryType": "BASIC", "salaryMode": "MONTH", "minAmount": 10000000, "maxAmount": nil, "CurrencyCode": "IDR",
		}},
		"hierarchicalJobCategory": map[string]any{
			"id": "b16aede4-0000-4000-8000-000000000000", "level": 3,
			"parents": []any{
				map[string]any{"id": "b298311a-0000-4000-8000-000000000000", "level": 2},
				map[string]any{"id": glintsITCategory, "level": 1},
			},
		},
	}
	if mutate != nil {
		mutate(data)
	}
	next, err := json.Marshal(map[string]any{"props": map[string]any{"pageProps": map[string]any{
		"initialData": map[string]any{"code": "OK", "data": data},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	ld, err := json.Marshal(map[string]any{
		"@context": "https://schema.org/", "@type": "JobPosting", "title": "Software Developer",
		"description": "<p>Build services in Java.</p>\n<ul><li>Spring Boot</li></ul>",
	})
	if err != nil {
		t.Fatal(err)
	}
	return `<html><head><script type="application/ld+json">` + string(ld) + `</script></head><body>` +
		`<script id="__NEXT_DATA__" type="application/json">` + string(next) + `</script></body></html>`
}

// glintsFake answers the sitemaps and posting pages from fixtures and counts what was asked.
type glintsFake struct {
	mu    sync.Mutex
	xmls  map[string]string
	pages map[string]string
	errs  map[string]error
	asked []string
}

func (f *glintsFake) lookup(url string, src map[string]string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = append(f.asked, url)
	if err, ok := f.errs[url]; ok {
		return "", err
	}
	body, ok := src[url]
	if !ok {
		return "", &StatusError{Code: 404}
	}
	return body, nil
}

func (f *glintsFake) GetXML(_ context.Context, url string, v any) error {
	body, err := f.lookup(url, f.xmls)
	if err != nil {
		return err
	}
	return xml.Unmarshal([]byte(body), v)
}

func (f *glintsFake) GetHTML(_ context.Context, url string) (*html.Node, error) {
	body, err := f.lookup(url, f.pages)
	if err != nil {
		return nil, err
	}
	return html.Parse(strings.NewReader(body))
}

func (f *glintsFake) askedFor(url string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, u := range f.asked {
		if u == url {
			n++
		}
	}
	return n
}

func newGlintsFake(t *testing.T) *glintsFake {
	return &glintsFake{
		xmls: map[string]string{
			glintsSitemapIndex:                        glintsIndex,
			"https://glints.com/sitemap_job_id_1.xml": glintsShard(glintsITURL, "https://glints.com/id/opportunities/jobs/kitchen/"+glintsOtherID),
			"https://glints.com/sitemap_job_vn_1.xml": glintsShard(glintsVNURL),
		},
		pages: map[string]string{
			glintsITURL: glintsPageHTML(t, glintsITID, nil),
			glintsVNURL: glintsPageHTML(t, glintsVNID, func(d map[string]any) {
				d["title"] = "Backend Developer"
				d["country"] = map[string]any{"code": "VN", "name": "Vietnam"}
			}),
		},
		errs: map[string]error{},
	}
}

func newTestGlints(f *glintsFake) glints {
	g := NewGlints(f).(glints)
	g.limiter = rate.NewLimiter(rate.Inf, 1)
	return g
}

func glintsByID(jobs []Job) map[string]Job {
	m := map[string]Job{}
	for _, j := range jobs {
		m[j.ExternalID] = j
	}
	return m
}

func TestGlintsReadsTheTechnicalPostingsOfEveryCountryShard(t *testing.T) {
	f := newGlintsFake(t)
	jobs, err := newTestGlints(f).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	got := glintsByID(jobs)
	if len(jobs) != 2 || got[glintsITID].ExternalID == "" || got[glintsVNID].ExternalID == "" {
		t.Fatalf("jobs = %+v, want the Indonesian and the Vietnamese developer once each", jobs)
	}
	if n := f.askedFor("https://glints.com/sitemap_sourced_job_id_1.xml"); n != 0 {
		t.Errorf("the sourced-job shard (JobStreet copies) was read %d times", n)
	}
	if n := f.askedFor("https://glints.com/id/opportunities/jobs/kitchen/" + glintsOtherID); n != 0 {
		t.Errorf("a non-technical posting's page was bought %d times", n)
	}
}

func TestGlintsMapsAPosting(t *testing.T) {
	f := newGlintsFake(t)
	jobs, err := newTestGlints(f).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	j := glintsByID(jobs)[glintsITID]
	if j.URL != glintsITURL || j.Title != "Software Developer" || j.Company != "MSBU" {
		t.Errorf("identity = %q %q %q", j.URL, j.Title, j.Company)
	}
	if j.Location != "Jakarta Pusat, DKI Jakarta, Indonesia" {
		t.Errorf("Location = %q, want the city, province and country without the district", j.Location)
	}
	if len(j.Countries) != 1 || j.Countries[0] != "id" {
		t.Errorf("Countries = %v, want [id]", j.Countries)
	}
	if j.WorkMode != "hybrid" || j.Remote {
		t.Errorf("WorkMode = %q Remote = %v, want hybrid and not remote", j.WorkMode, j.Remote)
	}
	if j.EmploymentType != "full_time" || !j.IsTechHint {
		t.Errorf("EmploymentType = %q IsTechHint = %v", j.EmploymentType, j.IsTechHint)
	}
	if j.SalaryMin == nil || *j.SalaryMin != 10000000 || j.SalaryMax != nil || j.SalaryCurrency != "IDR" || j.SalaryPeriod != "month" {
		t.Errorf("salary = %v-%v %q/%q", j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod)
	}
	if j.ExperienceYearsMin == nil || *j.ExperienceYearsMin != 3 {
		t.Errorf("ExperienceYearsMin = %v, want 3", j.ExperienceYearsMin)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-11-17" {
		t.Errorf("PostedAt = %v", j.PostedAt)
	}
	if j.ApplicationDeadline == nil || j.ApplicationDeadline.Format("2006-01-02") != "2026-12-18" {
		t.Errorf("ApplicationDeadline = %v", j.ApplicationDeadline)
	}
	if !strings.Contains(j.Description, "Spring Boot") {
		t.Errorf("Description = %q, want the ld+json body", j.Description)
	}
}

func TestGlintsRefreshesASeenPostingWithoutBuyingItsPage(t *testing.T) {
	f := newGlintsFake(t)
	seen := func(id string) bool { return id == glintsITID }
	jobs, err := newTestGlints(f).FetchNew(context.Background(), CompanyEntry{}, seen)
	if err != nil {
		t.Fatal(err)
	}
	got := glintsByID(jobs)
	if !got[glintsITID].SeenRefresh || got[glintsITID].Title != "" {
		t.Errorf("seen posting = %+v, want a bare liveness refresh", got[glintsITID])
	}
	if got[glintsVNID].SeenRefresh || got[glintsVNID].Title == "" {
		t.Errorf("new posting = %+v, want it hydrated", got[glintsVNID])
	}
	if n := f.askedFor(glintsITURL); n != 0 {
		t.Errorf("a seen posting's page was bought %d times", n)
	}
}

func TestGlintsSkipsWhatIsNotAnOpenPostingWithABodyAndAnEmployer(t *testing.T) {
	for name, mutate := range map[string]func(d map[string]any){
		"closed":          func(d map[string]any) { d["status"] = "CLOSED" },
		"closedAt":        func(d map[string]any) { d["closedAt"] = "2026-11-01T00:00:00Z" },
		"another id":      func(d map[string]any) { d["id"] = glintsOtherID },
		"no employer":     func(d map[string]any) { d["company"] = map[string]any{"name": " "} },
		"no title":        func(d map[string]any) { d["title"] = "" },
		"unreadable json": nil,
	} {
		t.Run(name, func(t *testing.T) {
			f := newGlintsFake(t)
			if mutate != nil {
				f.pages[glintsITURL] = glintsPageHTML(t, glintsITID, mutate)
			} else {
				f.pages[glintsITURL] = `<html><body>no state</body></html>`
			}
			jobs, err := newTestGlints(f).Fetch(context.Background(), CompanyEntry{})
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := glintsByID(jobs)[glintsITID]; ok {
				t.Error("the posting was stored")
			}
			if _, ok := glintsByID(jobs)[glintsVNID]; !ok {
				t.Error("the good posting beside it was lost")
			}
		})
	}
}

func TestGlintsReadsRemoteAndHiddenSalaryFromTheStructuredFields(t *testing.T) {
	f := newGlintsFake(t)
	f.pages[glintsITURL] = glintsPageHTML(t, glintsITID, func(d map[string]any) {
		d["workArrangementOption"] = "REMOTE"
		d["shouldShowSalary"] = false
		d["type"] = "PROJECT_BASED"
	})
	jobs, err := newTestGlints(f).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	j := glintsByID(jobs)[glintsITID]
	if j.WorkMode != "remote" || !j.Remote || j.EmploymentType != "contract" {
		t.Errorf("WorkMode = %q Remote = %v EmploymentType = %q", j.WorkMode, j.Remote, j.EmploymentType)
	}
	if j.SalaryMin != nil || j.SalaryCurrency != "" {
		t.Errorf("a hidden salary was read: %v %q", j.SalaryMin, j.SalaryCurrency)
	}
}

func TestGlintsSalaryInAnUnknownModeIsLeftOut(t *testing.T) {
	f := newGlintsFake(t)
	f.pages[glintsITURL] = glintsPageHTML(t, glintsITID, func(d map[string]any) {
		d["salaries"] = []any{map[string]any{
			"salaryType": "BASIC", "salaryMode": "FORTNIGHT", "minAmount": 5, "maxAmount": 6, "CurrencyCode": "IDR",
		}}
	})
	jobs, err := newTestGlints(f).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	if j := glintsByID(jobs)[glintsITID]; j.SalaryMin != nil || j.SalaryPeriod != "" {
		t.Errorf("salary = %v %q, want none for a mode the table does not know", j.SalaryMin, j.SalaryPeriod)
	}
}

func TestGlintsRetriesAShardBeforeFailingTheCrawl(t *testing.T) {
	f := newGlintsFake(t)
	shard := "https://glints.com/sitemap_job_vn_1.xml"
	f.errs[shard] = errors.New("refused")
	if _, err := newTestGlints(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("a shard that stays unreadable must fail the crawl: the source-scoped sweep would close its postings")
	}
	if n := f.askedFor(shard); n != glintsShardAttempts {
		t.Errorf("shard asked for %d times, want %d", n, glintsShardAttempts)
	}
}

func TestGlintsFailsAListingWithNothingTechnicalOrAnEmptyShard(t *testing.T) {
	f := newGlintsFake(t)
	f.xmls["https://glints.com/sitemap_job_id_1.xml"] = glintsShard("https://glints.com/id/opportunities/jobs/kitchen/" + glintsOtherID)
	f.xmls["https://glints.com/sitemap_job_vn_1.xml"] = glintsShard("https://glints.com/vn/opportunities/jobs/waiter/" + glintsVNID)
	if _, err := newTestGlints(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Error("a listing with no technical posting must fail, not report a quiet source")
	}
	f.xmls["https://glints.com/sitemap_job_vn_1.xml"] = `<urlset></urlset>`
	if _, err := newTestGlints(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Error("a shard listing no posting is a challenge page, not an empty slice of the site")
	}
}

func TestGlintsFailsWhenItListedNewPostingsAndReadNone(t *testing.T) {
	f := newGlintsFake(t)
	f.pages = map[string]string{}
	if _, err := newTestGlints(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Error("every page unreadable must fail the crawl")
	}
}

func TestGlintsBoundsOneRunsHydration(t *testing.T) {
	f := newGlintsFake(t)
	g := newTestGlints(f)
	g.maxNewPerRun = 1
	jobs, err := g.Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Errorf("hydrated %d postings under a budget of 1", len(jobs))
	}
}
