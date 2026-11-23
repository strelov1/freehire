package linksource

import (
	"net/url"
	"strings"
)

// MokaHR keeps the posting ID exclusively in the fragment, while each
// posting on the site shares the same URL path.
func mokaHRPostingRef(raw string) (id string, moka bool) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	switch strings.ToLower(u.Hostname()) {
	case "app.mokahr.com", "hire-r1.mokahr.com":
	default:
		return "", false
	}
	fragment, _, _ := strings.Cut(u.Fragment, "?")
	id, found := strings.CutPrefix(fragment, "/job/")
	if !found || !guidShape.MatchString(id) {
		return "", true
	}
	return id, true
}
