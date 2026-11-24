package main

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// apponeGetter serves canned bodies like fakeGetter, and answers the platform's own status for
// the URLs listed in status — the prober must tell a portal that does not exist (404) from a
// request the edge turned away (403).
type apponeGetter struct {
	fakeGetter
	status map[string]int
}

func (g apponeGetter) GetJSON(ctx context.Context, url string, v any) error {
	if code, ok := g.status[url]; ok {
		return &sources.StatusError{Method: http.MethodGet, Code: code, URL: url}
	}
	return g.fakeGetter.GetJSON(ctx, url, v)
}

func TestAppOneProbe(t *testing.T) {
	g := apponeGetter{
		fakeGetter: fakeGetter{
			sources.ApponePortalURL + "CRWA":  `{"companyName":"Charles River Watershed Association ","jobPosts":[{"jobPostId":"6ab41fa965690128921b7d13"},{"jobPostId":"6ab41b34cb7530b72b454b9e"}]}`,
			sources.ApponePortalURL + "quiet": `{"companyName":"Quiet LLC","jobPosts":[]}`,
		},
		status: map[string]int{
			sources.ApponePortalURL + "gone":    http.StatusNotFound,
			sources.ApponePortalURL + "refused": http.StatusForbidden,
		},
	}
	p := apponeProber{}
	if name, n, err := p.probe(context.Background(), g, "CRWA"); err != nil || name != "Charles River Watershed Association" || n != 2 {
		t.Errorf("live portal: got (%q,%d,%v), want the portal's own employer name and 2", name, n, err)
	}
	if _, n, err := p.probe(context.Background(), g, "quiet"); err != nil || n != 0 {
		t.Errorf("empty portal: got n=%d err=%v, want 0,nil", n, err)
	}
	if _, n, err := p.probe(context.Background(), g, "gone"); err != nil || n != 0 {
		t.Errorf("unknown portal: got n=%d err=%v, want a silent skip", n, err)
	}
	// A refusal is not evidence the portal is dead, so it is surfaced as a failed probe.
	if _, _, err := p.probe(context.Background(), g, "refused"); err == nil {
		t.Error("refused probe: got nil error, want it surfaced")
	}
}

func TestAppOneDedupFoldsCase(t *testing.T) {
	p := apponeProber{}
	if p.dedupKey("QCRP") != p.dedupKey("qcrp") {
		t.Error("QCRP and qcrp are the same portal and must dedup as one")
	}
}

// The seed URLs below are real apply links (2026-10-20). A posting link names no portal, so
// resolving it costs the one request for its record.
func TestAppOneResolveSeed(t *testing.T) {
	g := apponeGetter{
		fakeGetter: fakeGetter{
			sources.ApponePostingURL + "6ac801466826443a3bbf043c": `{"jobPostId":"6ac801466826443a3bbf043c","jobPortalUrl":"https://jobs.appone.com/athomecareagency"}`,
			sources.ApponePostingURL + "6abc28ad695e40521fe45170": `{"jobPostId":"6abc28ad695e40521fe45170","jobPortalUrl":"https://jobs.appone.com/QCRP"}`,
			sources.ApponePostingURL + "6a906fced2155dfaf69c5eca": `{"jobPostId":"6a906fced2155dfaf69c5eca","jobPortalUrl":""}`,
		},
		status: map[string]int{
			sources.ApponePostingURL + "6a99918b807ccd6e466971c1": http.StatusNotFound,
			sources.ApponePostingURL + "6acfbfde937210e754cfeaf7": http.StatusForbidden,
		},
	}
	cases := []struct {
		name, token, want string
		wantErr           bool
	}{
		{"posting resolves to its portal", "https://apply.appone.com/job/6ac801466826443a3bbf043c", "athomecareagency", false},
		{"portal keeps the platform's case", "https://apply.appone.com/job/6abc28ad695e40521fe45170", "QCRP", false},
		{"posting published without a portal", "https://apply.appone.com/job/6a906fced2155dfaf69c5eca", "", false},
		{"closed posting", "https://apply.appone.com/job/6a99918b807ccd6e466971c1", "", false},
		{"refused lookup is an error, not a skip", "https://apply.appone.com/job/6acfbfde937210e754cfeaf7", "", true},
		{"portal link is read offline", "https://jobs.appone.com/CRWA", "CRWA", false},
		{"bare slug", "bostonblc", "bostonblc", false},
		{"apply host without a posting id", "https://apply.appone.com/", "", false},
		{"legacy product names no portal", "https://recruiting.myapps.paychex.com/appone/MainInfoReq.asp?R_ID=7218553&B_ID=91&fid=1&SearchScreenID=23431&ssbgcolor=#FFFFFF", "", false},
		{"legacy product on its own host", "https://www.appone.com/MainInfoReq.asp?R_ID=7212267&B_ID=44&fid=1&SearchScreenID=12560&ssbgcolor=#FFFFFF", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := apponeProber{}.resolveSeed(context.Background(), g, c.token)
			if (err != nil) != c.wantErr || got != c.want {
				t.Errorf("resolveSeed(%q) = (%q, %v), want (%q, err=%v)", c.token, got, err, c.want, c.wantErr)
			}
		})
	}
}

func TestResolveCandidatesResolvesPostingSeeds(t *testing.T) {
	g := apponeGetter{fakeGetter: fakeGetter{
		sources.ApponePostingURL + "6ac801466826443a3bbf043c": `{"jobPortalUrl":"https://jobs.appone.com/athomecareagency"}`,
		sources.ApponePostingURL + "6a906fced2155dfaf69c5eca": `{"jobPortalUrl":""}`,
	}}
	seed := filepath.Join(t.TempDir(), "seed.json")
	body := `[{"board":"https://apply.appone.com/job/6ac801466826443a3bbf043c","company":"At Home InfuCare"},
	          {"board":"https://apply.appone.com/job/6a906fced2155dfaf69c5eca","company":"Video Reality, Inc."}]`
	if err := os.WriteFile(seed, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	boards, expected, err := resolveCandidates(context.Background(), apponeProber{}, g, seed)
	if err != nil {
		t.Fatalf("resolveCandidates: %v", err)
	}
	// The portal-less posting is dropped rather than proposed as a board named "".
	if len(boards) != 1 || boards[0] != "athomecareagency" {
		t.Fatalf("boards = %q, want [athomecareagency]", boards)
	}
	if expected["athomecareagency"].company != "At Home InfuCare" {
		t.Errorf("expected = %+v, want the seed's company carried to the resolved board", expected)
	}
}
