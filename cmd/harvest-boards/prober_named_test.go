package main

import (
	"context"
	"testing"
)

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
			// Teamtailor serves its listing as HTML on every board, vendor sub-domain and
			// employer domain alike — there is no JSON feed to ask. Live: a /jobs/<id>
			// permalink; the employer is the page's og:site_name. Empty: only the nav anchors,
			// which must not be counted.
			name: "teamtailor",
			p:    teamtailorProber{},
			getter: fakeGetter{
				"https://careers.acme.com/jobs?page=1": `<html><head><meta property="og:site_name" content="Acme Inc"></head>` +
					`<body><a href="/jobs/1234-senior-go">Senior Go</a></body></html>`,
				"https://careers.empty.com/jobs?page=1": `<html><head><meta property="og:site_name" content="Empty"></head>` +
					`<body><a href="/jobs">All jobs</a><a href="/departments">Departments</a></body></html>`,
			},
			live: "careers.acme.com", wantName: "Acme Inc", empty: "careers.empty.com",
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
		{
			// The channel title is the tenant's own name, trailing space and all.
			name: "gethired",
			p:    gethiredProber{},
			getter: fakeGetter{
				"https://acme.easyapply.co/rss":  `<rss><channel><title>Acme Inc </title><item><title>Eng</title></item></channel></rss>`,
				"https://empty.easyapply.co/rss": `<rss><channel><title>Empty</title></channel></rss>`,
			},
			live: "acme", wantName: "Acme Inc", empty: "empty",
		},
		{
			// The channel title is "<employer> Job Feed"; the suffix is the platform's, not the name.
			name: "taleobe",
			p:    taleobeProber{},
			getter: fakeGetter{
				"https://tbe.taleo.net/dispatcher/servlet/DispatcherServlet?act=redirectOrg&org=ADELPHI&redirectUrl=https%3A%2F%2Ftbe.taleo.net%2Fats%2Fservlet%2FRss%3Fcws%3D43%26org%3DADELPHI":     `<rss xmlns:taleo="urn:TBERss"><channel><title>Adelphi University Job Feed</title><item><title>Eng</title></item></channel></rss>`,
				"https://tbe.taleo.net/dispatcher/servlet/DispatcherServlet?act=redirectOrg&org=SIGSOURCE&redirectUrl=https%3A%2F%2Ftbe.taleo.net%2Fats%2Fservlet%2FRss%3Fcws%3D37%26org%3DSIGSOURCE": `<rss><channel><title>Signature Source Job Feed</title></channel></rss>`,
			},
			live: "ADELPHI:43", wantName: "Adelphi University", empty: "SIGSOURCE:37",
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
