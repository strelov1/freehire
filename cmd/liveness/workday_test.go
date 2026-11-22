package main

import (
	"context"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/job/liveness"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestWorkdayJobAPI(t *testing.T) {
	for raw, want := range map[string]string{
		"https://citi.wd5.myworkdayjobs.com/es/2/job/Heredia--Costa-Rica/Analyst_26999832":   "https://citi.wd5.myworkdayjobs.com/wday/cxs/citi/2/job/Heredia--Costa-Rica/Analyst_26999832",
		"https://wv.wd1.myworkdayjobs.com/WVI/job/Ambassel-Ethiopia/Officer_JR53064?utm=x":   "https://wv.wd1.myworkdayjobs.com/wday/cxs/wv/WVI/job/Ambassel-Ethiopia/Officer_JR53064",
		"https://wv.wd1.myworkdayjobs.com/en-US/WVI/job/Ambassel-Ethiopia/Officer_JR53064":   "https://wv.wd1.myworkdayjobs.com/wday/cxs/wv/WVI/job/Ambassel-Ethiopia/Officer_JR53064",
		"https://acme.wd1.myworkdayjobs.com/en-US/job/Berlin/Engineer_R-1":                   "",
		"https://ipas.wd5.myworkdayjobs.com/en-US/Ipas":                                      "",
		"https://unhcr.wd3.myworkdayjobs.com/en-US/External/details/Finance-Associate_JR266": "",
		"http://wv.wd1.myworkdayjobs.com/WVI/job/A/B_1":                                      "",
		"https://evil.example/x.myworkdayjobs.com/WVI/job/A/B_1":                             "",
		"https://t.me/jobs/7": "",
	} {
		if got := workdayJobAPI(raw); got != want {
			t.Errorf("workdayJobAPI(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestWorkdayPassReadsTheCXSVerdict(t *testing.T) {
	const posting = "https://citi.wd5.myworkdayjobs.com/es/2/job/Heredia/Analyst_1"
	for _, tc := range []struct {
		name, body string
		status     int
		want       liveness.Verdict
		refused    bool
	}{
		{"live", `{"jobPostingInfo":{}}`, 200, liveness.Live, false},
		{"removed", `{"errorCode":"S22","httpStatus":403,"message":"permission denied"}`, 403, liveness.Expired, false},
		{"metered 403", `<html>blocked</html>`, 403, liveness.Uncertain, true},
		{"rate limit", "", 429, liveness.Uncertain, true},
		{"bad tenant", `{"errorCode":"HTTP_422"}`, 422, liveness.Uncertain, false},
		{"gone is not proven", "", 404, liveness.Uncertain, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: linkedInTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != "https://citi.wd5.myworkdayjobs.com/wday/cxs/citi/2/job/Heredia/Analyst_1" {
					t.Errorf("unexpected request: %s", r.URL)
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header), Request: r}, nil
			})}
			got := workdayPass(client).check(context.Background(), candidate{url: posting})
			if got.verdict != tc.want || got.refused != tc.refused {
				t.Fatalf("got %+v, want %v refused=%v", got, tc.want, tc.refused)
			}
		})
	}
}

// A telegram posting whose stored URL is a LinkedIn or Workday posting goes to that host's paced
// pass; a t.me post or an unknown host is left to the age rule; other orphans keep the GET probe.
func TestRouteUnsignalledSendsJudgeableTelegramPostingsToTheirPass(t *testing.T) {
	rows := []db.SelectOrphanLivenessCandidatesRow{
		{ID: 1, Source: "telegram", URL: "https://www.linkedin.com/jobs/view/123/", LivenessStrikes: 1},
		{ID: 2, Source: "telegram", URL: "https://t.me/jobs/7"},
		{ID: 3, Source: "telegram", URL: "https://citi.wd5.myworkdayjobs.com/es/2/job/Heredia/Analyst_1"},
		{ID: 4, Source: "telegram", URL: "https://careers.example.com/jobs/1"},
		{ID: 5, Source: "manual", URL: "https://www.linkedin.com/jobs/view/456/"},
	}
	paced := map[string][]candidate{"linkedin": {{id: 9, source: "linkedin"}}}
	orphans, routed := routeUnsignalled(rows, paced)
	if len(orphans) != 1 || orphans[0].ID != 5 {
		t.Errorf("orphans = %+v, want only the manual posting", orphans)
	}
	if routed != 2 {
		t.Errorf("routed = %d, want 2", routed)
	}
	if li := paced["linkedin"]; len(li) != 2 || li[1].id != 1 || li[1].strikes != 1 || li[1].url != rows[0].URL {
		t.Errorf("linkedin queue = %+v, want the linkedin source row then telegram 1", li)
	}
	if wd := paced["workday"]; len(wd) != 1 || wd[0].id != 3 {
		t.Errorf("workday queue = %+v, want telegram 3", wd)
	}
	for pass := range paced {
		if !slices.ContainsFunc(pacedPasses(nil, nil), func(p pacedPass) bool { return p.source == pass }) {
			t.Errorf("no paced pass reads the %s queue", pass)
		}
	}
}
