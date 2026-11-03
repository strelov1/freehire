package sources

import (
	"net/url"
	"strings"
	"time"
)

// berlinstartupjobs adapts Berlin Startup Jobs (berlinstartupjobs.com), a board of Berlin startup
// postings: ~140 at a time, its terms allowing Berlin only. It is boardless and an aggregator —
// the employer is read per posting.
//
// robots.txt leaves the site open to generic crawlers, and the terms for job ads acknowledge that
// crawlers may download the ads. The posting set is post-sitemap.xml, whose entries are
// /<category>/<slug>/ pages; the site's apply link usually leaves for join.com, which the pipeline's
// dedup then sees as the same posting.
type berlinstartupjobs struct{ sitemapBoard }

const (
	berlinstartupjobsBase       = "https://berlinstartupjobs.com"
	berlinstartupjobsSitemapURL = berlinstartupjobsBase + "/post-sitemap.xml"
	// A small site that states no crawl limit for generic bots: one request every two seconds.
	berlinstartupjobsRequestInterval = 2 * time.Second
	berlinstartupjobsMaxNewPerRun    = 500
)

// berlinstartupjobsID returns a posting's identity, its /<category>/<slug> path, or "" for the
// home page and anything else the sitemap lists that is not a posting. The slug is the identity
// because the page carries no id the sitemap shows; a retitled posting would be a new one, which
// is the same call every slug-keyed adapter makes.
func berlinstartupjobsID(loc string) string {
	u, err := url.Parse(loc)
	if err != nil || u.Host != "berlinstartupjobs.com" {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	return parts[0] + "/" + parts[1]
}

// NewBerlinStartupJobs builds the adapter over the given client.
func NewBerlinStartupJobs(c sitemapBoardHTTP) Source {
	return berlinstartupjobs{newSitemapBoard(c, "berlinstartupjobs", berlinstartupjobsSitemapURL, "de", berlinstartupjobsID, berlinstartupjobsRequestInterval, berlinstartupjobsMaxNewPerRun)}
}

// One board, no per-tenant id.
func (berlinstartupjobs) boardless() {}

// Startups post here, each named per posting.
func (berlinstartupjobs) aggregator() {}
