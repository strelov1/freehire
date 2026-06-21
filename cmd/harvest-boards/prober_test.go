package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// fakeGetter decodes a canned body per URL into v; an unmapped URL is an error, standing
// in for the real client's response to a missing/moved board. It serves JSON (the API
// probers), POST-JSON (Workday's CXS listing), XML (the iCIMS/Deel sitemap probers), and
// HTML (the Freshteam listing prober), so it satisfies the wider httpClient.
type fakeGetter map[string]string

func (f fakeGetter) GetJSON(_ context.Context, url string, v any) error {
	body, ok := f[url]
	if !ok {
		return errMissing
	}
	return json.Unmarshal([]byte(body), v)
}

// PostJSON ignores the request body and returns the canned response for url, standing in
// for Workday's POST-only CXS listing.
func (f fakeGetter) PostJSON(_ context.Context, url string, _ any, v any) error {
	body, ok := f[url]
	if !ok {
		return errMissing
	}
	return json.Unmarshal([]byte(body), v)
}

func (f fakeGetter) GetXML(_ context.Context, url string, v any) error {
	body, ok := f[url]
	if !ok {
		return errMissing
	}
	return xml.Unmarshal([]byte(body), v)
}

// GetHTML parses the canned body for url as an HTML document, standing in for the real
// client's response to the Freshteam listing prober.
func (f fakeGetter) GetHTML(_ context.Context, url string) (*html.Node, error) {
	body, ok := f[url]
	if !ok {
		return nil, errMissing
	}
	return html.Parse(strings.NewReader(body))
}

func TestGreenhouseProbe(t *testing.T) {
	g := greenhouseProber{}
	getter := fakeGetter{
		"https://boards-api.greenhouse.io/v1/boards/acme/jobs":  `{"jobs":[{"id":1},{"id":2}]}`,
		"https://boards-api.greenhouse.io/v1/boards/acme":       `{"name":"Acme Inc"}`,
		"https://boards-api.greenhouse.io/v1/boards/empty/jobs": `{"jobs":[]}`,
		// A board whose jobs endpoint works but metadata lacks a name falls back to the slug.
		"https://boards-api.greenhouse.io/v1/boards/noname/jobs": `{"jobs":[{"id":7}]}`,
		"https://boards-api.greenhouse.io/v1/boards/noname":      `{}`,
	}

	cases := []struct {
		slug     string
		wantName string
		wantN    int
	}{
		{"acme", "Acme Inc", 2},
		{"empty", "", 0},
		{"noname", "noname", 1},
		{"gone", "", 0}, // absent from greenhouse (getter error) => skip, not failure
	}
	for _, tc := range cases {
		name, n, err := g.probe(context.Background(), getter, tc.slug)
		if err != nil {
			t.Errorf("%s: unexpected error %v", tc.slug, err)
		}
		if name != tc.wantName || n != tc.wantN {
			t.Errorf("%s: got (%q,%d), want (%q,%d)", tc.slug, name, n, tc.wantName, tc.wantN)
		}
	}
}

func TestWorkdayProbe(t *testing.T) {
	p := workdayProber{}
	getter := fakeGetter{
		"https://aig.wd1.myworkdayjobs.com/wday/cxs/aig/early_careers/jobs": `{"total":9,"jobPostings":[{"title":"x"}]}`,
		"https://acme.wd5.myworkdayjobs.com/wday/cxs/acme/empty/jobs":       `{"total":0,"jobPostings":[]}`,
	}
	// live: name falls back to tenant, count = total
	if name, n, err := p.probe(context.Background(), getter, "aig.wd1.myworkdayjobs.com/early_careers"); err != nil || name != "aig" || n != 9 {
		t.Errorf("live: got (%q,%d,%v), want (aig,9,nil)", name, n, err)
	}
	// empty board => skip
	if name, n, err := p.probe(context.Background(), getter, "acme.wd5.myworkdayjobs.com/empty"); err != nil || name != "" || n != 0 {
		t.Errorf("empty: got (%q,%d,%v), want (\"\",0,nil)", name, n, err)
	}
	// absent (getter error) => skip
	if name, n, err := p.probe(context.Background(), getter, "gone.wd1.myworkdayjobs.com/site"); err != nil || name != "" || n != 0 {
		t.Errorf("gone: got (%q,%d,%v), want (\"\",0,nil)", name, n, err)
	}
	// malformed board id => skip
	if _, n, err := p.probe(context.Background(), getter, "no-slash"); err != nil || n != 0 {
		t.Errorf("malformed: got (%d,%v), want (0,nil)", n, err)
	}
}

// icimsSitemap builds an iCIMS sitemap urlset from the given locs, for prober tests.
func icimsSitemap(locs ...string) string {
	s := `<?xml version="1.0" encoding="utf-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`
	for _, l := range locs {
		s += `<url><loc>` + l + `</loc></url>`
	}
	return s + `</urlset>`
}

// TestICIMSProbe: the iCIMS prober validates a slug by counting job postings in its
// sitemap. A sitemap with ≥1 /jobs/<id>/ loc is a live board (name falls back to slug);
// a sitemap with only the non-posting search/intro entries, or an absent sitemap (404),
// is a ("",0,nil) skip — covering both observed dead shapes (HTTP 404, and HTTP 200 with
// zero jobs).
func TestICIMSProbe(t *testing.T) {
	p := icimsProber{}
	getter := fakeGetter{
		"https://careers-acme.icims.com/sitemap.xml": icimsSitemap(
			"https://careers-acme.icims.com/jobs/search",
			"https://careers-acme.icims.com/jobs/intro",
			"https://careers-acme.icims.com/jobs/101/role-a/job",
			"https://careers-acme.icims.com/jobs/102/role-b/job",
		),
		// 200 but only non-posting entries => zero jobs => skip.
		"https://careers-empty.icims.com/sitemap.xml": icimsSitemap(
			"https://careers-empty.icims.com/jobs/search",
		),
	}

	// Live board: name == slug, jobs > 0.
	if name, n, err := p.probe(context.Background(), getter, "acme"); err != nil || name != "acme" || n != 2 {
		t.Errorf("acme: got (%q,%d,%v), want (acme,2,nil)", name, n, err)
	}
	// 200-with-zero-jobs => skip.
	if name, n, err := p.probe(context.Background(), getter, "empty"); err != nil || name != "" || n != 0 {
		t.Errorf("empty: got (%q,%d,%v), want (\"\",0,nil)", name, n, err)
	}
	// Absent sitemap (404 / getter error) => skip.
	if name, n, err := p.probe(context.Background(), getter, "gone"); err != nil || name != "" || n != 0 {
		t.Errorf("gone: got (%q,%d,%v), want (\"\",0,nil)", name, n, err)
	}
}

// The lever/ashby/bamboohr provers carry no company name in their payloads, so a live
// board's name falls back to its slug; an empty or absent board is a ("",0,nil) skip.
func TestSlugFallbackProbers(t *testing.T) {
	cases := []struct {
		name   string
		p      prober
		getter fakeGetter
		live   string // a slug that returns jobs
		empty  string // a slug that returns an empty board
	}{
		{
			name: "lever",
			p:    leverProber{},
			getter: fakeGetter{
				"https://api.lever.co/v0/postings/acme?mode=json":  `[{"id":"a"},{"id":"b"},{"id":"c"}]`,
				"https://api.lever.co/v0/postings/empty?mode=json": `[]`,
			},
			live: "acme", empty: "empty",
		},
		{
			name: "ashby",
			p:    ashbyProber{},
			getter: fakeGetter{
				"https://api.ashbyhq.com/posting-api/job-board/acme":  `{"jobs":[{"id":"a"},{"id":"b"}]}`,
				"https://api.ashbyhq.com/posting-api/job-board/empty": `{"jobs":[]}`,
			},
			live: "acme", empty: "empty",
		},
		{
			name: "bamboohr",
			p:    bamboohrProber{},
			getter: fakeGetter{
				"https://acme.bamboohr.com/careers/list":  `{"result":[{"id":"1"}]}`,
				"https://empty.bamboohr.com/careers/list": `{"result":[]}`,
			},
			live: "acme", empty: "empty",
		},
		{
			name: "recruitee",
			p:    recruiteeProber{},
			getter: fakeGetter{
				"https://acme.recruitee.com/api/offers/":  `{"offers":[{"id":1},{"id":2}]}`,
				"https://empty.recruitee.com/api/offers/": `{"offers":[]}`,
			},
			live: "acme", empty: "empty",
		},
		{
			name: "pinpoint",
			p:    pinpointProber{},
			getter: fakeGetter{
				"https://acme.pinpointhq.com/postings.json":  `{"data":[{"id":"1"}]}`,
				"https://empty.pinpointhq.com/postings.json": `{"data":[]}`,
			},
			live: "acme", empty: "empty",
		},
		{
			name: "breezy",
			p:    breezyProber{},
			getter: fakeGetter{
				"https://acme.breezy.hr/json":  `[{"id":"a"},{"id":"b"}]`,
				"https://empty.breezy.hr/json": `[]`,
			},
			live: "acme", empty: "empty",
		},
		{
			name: "trakstar",
			p:    trakstarProber{},
			getter: fakeGetter{
				"https://acme.hire.trakstar.com/jobfeeds/acme":   `<rss><channel><item><title>Eng</title></item></channel></rss>`,
				"https://empty.hire.trakstar.com/jobfeeds/empty": `<rss><channel></channel></rss>`,
			},
			live: "acme", empty: "empty",
		},
		{
			name: "personio",
			p:    personioProber{},
			getter: fakeGetter{
				"https://acme.jobs.personio.com/xml":  `<workzag-jobs><position><id>1</id></position></workzag-jobs>`,
				"https://empty.jobs.personio.com/xml": `<workzag-jobs></workzag-jobs>`,
			},
			live: "acme", empty: "empty",
		},
		{
			name: "deel",
			p:    deelProber{},
			getter: fakeGetter{
				"https://jobs.deel.com/acme/sitemap.xml":  `<urlset><url><loc>https://jobs.deel.com/acme/job-details/1</loc></url></urlset>`,
				"https://jobs.deel.com/empty/sitemap.xml": `<urlset><url><loc>https://jobs.deel.com/empty</loc></url></urlset>`,
			},
			live: "acme", empty: "empty",
		},
		{
			name: "freshteam",
			p:    freshteamProber{},
			getter: fakeGetter{
				"https://acme.freshteam.com/jobs":  `<html><body><a href="/jobs/abcdefghijkl/engineer">Engineer</a></body></html>`,
				"https://empty.freshteam.com/jobs": `<html><body><a href="/jobs/search">Search</a></body></html>`,
			},
			live: "acme", empty: "empty",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Live board: name == slug, jobs > 0.
			name, n, err := tc.p.probe(context.Background(), tc.getter, tc.live)
			if err != nil || name != tc.live || n == 0 {
				t.Errorf("live: got (%q,%d,%v), want (%q,>0,nil)", name, n, err, tc.live)
			}
			// Empty board.
			if name, n, err := tc.p.probe(context.Background(), tc.getter, tc.empty); err != nil || name != "" || n != 0 {
				t.Errorf("empty: got (%q,%d,%v), want (\"\",0,nil)", name, n, err)
			}
			// Absent board (getter error) => skip.
			if name, n, err := tc.p.probe(context.Background(), tc.getter, "gone"); err != nil || name != "" || n != 0 {
				t.Errorf("gone: got (%q,%d,%v), want (\"\",0,nil)", name, n, err)
			}
		})
	}
}

// Gem posts every board to one GraphQL URL, so liveness turns on the response's jobPostings,
// not the URL. The two cases use distinct getters rather than distinct slugs.
func TestGemProbe(t *testing.T) {
	p := gemProber{}
	live := fakeGetter{"https://jobs.gem.com/api/public/graphql": `{"data":{"oatsExternalJobPostings":{"jobPostings":[{"extId":"x"},{"extId":"y"}]}}}`}
	if name, n, err := p.probe(context.Background(), live, "acme"); err != nil || name != "acme" || n != 2 {
		t.Errorf("live: got (%q,%d,%v), want (\"acme\",2,nil)", name, n, err)
	}
	empty := fakeGetter{"https://jobs.gem.com/api/public/graphql": `{"data":{"oatsExternalJobPostings":{"jobPostings":[]}}}`}
	if name, n, err := p.probe(context.Background(), empty, "acme"); err != nil || name != "" || n != 0 {
		t.Errorf("empty: got (%q,%d,%v), want (\"\",0,nil)", name, n, err)
	}
	// A board the API rejects (getter error) => skip, not failure.
	if name, n, err := p.probe(context.Background(), fakeGetter{}, "gone"); err != nil || name != "" || n != 0 {
		t.Errorf("gone: got (%q,%d,%v), want (\"\",0,nil)", name, n, err)
	}
}

// The workable/smartrecruiters/teamtailor probers carry a company name in their payload, so
// a live board reports that name (not the slug); an empty or absent board is a ("",0,nil) skip.
func TestNamedProbers(t *testing.T) {
	cases := []struct {
		name     string
		p        prober
		getter   fakeGetter
		live     string
		wantName string
		empty    string
	}{
		{
			name: "workable",
			p:    workableProber{},
			getter: fakeGetter{
				"https://apply.workable.com/api/v1/widget/accounts/acme?details=true":  `{"name":"Acme Inc","jobs":[{"shortcode":"AB"}]}`,
				"https://apply.workable.com/api/v1/widget/accounts/empty?details=true": `{"name":"Empty","jobs":[]}`,
			},
			live: "acme", wantName: "Acme Inc", empty: "empty",
		},
		{
			name: "smartrecruiters",
			p:    smartRecruitersProber{},
			getter: fakeGetter{
				"https://api.smartrecruiters.com/v1/companies/acme/postings?limit=1":  `{"totalFound":42,"content":[{"id":"1"}]}`,
				"https://api.smartrecruiters.com/v1/companies/acme":                   `{"name":"Acme Inc"}`,
				"https://api.smartrecruiters.com/v1/companies/empty/postings?limit=1": `{"totalFound":0,"content":[]}`,
			},
			live: "acme", wantName: "Acme Inc", empty: "empty",
		},
		{
			name: "teamtailor",
			p:    teamtailorProber{},
			getter: fakeGetter{
				"https://jobs.acme.com/jobs?page=1":  `{"title":"Acme","items":[{"id":"1"},{"id":"2"}]}`,
				"https://jobs.empty.com/jobs?page=1": `{"title":"Empty","items":[]}`,
			},
			live: "jobs.acme.com", wantName: "Acme", empty: "jobs.empty.com",
		},
		{
			name: "join",
			p:    joinProber{},
			getter: fakeGetter{
				"https://join.com/api/public/companies/100/jobs?page=1&pageSize=1": `{"pagination":{"rowCount":7}}`,
				"https://join.com/api/public/companies/100":                        `{"name":"Acme Inc"}`,
				"https://join.com/api/public/companies/200/jobs?page=1&pageSize=1": `{"pagination":{"rowCount":0}}`,
			},
			live: "100", wantName: "Acme Inc", empty: "200",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name, n, err := tc.p.probe(context.Background(), tc.getter, tc.live)
			if err != nil || name != tc.wantName || n == 0 {
				t.Errorf("live: got (%q,%d,%v), want (%q,>0,nil)", name, n, err, tc.wantName)
			}
			if name, n, err := tc.p.probe(context.Background(), tc.getter, tc.empty); err != nil || name != "" || n != 0 {
				t.Errorf("empty: got (%q,%d,%v), want (\"\",0,nil)", name, n, err)
			}
			if name, n, err := tc.p.probe(context.Background(), tc.getter, "gone"); err != nil || name != "" || n != 0 {
				t.Errorf("gone: got (%q,%d,%v), want (\"\",0,nil)", name, n, err)
			}
		})
	}
}
