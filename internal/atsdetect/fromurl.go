package atsdetect

import (
	"net/url"
	"regexp"
	"strings"
)

// localeSegment matches a Workday URL's optional locale path segment (e.g. "en-us",
// "de-DE"), which sits before the career-site segment and is not part of the board id.
var localeSegment = regexp.MustCompile(`^[a-z]{2}-[A-Za-z]{2}$`)

// FromURL classifies a single outbound job-application URL into the (provider, board)
// pair one of our source adapters can crawl, returning ok=false when the URL is not a
// supported ATS or its shape does not expose the exact board id the adapter expects
// (e.g. an ADP vanity path, a Join job link, or a Workable per-job shortlink). Unlike
// Detect, which scans arbitrary HTML, this parses one known URL — the shape an
// aggregator's apply link gives us directly.
func FromURL(rawurl string) (provider, board string, ok bool) {
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
	case strings.HasSuffix(host, ".myworkdayjobs.com"):
		if site := workdaySite(segs); site != "" {
			return "workday", host + "/" + site, true
		}
	case host == "jobs.smartrecruiters.com":
		if len(segs) >= 1 {
			return "smartrecruiters", segs[0], true
		}
	case host == "job-boards.greenhouse.io", host == "boards.greenhouse.io",
		host == "job-boards.eu.greenhouse.io", host == "boards.eu.greenhouse.io":
		if len(segs) >= 1 && !reserved[segs[0]] {
			return "greenhouse", segs[0], true
		}
	case host == "jobs.lever.co":
		if len(segs) >= 1 {
			return "lever", segs[0], true
		}
	case host == "jobs.ashbyhq.com":
		if len(segs) >= 1 {
			return "ashby", segs[0], true
		}
	case strings.HasSuffix(host, ".applytojob.com"):
		if sub := subdomain(host, ".applytojob.com"); sub != "" {
			return "jazzhr", sub, true
		}
	case strings.HasSuffix(host, ".recruitee.com"):
		if sub := subdomain(host, ".recruitee.com"); sub != "" {
			return "recruitee", sub, true
		}
	case strings.HasSuffix(host, ".pinpointhq.com"):
		if sub := subdomain(host, ".pinpointhq.com"); sub != "" {
			return "pinpoint", sub, true
		}
	case strings.HasSuffix(host, ".careerplug.com"):
		if sub := subdomain(host, ".careerplug.com"); sub != "" {
			return "careerplug", sub, true
		}
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
	}
	return "", "", false
}

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

// workdaySite returns the career-site segment from a Workday URL's path, skipping the
// optional leading locale segment. It returns "" when no site precedes the per-job
// "/job" (or "/details") segment.
func workdaySite(segs []string) string {
	if len(segs) == 0 {
		return ""
	}
	i := 0
	if localeSegment.MatchString(segs[0]) {
		i = 1
	}
	if i >= len(segs) {
		return ""
	}
	site := segs[i]
	if site == "job" || site == "details" {
		return ""
	}
	return site
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
