package contribution

import (
	"net/url"
	"strings"
)

// atsBoards lists the supported multi-tenant ATS by host, mapping each to its source key. For
// all of these the board slug is the FIRST path segment, so a vacancy URL and a bare
// board-listing URL yield the same board. host matches exactly or as a subdomain suffix
// (".greenhouse.io" covers job-boards/boards/eu). Adding a path-based ATS is one row here;
// subdomain-based ATS (recruitee, teamtailor, …) extract the board differently and are added
// when board contribution expands to them.
var atsBoards = []struct{ host, source string }{
	{"greenhouse.io", "greenhouse"},
	{"jobs.lever.co", "lever"},
	{"jobs.ashbyhq.com", "ashby"},
	{"apply.workable.com", "workable"},
}

// recognizeBoard parses a pasted job link into the company board it belongs to: the source
// (ATS provider), the board slug, and the canonical URL to store (tails stripped). ok=false
// when the host is not a supported multi-tenant ATS or the URL carries no board segment.
func recognizeBoard(rawURL string) (source, board, canonical string, ok bool) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", "", "", false
	}
	src, known := sourceForHost(hostname(u))
	if !known {
		return "", "", "", false
	}
	board = firstPathSegment(u)
	if board == "" {
		return "", "", "", false
	}
	u.RawQuery = ""
	u.Fragment = ""
	u.Path = strings.TrimSuffix(strings.TrimSuffix(u.Path, "/"), "/apply")
	return src, board, u.String(), true
}

// sourceForHost returns the ATS source key for a host, matching exactly or as a subdomain.
func sourceForHost(host string) (string, bool) {
	for _, a := range atsBoards {
		if host == a.host || strings.HasSuffix(host, "."+a.host) {
			return a.source, true
		}
	}
	return "", false
}

// hostname is u's lowercased hostname with a leading "www." stripped.
func hostname(u *url.URL) string {
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}

// firstPathSegment returns u's first non-empty path segment ("/acme/jobs/1" → "acme",
// "/acme" → "acme", "/" → "").
func firstPathSegment(u *url.URL) string {
	p := strings.Trim(u.Path, "/")
	if p == "" {
		return ""
	}
	if i := strings.IndexByte(p, '/'); i >= 0 {
		return p[:i]
	}
	return p
}
