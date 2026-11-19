package sources

import (
	"regexp"
	"time"
)

// dsml adapts the job board of DSML Kazakhstan (dsml.kz), the Kazakh data-science and ML
// community. It is boardless and an aggregator — the employer is read per posting.
//
// robots.txt allows search crawling (it disallows only /api/ and account pages). The sitemap
// lists every open posting as /jobs/<uuid>, beside category and city pages under /jobs/ that are
// not postings; each posting page carries a schema.org JobPosting with salary.
type dsml struct{ sitemapBoard }

const (
	dsmlSitemapURL = "https://dsml.kz/sitemap.xml"
	// A small community site on Vercel: one request every two seconds.
	dsmlRequestInterval = 2 * time.Second
	dsmlMaxNewPerRun    = 200
)

var dsmlJobIDPattern = regexp.MustCompile(`^https://dsml\.kz/jobs/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$`)

// dsmlID returns a posting's uuid, or "" for any sitemap entry that is not a posting.
func dsmlID(loc string) string { return firstSubmatch(dsmlJobIDPattern, loc) }

// NewDSML builds the adapter over the given client.
func NewDSML(c sitemapBoardHTTP) Source {
	return dsml{newSitemapBoard(c, "dsml", dsmlSitemapURL, "kz", dsmlID, dsmlRequestInterval, dsmlMaxNewPerRun)}
}

// One board, no per-tenant id.
func (dsml) Boardless() {}

// Companies post here, each named per posting.
func (dsml) Aggregator() {}
