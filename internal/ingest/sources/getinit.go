package sources

import (
	"regexp"
	"time"
)

// getinit adapts Get in IT (www.get-in-it.de), the German entry-level IT job board: ~1.2k
// postings, most for students, trainees and career starters. It is boardless and an aggregator —
// the employer is read per posting.
//
// The posting set is the public sitemap (robots.txt leaves /jobsuche/p<id> and the sitemap open
// and disallows only the /feeds/ exports and previews); every fact comes from the posting page's
// JobPosting. Its terms of use forbid automated queries "ohne ausdrückliche Einwilligung" but
// exempt job search engines, which is what this is; see sitemapBoard for the shared body.
type getinit struct{ sitemapBoard }

const (
	getinitSitemapURL = "https://www.get-in-it.de/sitemap.xml"
	// getinitRequestInterval is a courtesy ceiling, not a measured limit: a run is ~1.2k pages on
	// the first crawl and a handful afterwards.
	getinitRequestInterval = time.Second
	getinitMaxNewPerRun    = 1500
)

// getinitPostingPath matches a posting URL, /jobsuche/p<id>; the sitemap's other /jobsuche/
// entries are not postings.
var getinitPostingPath = regexp.MustCompile(`^https://www\.get-in-it\.de/jobsuche/p(\d+)/?$`)

// getinitID returns the numeric posting id a sitemap URL carries, or "".
func getinitID(loc string) string {
	if m := getinitPostingPath.FindStringSubmatch(loc); m != nil {
		return m[1]
	}
	return ""
}

// NewGetInIT builds the adapter over the given client.
func NewGetInIT(c sitemapBoardHTTP) Source {
	return getinit{newSitemapBoard(c, "getinit", getinitSitemapURL, "de", getinitID, getinitRequestInterval, getinitMaxNewPerRun)}
}

// One national board, no per-tenant id.
func (getinit) boardless() {}

// Employers post here, each named per posting.
func (getinit) aggregator() {}
