package companyname

import (
	"net/url"
	"strings"
)

// BoardFromURL extracts the ATS board identifier from a representative job URL
// for the given source, matching the host/path shape each resolver fetches
// against. It returns ("", false) for unknown sources or unparseable URLs so the
// caller skips rather than guesses.
func BoardFromURL(source, rawURL string) (string, bool) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return "", false
	}
	switch source {
	case "pinpoint", "bamboohr":
		// board is the leftmost host label: {board}.pinpointhq.com
		if i := strings.IndexByte(u.Host, '.'); i > 0 {
			return u.Host[:i], true
		}
	case "lever", "ashby":
		// board is the first path segment: jobs.lever.co/{board}/...
		if seg := firstPathSegment(u.Path); seg != "" {
			return seg, true
		}
	}
	return "", false
}

func firstPathSegment(p string) string {
	p = strings.TrimPrefix(p, "/")
	if i := strings.IndexByte(p, '/'); i >= 0 {
		p = p[:i]
	}
	return p
}
