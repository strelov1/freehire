package sources

import (
	"net/url"
	"strings"
	"time"

	"github.com/MoeForge/moego/regexp"
	xhtml "golang.org/x/net/html"
)

// remote100k adapts Remote100k (remote100k.com), a remote board of US/Canada/EU postings paying
// $100k+. It is boardless and an aggregator: the employer is read per posting. See
// providers/remote100k.md for what was measured.
//
// robots.txt disallows only /admin/, /r1/admin/, /api/ and /health/. The sitemap names every
// /remote-job/<slug> page (~1.1k, no pagination needed) and each page's schema.org JobPosting
// carries the facts; the employer's apply link is the page's "Apply for This Job" anchor.
type remote100k struct{ sitemapBoard }

const (
	remote100kSitemapURL = "https://remote100k.com/sitemap.xml"
	// remote100kRequestInterval is a courtesy ceiling, not a known limit (27 pages in a row at
	// 0.3 s drew no refusal).
	remote100kRequestInterval = time.Second
	remote100kMaxNewPerRun    = 1500
	// remote100kMaxAge: the site never retires a posting (every page answers 200 and stays in the
	// sitemap), and of the 357 open ones past 90 days on 2026-11-25 the employer's ATS had closed
	// 183 and kept 104.
	remote100kMaxAge = 90 * 24 * time.Hour
)

var remote100kPostingPath = regexp.MustCompile(`^https://remote100k\.com/remote-job/([a-z0-9][a-z0-9-]*)/?$`)

// remote100kID returns the posting slug (it is the JobPosting identifier value too), or "" for the
// sitemap's category, company and blog pages.
func remote100kID(loc string) string {
	if m := remote100kPostingPath.FindStringSubmatch(loc); m != nil {
		return m[1]
	}
	return ""
}

// remote100kApplyURL returns the href of the "Apply for This Job" anchor when it leaves the site.
func remote100kApplyURL(root *xhtml.Node) string {
	var href string
	walk(root, func(n *xhtml.Node) bool {
		if href != "" {
			return false
		}
		if n.Type == xhtml.ElementNode && n.Data == "a" && strings.Contains(nodeText(n), "Apply for This Job") {
			href = Attr(n, "href")
			return false
		}
		return true
	})
	u, err := url.Parse(strings.TrimSpace(href))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || strings.HasSuffix(u.Host, "remote100k.com") {
		return ""
	}
	return u.String()
}

// NewRemote100k builds the adapter over the given client.
func NewRemote100k(c sitemapBoardHTTP) Source {
	b := newSitemapBoard(c, "remote100k", remote100kSitemapURL, "", remote100kID, remote100kRequestInterval, remote100kMaxNewPerRun)
	b.applyURL, b.applicantCountries, b.maxAge = remote100kApplyURL, true, remote100kMaxAge
	return remote100k{b}
}

// One global feed, no per-tenant id.
func (remote100k) Boardless() {}

// Employers post here, each named per posting.
func (remote100k) Aggregator() {}

// fullCatalog: one sitemap lists the whole site and a run reads every stored posting, so one
// left out (past remote100kMaxAge, or 404) is gone. A sitemap that fails or names nothing
// fails the run, and a page that fails to read is an unreadable detail, which withholds the
// source-scoped close. Without it an employer whose every posting aged out never enters the
// company scope and its postings stay open.
func (remote100k) FullCatalog() {}
