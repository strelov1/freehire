package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// errMissing is the sentinel the fake getter returns for an unmapped URL. In production the
// real client returns its own transport error for a missing board, treated identically.
var errMissing = errors.New("harvest: not found")

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

// GetText serves the canned body as raw text (the Paycom prober reads the session JWT out
// of the portal page).
func (f fakeGetter) GetText(_ context.Context, url string) (string, error) {
	body, ok := f[url]
	if !ok {
		return "", errMissing
	}
	return body, nil
}

func (f fakeGetter) GetJSONWithHeaders(_ context.Context, url string, _ map[string]string, v any) error {
	return f.GetJSON(context.Background(), url, v)
}

func (f fakeGetter) PostJSONWithHeaders(_ context.Context, url string, _ map[string]string, body, v any) error {
	return f.PostJSON(context.Background(), url, body, v)
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
	// live: the tenant is a token derived from the board id, not a name Workday published,
	// so the prober reports none; count = total
	if name, n, err := p.probe(context.Background(), getter, "aig.wd1.myworkdayjobs.com/early_careers"); err != nil || name != "" || n != 9 {
		t.Errorf("live: got (%q,%d,%v), want (\"\",9,nil)", name, n, err)
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

	// Live board: the sitemap carries no company name, so none is reported; jobs > 0.
	if name, n, err := p.probe(context.Background(), getter, "acme"); err != nil || name != "" || n != 2 {
		t.Errorf("acme: got (%q,%d,%v), want (\"\",2,nil)", name, n, err)
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

// These platforms publish no employer name in their payloads, so a live board reports "" as
// its name and takes its label from the seed; an empty or absent board is a ("",0,nil) skip.
// The empty name is the contract the corroboration gate relies on: a derived token here
// (a slug, a tenant, a host) would be indistinguishable from a name the platform published,
// and would gate the board against something the employer never chose.
func TestNamelessProbers(t *testing.T) {
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
			name: "pinpoint",
			p:    pinpointProber{},
			getter: fakeGetter{
				"https://acme.pinpointhq.com/postings.json":  `{"data":[{"id":"1"}]}`,
				"https://empty.pinpointhq.com/postings.json": `{"data":[]}`,
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
		{
			name: "teamtailor",
			p:    teamtailorProber{},
			getter: fakeGetter{
				// Teamtailor serves its listing as HTML on every board, vendor sub-domain and
				// employer domain alike — there is no JSON feed to ask. Live: a /jobs/<id>
				// permalink. Empty: only the nav anchors, which must not be counted.
				"https://careers.acme.com/jobs?page=1": `<html><body><a href="/jobs/1234-senior-go">Senior Go</a></body></html>`,
				"https://careers.empty.com/jobs?page=1": `<html><body><a href="/jobs">All jobs</a>` +
					`<a href="/departments">Departments</a></body></html>`,
			},
			live: "careers.acme.com", empty: "careers.empty.com",
		},
		{
			name: "jobvite",
			p:    jobviteProber{},
			getter: fakeGetter{
				// Live: a /<slug>/job/<code> permalink. Empty: only the /jobs and /jobAlerts
				// nav anchors, which the job pattern must not count.
				"https://jobs.jobvite.com/acme/jobs":  `<html><body><a href="/acme/job/ojJpAfwL">Engineer</a></body></html>`,
				"https://jobs.jobvite.com/empty/jobs": `<html><body><a href="/empty/jobs">All</a><a href="/empty/jobAlerts">Alerts</a></body></html>`,
			},
			live: "acme", empty: "empty",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Live board: no name reported, jobs > 0.
			name, n, err := tc.p.probe(context.Background(), tc.getter, tc.live)
			if err != nil || name != "" || n == 0 {
				t.Errorf("live: got (%q,%d,%v), want (\"\",>0,nil)", name, n, err)
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
	// Gem publishes no employer name, so the prober reports none — the contract that lets
	// the corroboration gate tell "no name" from "a name that disagrees".
	if name, n, err := p.probe(context.Background(), live, "acme"); err != nil || name != "" || n != 2 {
		t.Errorf("live: got (%q,%d,%v), want (\"\",2,nil)", name, n, err)
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
			// Teamtailor used to be listed here, on the strength of a JSON feed that returned a
			// company name. The platform serves HTML on every board and no such feed exists, so
			// this case asserted a shape only the fake ever produced. It now lives with the
			// nameless probers, where it belongs.
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

// Recruitee and Breezy are the two platforms in this tool whose public list endpoint carries
// the employer's own name. Extracting it is what lets the corroboration gate fire for them at
// all, so the name — not just the liveness count — is the assertion that matters.
func TestNameReportingProbers(t *testing.T) {
	t.Run("recruitee", func(t *testing.T) {
		getter := fakeGetter{
			"https://11bitstudios.recruitee.com/api/offers/": `{"offers":[{"id":1,"company_name":"11 bit studios"},{"id":2,"company_name":"11 bit studios"}]}`,
			"https://empty.recruitee.com/api/offers/":        `{"offers":[]}`,
		}
		p := recruiteeProber{}
		if name, n, err := p.probe(context.Background(), getter, "11bitstudios"); err != nil || name != "11 bit studios" || n != 2 {
			t.Errorf("live: got (%q,%d,%v), want (\"11 bit studios\",2,nil)", name, n, err)
		}
		if name, n, err := p.probe(context.Background(), getter, "empty"); err != nil || name != "" || n != 0 {
			t.Errorf("empty: got (%q,%d,%v), want (\"\",0,nil)", name, n, err)
		}
	})
	t.Run("breezy", func(t *testing.T) {
		getter := fakeGetter{
			"https://accelone.breezy.hr/json": `[{"id":"a","company":{"name":"AccelOne"}},{"id":"b","company":{"name":"AccelOne"}}]`,
			"https://empty.breezy.hr/json":    `[]`,
		}
		p := breezyProber{}
		if name, n, err := p.probe(context.Background(), getter, "accelone"); err != nil || name != "AccelOne" || n != 2 {
			t.Errorf("live: got (%q,%d,%v), want (\"AccelOne\",2,nil)", name, n, err)
		}
		if name, n, err := p.probe(context.Background(), getter, "empty"); err != nil || name != "" || n != 0 {
			t.Errorf("empty: got (%q,%d,%v), want (\"\",0,nil)", name, n, err)
		}
	})
}

// TestApploiProberRejectsNonNumericSlug guards the fix for a confirmed live false-positive
// (see #1884): api.apploi.com/v1/jobs?employer=<slug> answers a non-empty, "live" jobs list
// even when slug isn't a real employer id — it silently ignores a value it can't parse. The
// canned response below WOULD read as a live board if the guard were missing and the request
// went out at all, so this test fails loudly if the numeric-slug check regresses.
func TestApploiProberRejectsNonNumericSlug(t *testing.T) {
	getter := fakeGetter{
		"https://api.apploi.com/v1/jobs?employer=101-edu&limit=100": `{"data":[{"published":true,"archived":false}]}`,
	}
	if _, n, err := (apploiProber{}).probe(context.Background(), getter, "101-edu"); err != nil || n != 0 {
		t.Errorf("got (%d,%v), want (0,nil) for a non-numeric slug", n, err)
	}
}

// TestApploiProberAcceptsNumericSlug confirms the guard doesn't also reject apploi's real
// board-id shape.
func TestApploiProberAcceptsNumericSlug(t *testing.T) {
	getter := fakeGetter{
		"https://api.apploi.com/v1/jobs?employer=55591&limit=100": `{"data":[{"published":true,"archived":false},{"published":false,"archived":true}]}`,
	}
	if _, n, err := (apploiProber{}).probe(context.Background(), getter, "55591"); err != nil || n != 1 {
		t.Errorf("got (%d,%v), want (1,nil) for a numeric slug", n, err)
	}
}
