package atsdetect

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/atsboard"
)

// FromURL classifies a single outbound job-application URL into the (provider, board)
// pair one of our source adapters can crawl, returning ok=false when the URL is not a
// supported ATS or its shape does not expose the exact board id the adapter expects
// (e.g. an ADP vanity path, a Join job link, or a Workable per-job shortlink). Unlike
// Detect, which scans arbitrary HTML, this parses one known URL — the shape an
// aggregator's apply link gives us directly.
//
// The shared table answers first. internal/atsboard is the ONE definition of "which board
// does this URL address", read by the contribution flow, link resolution and boardresolve;
// this package used to carry its own copy of eleven of those hosts, and the two had already
// drifted in both directions — atsdetect read a SmartRecruiters portal segment as the board,
// while atsboard read a lowercase Workday locale and a per-job "/job" path as career sites.
// Delegating also means an ATS added to atsboard is immediately visible to the harvest tools,
// which was the standing cost of the split.
//
// What stays here are the six shapes atsboard deliberately EXCLUDES. That exclusion is not an
// oversight to correct: atsboard is the accept-set for internal/contribution, which PAYS for
// onboarded boards, so widening it is a money-affecting decision that needs its own proposal.
// These six are harvest-only, and the harvest path validates every candidate against the
// platform's API before committing it.
func FromURL(rawurl string) (provider, board string, ok bool) {
	if src, brd, _, found := atsboard.Recognize(rawurl); found {
		return src, brd, true
	}

	u, err := url.Parse(rawurl)
	if err != nil {
		return "", "", false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", "", false
	}
	segs := pathSegments(u.Path)

	switch {
	case strings.HasSuffix(host, ".icims.com"):
		// Our icims adapter builds the host "careers-<board>.icims.com", so only a
		// careers-prefixed subdomain yields a crawlable board.
		if sub := subdomain(host, ".icims.com"); sub != "" {
			if tenant := strings.TrimPrefix(sub, "careers-"); tenant != sub && tenant != "" {
				return "icims", tenant, true
			}
		}
	case strings.HasSuffix(host, ".oraclecloud.com"):
		if site := segAfter(segs, "sites"); site != "" {
			return "oracle", host + "/" + site, true
		}
	case strings.HasSuffix(host, ".taleo.net"):
		// board is the tenant host + the public careersection number, e.g.
		// "valero.taleo.net/2" from /careersection/2/jobsearch.ftl.
		if section := segAfter(segs, "careersection"); section != "" {
			return "taleo", host + "/" + section, true
		}
	case host == "www.governmentjobs.com", host == "www.schooljobs.com":
		// board is "<domain>/<agency>"; the two domains are separate tenant spaces.
		if agency := segAfter(segs, "careers"); agency != "" {
			return "neogov", strings.TrimPrefix(host, "www.") + "/" + agency, true
		}
	case host == "www.comeet.com", host == "comeet.com":
		// board is "<slug>/<companyUID>" — Comeet's adapter needs both halves, so the slug alone
		// is not a shorter board id but one the crawl rejects. The uid's fixed shape ("B1.001")
		// is what proves the path names a board: comeet.com also serves the vendor's own pages,
		// whose second segment would otherwise read as a company uid.
		if len(segs) >= 3 && segs[0] == "jobs" && comeetUID.MatchString(segs[2]) {
			return "comeet", segs[1] + "/" + segs[2], true
		}
	case host == "www.paycomonline.net", host == "paycomonline.net":
		// board is the 32-hex client key in /v4/ats/web.php/portal/<clientkey>/...
		if key := segAfter(segs, "portal"); key != "" {
			return "paycom", key, true
		}
	}
	return "", "", false
}

// comeetUID matches a Comeet company UID: two alphanumerics, a dot, three more ("B1.001",
// "32.00E"). Requiring the shape is what separates a board URL from any other comeet.com path.
var comeetUID = regexp.MustCompile(`^[0-9A-Za-z]{2}\.[0-9A-Za-z]{3}$`)

// pathSegments splits a URL path into its non-empty segments.
func pathSegments(p string) []string {
	var out []string
	for _, s := range strings.Split(p, "/") {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// subdomain returns the leftmost label of host once suffix is removed, or "" when the
// remainder is empty or itself multi-label (a shape our subdomain-keyed adapters don't
// crawl).
func subdomain(host, suffix string) string {
	sub := strings.TrimSuffix(host, suffix)
	if sub == "" || strings.Contains(sub, ".") {
		return ""
	}
	return sub
}

// segAfter returns the path segment following the first occurrence of key, or "" when
// key is absent or terminal.
func segAfter(segs []string, key string) string {
	for i, s := range segs {
		if s == key && i+1 < len(segs) {
			return segs[i+1]
		}
	}
	return ""
}
