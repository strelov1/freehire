package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"testing"
)

// fakeGetter decodes a canned body per URL into v; an unmapped URL is an error, standing
// in for the real client's response to a missing/moved board. It serves JSON (the API
// probers), POST-JSON (Workday's CXS listing), and XML (the iCIMS sitemap prober), so it
// satisfies the wider httpClient.
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
