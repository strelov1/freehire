package main

import (
	"context"
	"slices"
	"testing"
)

func TestWorkdayCandidateFromURL(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want string
		ok   bool
	}{
		{"site at the host root", "https://acehardware.wd1.myworkdayjobs.com/External",
			"acehardware.wd1.myworkdayjobs.com/External", true},
		{"locale segment is not the site", "https://carbonhealth.wd1.myworkdayjobs.com/en-US/Careers/job/Remote/Nurse_R-1",
			"carbonhealth.wd1.myworkdayjobs.com/Careers", true},
		{"three-digit data centre", "https://aaregional.wd503.myworkdayjobs.com/Search?q=x",
			"aaregional.wd503.myworkdayjobs.com/Search", true},
		{"host case is folded, site case is kept", "https://ACME.WD1.MyWorkdayJobs.com/External_Careers",
			"acme.wd1.myworkdayjobs.com/External_Careers", true},
		{"robots.txt is not a site", "https://3m.wd1.myworkdayjobs.com/robots.txt", "", false},
		{"a bare locale is not a site", "https://3m.wd1.myworkdayjobs.com/en", "", false},
		{"a locale with nothing after it is not a site", "https://3m.wd1.myworkdayjobs.com/en-US/", "", false},
		{"the CXS API path is not a site", "https://acme.wd1.myworkdayjobs.com/wday/cxs/acme/External/jobs", "", false},
		{"the host root is not a site", "https://acme.wd1.myworkdayjobs.com/", "", false},
		{"a host without a data centre label is not Workday", "https://www.myworkdayjobs.com/External", "", false},
		{"another platform's URL is not Workday", "https://boards.greenhouse.io/acme", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := workdayCandidate(tc.url)
			if ok != tc.ok || got != tc.want {
				t.Errorf("workdayCandidate(%q) = (%q, %v), want (%q, %v)", tc.url, got, ok, tc.want, tc.ok)
			}
		})
	}
}

// Workday's boards are spread over a subdomain per tenant, so its CDX query is a wildcard
// over the whole domain rather than one host with a path glob. The glob is what makes the
// difference visible: "*.myworkdayjobs.com/*" is the query the index answers 502 to, and this
// test pins the working spelling so a future edit cannot quietly reintroduce it.
func TestWorkdayProberDiscoversFromCommonCrawl(t *testing.T) {
	f := fakeGetter{
		"https://index.commoncrawl.org/collinfo.json": commonCrawlCollInfoBody,
		"https://index.commoncrawl.org/CC-MAIN-2026-30-index?url=*.myworkdayjobs.com&output=json&showNumPages=true&pageSize=1": `{"pages":2}`,
		"https://index.commoncrawl.org/CC-MAIN-2026-30-index?url=*.myworkdayjobs.com&output=json&page=0&pageSize=1": `{"url": "https://acme.wd1.myworkdayjobs.com/External/job/London/Engineer_R-1"}
{"url": "https://acme.wd1.myworkdayjobs.com/robots.txt"}`,
		"https://index.commoncrawl.org/CC-MAIN-2026-30-index?url=*.myworkdayjobs.com&output=json&page=1&pageSize=1":            `{"url": "https://beta.wd3.myworkdayjobs.com/en-GB/Careers"}`,
		"https://index.commoncrawl.org/CC-MAIN-2026-25-index?url=*.myworkdayjobs.com&output=json&showNumPages=true&pageSize=1": `{"pages":0}`,
		"https://index.commoncrawl.org/CC-MAIN-2026-21-index?url=*.myworkdayjobs.com&output=json&showNumPages=true&pageSize=1": `{"pages":0}`,
	}

	got, err := (workdayProber{}).discover(context.Background(), f)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	want := []string{"acme.wd1.myworkdayjobs.com/External", "beta.wd3.myworkdayjobs.com/Careers"}
	if !slices.Equal(got, want) {
		t.Errorf("discover = %v, want %v", got, want)
	}
}
