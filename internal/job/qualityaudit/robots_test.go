package qualityaudit

import (
	"testing"
	"time"
)

func TestRobotsGroupChoice(t *testing.T) {
	r := parseRobots(`
User-agent: *
Disallow: /

User-agent: Freehire
Disallow: /private
Crawl-delay: 5
`)
	if !r.allowed("/jobs/1") {
		t.Error("the freehire group allows /jobs/1, the * group must not apply")
	}
	if r.allowed("/private/x") {
		t.Error("/private/x is disallowed for freehire")
	}
	if r.crawlDelay != 5*time.Second {
		t.Errorf("crawl delay = %s, want 5s", r.crawlDelay)
	}
}

func TestRobotsFallsBackToStar(t *testing.T) {
	r := parseRobots("User-agent: googlebot\nDisallow: /\n\nUser-agent: *\nDisallow: /search\n")
	if !r.allowed("/jobs/1") || r.allowed("/search?q=go") {
		t.Error("the * group should apply to freehire")
	}
}

func TestRobotsLongestMatchAndAllowTie(t *testing.T) {
	r := parseRobots(`User-agent: *
Disallow: /jobs
Allow: /jobs/view
Disallow: /a
Allow: /a
Disallow: /*.pdf$
`)
	cases := map[string]bool{
		"/jobs/123":      false,
		"/jobs/view/123": true,
		"/a/b":           true, // equal length: Allow wins
		"/file.pdf":      false,
		"/file.pdf?x=1":  true, // $ anchors the end
		"/other":         true,
	}
	for path, want := range cases {
		if got := r.allowed(path); got != want {
			t.Errorf("allowed(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestRobotsGroupedAgents(t *testing.T) {
	r := parseRobots("User-agent: bingbot\nUser-agent: freehire\nDisallow: /x\n")
	if r.allowed("/x/1") {
		t.Error("a group naming several agents applies to each")
	}
}

func TestRobotsEmptyDisallowAllowsAll(t *testing.T) {
	r := parseRobots("User-agent: *\nDisallow:\n")
	if !r.allowed("/anything") {
		t.Error("an empty Disallow allows everything")
	}
}

func TestRobotsStatusRules(t *testing.T) {
	if r := robotsFromStatus(404, ""); !r.allowed("/x") {
		t.Error("a 4xx robots.txt allows everything (RFC 9309)")
	}
	if r := robotsFromStatus(503, ""); r.allowed("/x") {
		t.Error("a 5xx robots.txt disallows everything (RFC 9309)")
	}
	if r := robotsFromStatus(0, ""); r.allowed("/x") {
		t.Error("an unreachable robots.txt disallows everything")
	}
}
