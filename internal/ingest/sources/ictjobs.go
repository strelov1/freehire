package sources

import (
	"net/url"
	"strings"
	"time"
)

// ictjobs adapts ictjobs.ch, Switzerland's IT job board (WordPress, JobCloud-independent): about
// 190 postings from Swiss employers, every one filed under an IT category (software, systems and
// networks, security, support, consulting, project management). It is boardless and an
// aggregator — the employer is read per posting — and IT-only by the source's own definition, so
// every posting carries the technical hint.
//
// robots.txt disallows only /cms/wp-admin/. The posting set is post-sitemap.xml, whose entries
// are /<category>/<slug>/ pages, each carrying a schema.org JobPosting with the full body, the
// employer, the place, the employment type and often a stated salary. The www host redirects to
// the bare domain. See providers/ictjobs.md.
type ictjobs struct{ sitemapBoard }

const (
	ictjobsHost       = "ictjobs.ch"
	ictjobsSitemapURL = "https://" + ictjobsHost + "/post-sitemap.xml"
	// A small site that states no crawl limit: one request every two seconds, so a cold
	// catalogue of ~190 pages takes about six minutes.
	ictjobsRequestInterval = 2 * time.Second
	ictjobsMaxNewPerRun    = 300
)

// ictjobsID returns a posting's identity, its /<category>/<slug> path, or "" for anything else
// the sitemap lists. The slug is the identity because the sitemap shows no id; a retitled posting
// would be a new one, the call every slug-keyed adapter makes.
func ictjobsID(loc string) string {
	u, err := url.Parse(loc)
	if err != nil || u.Host != ictjobsHost {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	return parts[0] + "/" + parts[1]
}

// NewICTJobs builds the adapter over the given client.
func NewICTJobs(c sitemapBoardHTTP) Source {
	b := newSitemapBoard(c, "ictjobs", ictjobsSitemapURL, "ch", ictjobsID, ictjobsRequestInterval, ictjobsMaxNewPerRun)
	b.techHint = true
	b.ignoreTelecommute = true
	return ictjobs{b}
}

// One board, no per-tenant id.
func (ictjobs) Boardless() {}

// Swiss employers post here, each named per posting.
func (ictjobs) Aggregator() {}
