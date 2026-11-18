package main

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// applitrackProber validates an AppliTrack tenant ("<board>") by counting the postings on the
// board's "All Vacancies" listing, the same listing the adapter crawls. It mirrors the
// adapter's unexported URL shape and costs two requests per live candidate, where the adapter
// fallback would run a whole crawl.
//
// Everything is read off the RAW body rather than a DOM: the platform document.writes its markup
// as escaped JavaScript, and a posting id in a link is ASCII and matchable where it sits.
type applitrackProber struct{}

const applitrackProbeBase = "https://www.applitrack.com"

// applitrackProbeID matches a posting link's id in the raw listing body. The listing links a
// posting through a javascript: call whose argument is percent-encoded, so the parameter
// separator is "%3D" there and "=" elsewhere; both are accepted, like the adapter's pattern.
var applitrackProbeID = regexp.MustCompile(`(?i)AppliTrackJobId(?:=|%3D)(\d+)`)

// applitrackProbeName captures the tenant name the career site renders in its heading. It is
// AppliTrack's own tenant field, not free text, so it gates a candidate against the seed's claim
// and names the board when the seed claimed nothing.
var applitrackProbeName = regexp.MustCompile(`(?is)<h1[^>]*>\s*Open Positions for\s+(.*?)\s*</h1>`)

func (applitrackProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	// A tenant that does not exist answers 404 and the client surfaces that as an error; for
	// harvest that is simply "not a live board" — skip silently, do not propagate.
	listing, err := c.GetText(ctx, applitrackProbeURL(board))
	if err != nil {
		return "", 0, nil
	}
	ids := map[string]bool{}
	for _, id := range applitrackProbeID.FindAllStringSubmatch(listing, -1) {
		ids[id[1]] = true
	}
	if len(ids) == 0 {
		return "", 0, nil
	}
	// The name lives on the shell page, not on the payload endpoint above, so it is one more
	// request — spent only once the board is known to carry postings.
	return applitrackName(ctx, c, board), len(ids), nil
}

// applitrackProbeURL is the board's condensed "All Vacancies" listing.
func applitrackProbeURL(board string) string {
	return fmt.Sprintf("%s/%s/onlineapp/jobpostings/Output.asp?AppliTrackLayoutMode=condensed&all=1",
		applitrackProbeBase, url.PathEscape(board))
}

// dedupKey folds a tenant to lower case: the path segment is case-insensitive (Saskatoon,
// SASKATOON and saskatoon serve the same board), so a differently cased candidate would
// otherwise be appended a second time.
func (applitrackProber) dedupKey(board string) string { return strings.ToLower(board) }

// applitrackName reads the district name off the career site's heading, or "" when the page
// cannot be read or carries none, leaving the seed's own name to label the board.
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
