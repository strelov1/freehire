// Package outboundurl decorates the outbound job-posting URLs freehire serves so
// the destination ATS/board can attribute the click back to us. It touches only
// the served (public) representation — the raw jobs.url column stays untagged, so
// dedup, content-hashing, and liveness probing keep working on the canonical URL.
package outboundurl

import "net/url"

// utmSource is the fixed utm_source value stamped on every outbound link, mirroring
// how the notification builders hardcode "telegram-bot"/"email" for internal links.
// It is the canonical brand domain (freehire.me since the .dev -> .me migration).
const utmSource = "freehire.me"

// Tag returns raw with utm_source=freehire.me set as a query parameter. It parses
// the URL so an existing query string is preserved (the tag is appended with the
// correct "?"/"&" separator) and any pre-existing utm_source is overwritten, keeping
// attribution consistently ours. An empty or unparseable URL is returned unchanged
// rather than mangled.
func Tag(raw string) string {
	if raw == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	q.Set("utm_source", utmSource)
	u.RawQuery = q.Encode()
	return u.String()
}
