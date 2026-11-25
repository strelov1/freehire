package sources

import (
	"time"

	"github.com/MoeForge/moego/regexp"
)

// remotejobsfinder adapts remotejobsfinder.co, a US-focused remote/hybrid board whose postings
// are re-listings of employers' own jobs (company logos come from a data vendor, there is no
// apply link on the page). Boardless and an aggregator: the employer is read per posting.
//
// robots.txt leaves the sitemap open; /sitemap_index.xml names sitemap_listings_active.xml, one
// flat list of the English posting pages (the site serves other languages under /<lang>/, which
// this adapter does not read, so a posting is not ingested twice). Every fact comes from the
// page's JobPosting; see sitemapBoard for the shared body and providers/remotejobsfinder.md.
type remotejobsfinder struct{ sitemapBoard }

const (
	remotejobsfinderSitemapURL = "https://remotejobsfinder.co/sitemap_listings_active.xml"
	// Pages are ~300 KB: a courtesy ceiling, not a known limit.
	remotejobsfinderRequestInterval = 500 * time.Millisecond
	// The sitemap names ~3.9k postings, so a cold run reads them all.
	remotejobsfinderMaxNewPerRun = 4500
)

// remotejobsfinderPostingPath matches an English posting URL and captures its uuid; the slug in
// front of it is the title.
var remotejobsfinderPostingPath = regexp.MustCompile(`^https://remotejobsfinder\.co/en/(?:remote|hybrid)-jobs/[^/]+/[^/?#]+_([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$`)

// remotejobsfinderID returns the posting uuid a sitemap URL carries, or "".
func remotejobsfinderID(loc string) string {
	if m := remotejobsfinderPostingPath.FindStringSubmatch(loc); m != nil {
		return m[1]
	}
	return ""
}

// NewRemoteJobsFinder builds the adapter over the given client. Every posting is US-based (the
// sitemap holds only /usa/ paths) and the page states no jobLocation, hence the "us" fallback.
func NewRemoteJobsFinder(c sitemapBoardHTTP) Source {
	return remotejobsfinder{newSitemapBoard(c, "remotejobsfinder", remotejobsfinderSitemapURL, "us", remotejobsfinderID, remotejobsfinderRequestInterval, remotejobsfinderMaxNewPerRun)}
}

// One global feed, no per-tenant board.
func (remotejobsfinder) Boardless() {}

// Employers post here, each named per posting.
func (remotejobsfinder) Aggregator() {}
