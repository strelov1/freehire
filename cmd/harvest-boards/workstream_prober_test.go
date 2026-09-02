package main

import (
	"context"
	"testing"
)

// workstreamListingPage renders a career-site page the way www.workstream.us does: the posting
// links a card repeats three times, and the inline searchBaseUrl the prober steers by.
func workstreamListingPage(title, searchBase string, postings ...string) string {
	body := ""
	for _, p := range postings {
		body += `<div class="position-card"><a href="` + p + `?locale=en">Role</a>` +
			`<a class="view-position-btn" href="` + p + `?locale=en">→</a></div>`
	}
	return `<html><head><title>` + title + `</title></head><body>` + body +
		`<a href="/j/965a796b/moxies?locale=en">All positions</a>` +
		`<script>var currentPage = 1;
var totalPages = 4;
var searchBaseUrl = '` + searchBase + `';</script></body></html>`
}

func TestWorkstreamProbe(t *testing.T) {
	p := workstreamProber{}
	getter := fakeGetter{
		// A multi-brand employer: "/j/<board>/positions" is the listing itself.
		"https://www.workstream.us/j/965a796b/positions": workstreamListingPage(
			"FineCasual Careers and Jobs", "https://www.workstream.us/j/965a796b/positions",
			"/j/965a796b/moxies/pickering-79247/line-cook-09eacfc8",
			"/j/965a796b/chop-steakhouse-bar/vaughan-79642/server-b74e7ee4"),
		// A single-brand employer: the request is redirected to the brand root, a LOCATIONS
		// listing that carries no posting link and names where the positions are.
		"https://www.workstream.us/j/a8f9405b/positions": workstreamListingPage(
			"Sarku Japan Careers and Jobs",
			"https://www.workstream.us/j/a8f9405b/sarku-japan/positions"),
		"https://www.workstream.us/j/a8f9405b/sarku-japan/positions": workstreamListingPage(
			"Sarku Japan Careers and Jobs",
			"https://www.workstream.us/j/a8f9405b/sarku-japan/positions",
			"/j/a8f9405b/sarku-japan/clearwater-217806/crew-member-3ae49676"),
		// A live career site with nothing open.
		"https://www.workstream.us/j/00161ada/positions": workstreamListingPage(
			"Closed Co Careers and Jobs", "https://www.workstream.us/j/00161ada/positions"),
	}

	// The employer name comes off the title, and a card's repeated links count once.
	if name, n, err := p.probe(context.Background(), getter, "965a796b"); err != nil ||
		name != "FineCasual" || n != 2 {
		t.Errorf("multi-brand: got (%q,%d,%v), want (\"FineCasual\",2,nil)", name, n, err)
	}
	// The single-brand board is counted from the listing the page pointed at, not from the
	// locations page the redirect landed on.
	if name, n, err := p.probe(context.Background(), getter, "a8f9405b"); err != nil ||
		name != "Sarku Japan" || n != 1 {
		t.Errorf("single-brand: got (%q,%d,%v), want (\"Sarku Japan\",1,nil)", name, n, err)
	}
	// A site with no open posting is not a board worth committing.
	if _, n, err := p.probe(context.Background(), getter, "00161ada"); err != nil || n != 0 {
		t.Errorf("empty: got n=%d err=%v, want 0,nil", n, err)
	}
	// A retired employer answers 410; the client reports an error and the prober reads that as
	// "not a live board" rather than aborting the harvest.
	if _, n, err := p.probe(context.Background(), getter, "deadbeef"); err != nil || n != 0 {
		t.Errorf("gone: got n=%d err=%v, want 0,nil", n, err)
	}
}

func TestWorkstreamEmployer(t *testing.T) {
	cases := map[string]string{
		"FineCasual Careers and Jobs": "FineCasual",
		// The site truncates a long title mid-suffix; trimming the whole phrase would leave the
		// debris in the name the board file carries (two live boards did exactly that).
		"Good Charlie's Oyster Bar & Seafood Kitchen Careers and...": "Good Charlie's Oyster Bar & Seafood Kitchen",
		// A title that carries no suffix at all is the employer name already.
		"Sarku Japan": "Sarku Japan",
		"":            "",
	}
	for title, want := range cases {
		if got := workstreamEmployer(title); got != want {
			t.Errorf("workstreamEmployer(%q) = %q, want %q", title, got, want)
		}
	}
}

func TestWorkstreamCandidate(t *testing.T) {
	cases := map[string]string{
		"https://www.workstream.us/j/965a796b/chop-steakhouse-bar/vaughan-79642/server-b74e7ee4": "965a796b",
		"https://www.workstream.us/j/004a7767/chick-fil-a/positions?geo=Ashburn":                 "004a7767",
		"https://www.workstream.us/j/004a7767":                                                   "004a7767",
		"https://www.workstream.us/j/sarku_japan_sarku_japan":                                    "",
		"https://www.workstream.us/j/css/index.css":                                              "",
		"https://www.workstream.us/j/share/redirect":                                             "",
		"https://www.workstream.us/blog/hiring":                                                  "",
	}
	for rawURL, want := range cases {
		got, ok := workstreamCandidate(rawURL)
		if want == "" {
			if ok {
				t.Errorf("workstreamCandidate(%q) = %q, want no candidate", rawURL, got)
			}
			continue
		}
		if !ok || got != want {
			t.Errorf("workstreamCandidate(%q) = (%q,%v), want %q", rawURL, got, ok, want)
		}
	}
}

func TestWorkstreamProberRegistered(t *testing.T) {
	p, ok := proberFor("workstream")
	if !ok {
		t.Fatal(`proberFor("workstream") not found`)
	}
	// The bespoke prober must win: the adapter fallback would run a whole paced crawl per
	// candidate, and it answers outside the run's rate limiter.
	if _, fellBack := p.(adapterProber); fellBack {
		t.Errorf("workstream resolved to the adapter fallback, got %T", p)
	}
	if _, canDiscover := p.(discoverer); !canDiscover {
		t.Error("workstream should discover its own candidates — the platform publishes no seed list")
	}
}
