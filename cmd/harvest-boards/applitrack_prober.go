package main

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// applitrackProber validates an AppliTrack tenant ("<board>") by counting the postings in the
// slice the adapter would actually ingest: the categories that district files its IT work under,
// never its whole board (internal/ingest/sources/applitrack.go argues why at length). A district
// with plenty of open jobs and no technology category is not a board worth committing — the crawl
// would return nothing from it every time.
//
// It mirrors the adapter's URL shape, which is unexported (this tool lives outside that
// package), and asks the adapter's own allowlist which categories count, so the two cannot drift.
// It costs two or three requests per candidate — the adapter fallback would instead run a whole
// crawl, hydrating a posting page each.
//
// Everything is read off the RAW body rather than a DOM: the platform document.writes its markup
// as escaped JavaScript, and the two things wanted here — a category name in a menu option and a
// posting id in a link — are both ASCII and both matchable where they sit, which needs neither
// the unescaping nor the parse the adapter does.
type applitrackProber struct{}

const applitrackProbeBase = "https://www.applitrack.com"

// applitrackProbeCategory captures a category name from a menu option, whose value is the
// JavaScript object literal the site's own search script reads. The apostrophes quoting the
// attribute arrive backslash-escaped, since the whole menu is written through a JS string.
var applitrackProbeCategory = regexp.MustCompile(`\{id:"([^"]*)"`)

// applitrackProbeID matches a posting link's id in the raw listing body. The listing links a
// posting through a javascript: call whose argument is percent-encoded, so the parameter
// separator arrives as "%3D" there and as a plain "=" elsewhere on the page; both are accepted,
// mirroring the adapter's own pattern.
var applitrackProbeID = regexp.MustCompile(`(?i)AppliTrackJobId(?:=|%3D)(\d+)`)

// applitrackProbeName captures the tenant name the career site renders in its heading. This is
// AppliTrack's own tenant field rather than free text a district typed into a page — every site
// sampled renders the same sentence — so it is worth reporting: it gates a candidate against what
// the seed claimed and, where the seed claimed nothing, it is what names the board in the file.
var applitrackProbeName = regexp.MustCompile(`(?is)<h1[^>]*>\s*Open Positions for\s+(.*?)\s*</h1>`)

func (applitrackProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	// The unfiltered listing is the category menu and no postings. A tenant that does not exist
	// answers 404 and the client surfaces that as an error; for harvest that is simply "not a
	// live board" — skip silently, do not propagate.
	menu, err := c.GetText(ctx, applitrackProbeURL(board, ""))
	if err != nil {
		return "", 0, nil
	}
	ids := map[string]bool{}
	for _, m := range applitrackProbeCategory.FindAllStringSubmatch(menu, -1) {
		if !sources.AppliTrackTechnologyCategory(m[1]) {
			continue
		}
		listing, err := c.GetText(ctx, applitrackProbeURL(board, m[1]))
		if err != nil {
			return "", 0, nil
		}
		for _, id := range applitrackProbeID.FindAllStringSubmatch(listing, -1) {
			ids[id[1]] = true
		}
	}
	if len(ids) == 0 {
		return "", 0, nil
	}
	// The name lives on the shell page, not on the payload endpoint above, so it is one more
	// request — spent only once the board is known to carry technical postings.
	return applitrackName(ctx, c, board), len(ids), nil
}

// applitrackProbeURL is the condensed listing, unfiltered when category is empty and restricted
// to that category otherwise.
func applitrackProbeURL(board, category string) string {
	u := fmt.Sprintf("%s/%s/onlineapp/jobpostings/Output.asp?AppliTrackLayoutMode=condensed",
		applitrackProbeBase, url.PathEscape(board))
	if category != "" {
		u += "&category=" + url.QueryEscape(category)
	}
	return u
}

// dedupKey folds a tenant to lower case. The path segment is case-insensitive (Saskatoon,
// SASKATOON and saskatoon all serve the same board, confirmed live), so a candidate spelled in
// another case than one the board file already holds is the same board — without this it probes
// live, passes the gate against its own district, and is appended a second time.
func (applitrackProber) dedupKey(board string) string { return strings.ToLower(board) }

// applitrackName reads the district name off the career site's heading, or "" when the page
// cannot be read or does not carry one — an absent name simply leaves the seed's own to label
// the board.
func applitrackName(ctx context.Context, c httpClient, board string) string {
	page, err := c.GetText(ctx, fmt.Sprintf("%s/%s/onlineapp/jobpostings/view.asp",
		applitrackProbeBase, url.PathEscape(board)))
	if err != nil {
		return ""
	}
	m := applitrackProbeName.FindStringSubmatch(page)
	if m == nil {
		return ""
	}
	return strings.Join(strings.Fields(m[1]), " ")
}
