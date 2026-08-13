package companyname

import (
	"net/url"
	"strings"
)

// Board returns the ATS board identifier to resolve a company's real name for.
// Most sources embed it in a representative job URL (BoardFromURL). join.com is
// the exception: its job URL carries the company's URL-slug domain, not its
// numeric company id, so the id can't be recovered from the URL at all — but
// when name is itself a numeric placeholder (see NumericPlaceholder), that
// placeholder IS the join company id, verbatim, because that's how it got
// written into the company field in the first place. Using it directly needs
// no URL parsing and can't be spoofed by an unrelated numeric-looking name from
// another source, since this path only fires for join.
func Board(source, name, rawURL string) (string, bool) {
	if source == "join" && NumericPlaceholder(name) {
		return name, true
	}
	return BoardFromURL(source, rawURL)
}

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
