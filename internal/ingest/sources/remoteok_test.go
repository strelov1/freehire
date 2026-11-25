package sources

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestRemoteOKProvider(t *testing.T) {
	if got := NewRemoteOK(nil).Provider(); got != "remoteok" {
		t.Errorf("Provider() = %q, want remoteok", got)
	}
}

func TestRemoteOKIsBoardlessAggregator(t *testing.T) {
	s := NewRemoteOK(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("remoteok should implement the boardless marker")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("remoteok should implement the aggregator marker")
	}
}

func TestRemoteOKRegisteredAndFilterable(t *testing.T) {
	if _, ok := All(nil)["remoteok"]; !ok {
		t.Error("All() should register provider remoteok")
	}
	if !slices.Contains(FilterableProviders(), "remoteok") {
		t.Error("FilterableProviders() should include remoteok")
	}
}

// remoteokTestRow mirrors the site's listing markup: a job row and its "expand" row, which
// holds the body in div.markdown with the newlines written as the two characters \n.
func remoteokTestRow(id, slug, company, title, locations, body string) string {
	bodyDiv := `<div class="markdown">` + body + `</div>`
	if strings.HasPrefix(body, "<") {
		bodyDiv = `<div class="html">` + body + `</div>`
	}
	return `<tr  data-offset="1" data-slug="` + slug + `" data-company="` + company + `" id="job-` + id + `" data-id="` + id + `" data-href="/remote-jobs/` + slug + `" class="job job-` + id + `">
<td class="company position company_and_position"><a href="/remote-jobs/` + slug + `"><h2  itemprop="title">
` + title + `
</h2></a><h3 itemprop="name">` + company + `</h3>` + locations + `</td>
<td class="time"><time datetime="2026-11-20T08:20:14+00:00">5d</time></td></tr>
<tr class="expand expand-` + id + ` active " data-id="` + id + `"><td colspan="999"><div class="description" itemprop="description">
<div class="company_profile"><a href="/company/x">noise</a></div>
` + bodyDiv + `</div></td></tr>
`
}

const remoteokTestAPI = `[
{"legal":"API Terms: link back"},
{"id":"1138077","slug":"tax-real-remax-1138077","company":"Real REMAX Group","position":"Senior Tax Associate","description":"<p>SchÃ¶n, youâ\u0080\u0099ll file.</p>","location":"MÃ¼nchen","date":"2026-11-24T00:00:13+00:00","url":"https://remoteOK.com/remote-jobs/tax-real-remax-1138077","salary_min":118000,"salary_max":150000},
{"id":"1138074","slug":"advisor-thrive-1138074","company":"Thrive","position":"Strategic Advisor","description":"<p>Advise.</p>","location":"","date":"2026-11-24T00:10:00+00:00","url":"https://remoteOK.com/remote-jobs/advisor-thrive-1138074","salary_min":0,"salary_max":0}
]`

func remoteokTestPages() (page0, page1 string) {
	page0 = `` +
		remoteokTestRow("1138077", "tax-real-remax-1138077", "Real REMAX Group", "Senior Tax Associate", `<div class="location tooltip">Newark</div><div class="location">💰 Upgrade to Premium to see salary</div>`, `plain`) +
		remoteokTestRow("1137686", "reviewer-1137686", "Acme", "Reviewer", `<div class="location tooltip"><a href="/x">🇺🇸 United States</a></div>`, `<h3>About</h3><p>Own <strong>HTML</strong> body.</p>`) +
		remoteokTestRow("1138074", "advisor-thrive-1138074", "Thrive", "Strategic Advisor", `<div class="location tooltip"><a href="/x">🌏 Probably worldwide</a></div><div class="location">⏰ Contractor</div>`, `plain`)
	page1 = `` +
		remoteokTestRow("1137451", "ese-toptal-1137451", "Toptal &amp; Co", "Enterprise SDR", `<div class="location tooltip"><a href="/x">🇺🇸 United States</a></div><div class="location tooltip"><a href="/y">🇨🇦 Canada</a></div><div class="location">💰 $80k - $160k</div>`,
			`\nAbout Toptal\n\nWe &lt;build&gt; things.\nSecond line.`)
	return
}

func remoteokTestClient() *routedHTTP {
	p0, p1 := remoteokTestPages()
	return (&routedHTTP{}).
		route("remoteok.com/api", remoteokTestAPI).
		route("offset=0", p0).route("offset=50", p1).route("offset=100", ``)
}

func TestRemoteOKFetchWalksListingAndPrefersAPI(t *testing.T) {
	jobs, err := NewRemoteOK(remoteokTestClient()).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 4 {
		t.Fatalf("got %d jobs, want 4 (2 from the API, 2 only in the listing; legal notice dropped)", len(jobs))
	}
	by := map[string]Job{}
	for _, j := range jobs {
		by[j.ExternalID] = j
	}

	api := by["1138077"]
	if api.Location != "München" || !strings.Contains(api.Description, "Schön, you’ll file.") {
		t.Errorf("API mojibake not repaired: loc=%q desc=%q", api.Location, api.Description)
	}
	if api.SalaryMin == nil || *api.SalaryMin != 118000 || *api.SalaryMax != 150000 || api.SalaryCurrency != "USD" || api.SalaryPeriod != "year" {
		t.Errorf("salary not mapped: %+v", api)
	}
	if api.URL != "https://remoteOK.com/remote-jobs/tax-real-remax-1138077" {
		t.Errorf("URL = %q", api.URL)
	}

	empty := by["1138074"]
	if empty.Location != "Worldwide" || empty.EmploymentType != "contract" || empty.SalaryMin != nil {
		t.Errorf("empty API location / listing facets: %+v", empty)
	}

	old := by["1137451"]
	if old.Company != "Toptal & Co" || old.Title != "Enterprise SDR" || old.Location != "United States, Canada" {
		t.Errorf("listing-only mapping: %+v", old)
	}
	if old.URL != "https://remoteOK.com/remote-jobs/ese-toptal-1137451" || old.WorkMode != "remote" || old.PostedAt == nil {
		t.Errorf("listing-only URL/mode/date: %+v", old)
	}
	if want := "<p>About Toptal</p><p>We &lt;build&gt; things.<br>Second line.</p>"; old.Description != want {
		t.Errorf("Description = %q, want %q", old.Description, want)
	}
	if got := by["1137686"].Description; got != "<h3>About</h3><p>Own <strong>HTML</strong> body.</p>" {
		t.Errorf("div.html body = %q", got)
	}
	if old.SalaryMin != nil {
		t.Error("the listing's placeholder salary must not be read as a salary")
	}
}

func TestRemoteOKFetchNewKeepsSeenListingOnlyBodies(t *testing.T) {
	seen := func(id string) bool { return id == "1137451" || id == "1138077" }
	jobs, err := NewRemoteOK(remoteokTestClient()).(HydratingSource).FetchNew(context.Background(), CompanyEntry{}, seen)
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	for _, j := range jobs {
		switch j.ExternalID {
		case "1137451":
			if !j.SeenRefresh || j.Description != "" {
				t.Errorf("seen listing-only posting must be a bare refresh: %+v", j)
			}
		case "1138077":
			if j.SeenRefresh || j.Description == "" {
				t.Errorf("an API posting is always refreshed in full: %+v", j)
			}
		}
	}
}

func TestRemoteOKFailedListingPageFailsTheCrawl(t *testing.T) {
	p0, _ := remoteokTestPages()
	fake := (&routedHTTP{}).route("remoteok.com/api", remoteokTestAPI).route("offset=0", p0).
		routeErr("offset=50", errors.New("boom"))
	if _, err := NewRemoteOK(fake).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("a failed listing page must fail the crawl, not return a partial board")
	}
}
