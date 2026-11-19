package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"golang.org/x/net/html"
	"golang.org/x/time/rate"
)

// hubstaffCards mirrors the live card markup: an hourly posting, a fixed-price one, a full-time
// one with no pay, and a card without a client (dropped).
const hubstaffCards = `<div class="search-result"><div class="main-details"><a class="name margin-right-10" rel="nofollow" href="/jobs/seo-writer-aa11">SEO Writer</a><span class="label label-hourly">hourly</span><div class="pay-rate">$8/hr</div><div class="job-company"><a class="is-inline-block job-agency" rel="nofollow" target="_blank" href="http://acme.example"><i class="hi hi-agency"></i> Acme LLC</a><span class="location text-success"><i class="hi hi-pin"></i> <strong>HQ:</strong> Billings, Montana, United States</span><span><i class="hi hi-remote hi-16"></i> Remote job</span><span class="is-inline-block"><i class="hi hi-calendar"></i> <span class="a-tooltip" data-original-title="November 18, 2026 at  9:45 pm">4 hrs ago</span></span></div></div></div>` +
	`<div class="search-result"><div class="main-details"><a class="name" href="/jobs/logo-design-bb22">Logo &amp; Brand Design</a><span class="label label-fixed_price">fixed price</span><div class="fixed-price">$500</div><div class="job-company"><a class="job-agency" href="javascript:void(0)"><i class="hi hi-agency"></i> Globex</a><span class="location"><i class="hi hi-pin"></i> <strong>HQ:</strong> London, England, United Kingdom</span><span><i class="hi hi-remote"></i> Remote job</span></div></div></div>` +
	`<div class="search-result"><div class="main-details"><a class="name" href="/jobs/support-cc33">Support Agent</a><span class="label label-full_time">full time</span><div class="pay-rate"></div><div class="job-company"><a class="job-agency" href="https://initech.example/"><i class="hi hi-agency"></i> Initech</a><span><i class="hi hi-remote"></i> Remote job</span></div></div></div>` +
	`<div class="search-result"><div class="main-details"><a class="name" href="/jobs/no-client-dd44">No Client</a></div></div>`

// hubstaffAnswer wraps markup the way the site does: a JS call whose argument is a string literal.
func hubstaffAnswer(markup string) string {
	lit, _ := json.Marshal(strings.ReplaceAll(markup, "/", `\/`)) // JS answers escape slashes
	return `$('#results').html(` + strings.ReplaceAll(string(lit), `\\/`, `\/`) + `);` + "\n"
}

type hubstaffFake struct {
	pages   map[int]string // page number -> answer; absent pages answer an empty result
	details map[string]string
	failing map[string]bool
	detailN atomic.Int64
}

func (f *hubstaffFake) GetTextWithHeaders(_ context.Context, u string, h map[string]string) (string, error) {
	if h["X-Requested-With"] != "XMLHttpRequest" {
		return "", fmt.Errorf("listing asked without the XHR header")
	}
	var page int
	if _, err := fmt.Sscanf(u, hubstafftalentBase+"/search/jobs?page=%d", &page); err != nil {
		return "", fmt.Errorf("unexpected url %s", u)
	}
	if a, ok := f.pages[page]; ok {
		return a, nil
	}
	return hubstaffAnswer(`<h5>Your search did not match any jobs</h5>`), nil
}

func (f *hubstaffFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	f.detailN.Add(1)
	slug := strings.TrimPrefix(u, hubstafftalentBase+"/jobs/")
	if f.failing[slug] {
		return nil, fmt.Errorf("refused")
	}
	return html.Parse(strings.NewReader(f.details[slug]))
}

func hubstaffSource(f *hubstaffFake, maxNew int) Source {
	return hubstafftalent{http: f, limiter: rate.NewLimiter(rate.Inf, 1), maxNewPerRun: maxNew}
}

func hubstaffDetail(body string) string {
	return `<html><body><div class="job-description push-bottom-30">` + body + `</div></body></html>`
}

func TestHubstaffTalentProviderAndMarkers(t *testing.T) {
	s := NewHubstaffTalent(nil)
	if s.Provider() != "hubstafftalent" {
		t.Errorf("Provider() = %q", s.Provider())
	}
	if _, ok := s.(boardless); !ok {
		t.Error("hubstafftalent should be boardless")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("hubstafftalent should be an aggregator")
	}
	if _, ok := s.(HydratingSource); !ok {
		t.Error("hubstafftalent should be a HydratingSource")
	}
	if _, ok := All(nil)["hubstafftalent"]; !ok {
		t.Error("All() should register hubstafftalent")
	}
	if !slices.Contains(FilterableProviders(), "hubstafftalent") {
		t.Error("FilterableProviders() should include hubstafftalent")
	}
}

func TestHubstaffTalentFetchMapsCardsAndBodies(t *testing.T) {
	f := &hubstaffFake{
		pages: map[int]string{1: hubstaffAnswer(hubstaffCards)},
		details: map[string]string{
			"seo-writer-aa11":  hubstaffDetail("<p>Write SEO</p>"),
			"logo-design-bb22": hubstaffDetail("<p>Design</p>"),
			"support-cc33":     hubstaffDetail("<p>Help</p>"),
		},
	}
	jobs, err := hubstaffSource(f, 10).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3 (the card without a client is dropped)", len(jobs))
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	a := byID["seo-writer-aa11"]
	if a.URL != "https://hubstafftalent.net/jobs/seo-writer-aa11" || a.Title != "SEO Writer" || a.Company != "Acme LLC" ||
		a.CompanyWebsite != "http://acme.example" || a.Location != "Billings, Montana, United States" ||
		!a.Remote || a.WorkMode != "remote" || a.EmploymentType != "contract" ||
		a.SalaryPeriod != "hour" || a.SalaryCurrency != "USD" || a.SalaryMin == nil || *a.SalaryMin != 8 || *a.SalaryMax != 8 ||
		!strings.Contains(a.Description, "Write SEO") {
		t.Errorf("hourly card mapped wrong: %+v", a)
	}
	if a.PostedAt == nil || a.PostedAt.Format("2006-01-02 15:04") != "2026-11-18 21:45" {
		t.Errorf("PostedAt = %v, want 2026-11-18 21:45 UTC", a.PostedAt)
	}
	b := byID["logo-design-bb22"]
	if b.Title != "Logo & Brand Design" || b.EmploymentType != "contract" || b.SalaryMin != nil || b.CompanyWebsite != "" {
		t.Errorf("fixed-price card must carry no salary or website: %+v", b)
	}
	if c := byID["support-cc33"]; c.EmploymentType != "full_time" || c.SalaryMin != nil || c.CompanyWebsite != "https://initech.example/" {
		t.Errorf("full-time card mapped wrong: %+v", c)
	}
}

func TestHubstaffTalentSeenIsRefreshedWithoutDetail(t *testing.T) {
	f := &hubstaffFake{
		pages:   map[int]string{1: hubstaffAnswer(hubstaffCards)},
		details: map[string]string{"logo-design-bb22": hubstaffDetail("<p>Design</p>"), "support-cc33": hubstaffDetail("<p>Help</p>")},
	}
	jobs, err := hubstaffSource(f, 10).(HydratingSource).FetchNew(context.Background(), CompanyEntry{},
		func(id string) bool { return id == "seo-writer-aa11" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if n := f.detailN.Load(); n != 2 {
		t.Errorf("detail requests = %d, want 2 (the stored posting costs none)", n)
	}
	for _, j := range jobs {
		if (j.ExternalID == "seo-writer-aa11") != j.SeenRefresh {
			t.Errorf("%s SeenRefresh = %v", j.ExternalID, j.SeenRefresh)
		}
	}
}

func TestHubstaffTalentBudgetAndUnreadableBody(t *testing.T) {
	f := &hubstaffFake{
		pages:   map[int]string{1: hubstaffAnswer(hubstaffCards)},
		details: map[string]string{"seo-writer-aa11": hubstaffDetail("<p>Write SEO</p>")},
		failing: map[string]bool{"logo-design-bb22": true},
	}
	// Budget of 2: the third unseen posting is deferred, the second one's page is refused.
	jobs, err := hubstaffSource(f, 2).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "seo-writer-aa11" {
		t.Errorf("got %+v, want only the readable posting", jobs)
	}
	// Every body refused is a broken crawl, not a quiet one.
	f.failing["seo-writer-aa11"] = true
	if _, err := hubstaffSource(f, 2).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Error("a crawl that read no body must fail")
	}
}

func TestHubstaffTalentListingWalksPagesAndFailsLoudly(t *testing.T) {
	two := `<div class="search-result"><a class="name" href="/jobs/p2-ee55">P2</a><a class="job-agency" href="#">Hooli</a></div>`
	f := &hubstaffFake{
		pages:   map[int]string{1: hubstaffAnswer(hubstaffCards), 2: hubstaffAnswer(two)},
		details: map[string]string{},
	}
	jobs, err := hubstaffSource(f, 0).Fetch(context.Background(), CompanyEntry{})
	if err != nil || len(jobs) != 0 {
		t.Fatalf("budget 0 yields no job: %v %+v", err, jobs)
	}
	cards, err := hubstaffSource(f, 0).(hubstafftalent).list(context.Background())
	if err != nil || len(cards) != 4 {
		t.Errorf("list = %d cards, err %v; want 4 across two pages", len(cards), err)
	}
	if _, err := hubstaffSource(&hubstaffFake{}, 1).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Error("an empty first page is a moved layout, not an empty marketplace")
	}
}

func TestHubstaffTalentListingWalksPastAnUnmappablePage(t *testing.T) {
	noClient := `<div class="search-result"><div class="main-details"><a class="name" href="/jobs/no-client-dd44">No Client</a></div></div>`
	later := `<div class="search-result"><div class="main-details"><a class="name" href="/jobs/qa-ee55">QA Tester</a><div class="job-company"><a class="job-agency" href="#">Umbrella</a></div></div></div>`
	f := &hubstaffFake{pages: map[int]string{1: hubstaffAnswer(hubstaffCards), 2: hubstaffAnswer(noClient), 3: hubstaffAnswer(later)}}
	jobs, err := hubstaffSource(f, 0).(hubstafftalent).list(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(jobs) != 4 || jobs[3].ExternalID != "qa-ee55" {
		t.Fatalf("got %d listed postings, want page 3's posting read after the unmappable page 2: %+v", len(jobs), jobs)
	}
}
