// Package adzunadesc fetches the full job description Adzuna's search API withholds.
//
// api.adzuna.com hands back only a snippet of a posting's body — "we currently only provide a
// snipped of the job description in the response" (developer.adzuna.com), confirmed live at
// exactly 500 characters, ellipsis-terminated. The full text lives only on Adzuna's own site, as
// a schema.org JobPosting block on the same URL the API already gives as redirect_url — but only
// on ONE of the two shapes that URL takes. Adzuna's own hosted page ("/details/...") serves it
// plainly; its ad-network tracking redirect ("/land/ad/...") answers its own branded "Access
// Denied" page to a non-browser request (confirmed live 2026-08-08, both the UK and DE hosts,
// even with browser-shaped headers). Eligible tells the two apart.
package adzunadesc

import (
	"net/url"
	"strings"
)

// adzunaSource is the provider name the jobs table stores for this source, matching
// sources.Provider() for the adzuna adapter.
const adzunaSource = "adzuna"

// Eligible reports whether a stored job's URL is Adzuna's own hosted job page — the shape
// this package can actually read a full description from. source must be "adzuna": every
// other provider's postings are out of scope regardless of what their URL looks like.
func Eligible(source, rawURL string) bool {
	return source == adzunaSource && detailsPageURL(rawURL)
}

// detailsPageURL reports whether rawURL is Adzuna's own hosted job page rather than its
// ad-network tracking redirect. Split out from Eligible because the runner re-checks this
// half alone, on the URL a claim carries — a stored job's url can drift between the two
// shapes for the SAME posting across re-crawls (Adzuna's API answered redirect_url
// differently for one posting between 03:03 and 08:25 on 2026-08-09, live in prod), so an
// enqueue decided at write time is not a promise about what the claim will find hours
// later when the worker gets to it.
func detailsPageURL(rawURL string) bool {
	if rawURL == "" {
		return false
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	// The ad-network redirect always carries "/land/ad/" in its path; Adzuna's own hosted
	// page never does. Checked first and alone: a URL carrying both is not something Adzuna
	// serves, and treating it as ineligible is the safe read.
	if strings.Contains(u.Path, "/land/") {
		return false
	}
	return strings.Contains(u.Path, "/details/")
}
