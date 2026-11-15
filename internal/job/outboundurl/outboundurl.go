// Package outboundurl decorates the outbound job-posting URLs freehire serves so the
// destination ATS/board can attribute the click to us. Only the served (public)
// representation is tagged; the raw jobs.url stays canonical for dedup, hashing and
// liveness probing.
package outboundurl

import (
	"net/url"
	"strings"
)

// utmSource is the utm_source value stamped on every outbound link: the canonical
// brand domain.
const utmSource = "freehire.me"

// Untag removes the utm_source this package adds, returning the URL as the source
// published it. The tag is applied on the way out of jobview, so a consumer holding a
// served Job has no other route back to the canonical URL. The OJCP projection needs
// it: `official_job_url` is the canonical employer page agents deduplicate and
// domain-verify against.
//
// Only OUR parameter is removed. A utm_source the source published was already
// overwritten by Tag and cannot be recovered; every other query parameter is left
// as it was, since it may be the posting's identifier.
func Untag(tagged string) string {
	if tagged == "" {
		return tagged
	}
	u, err := url.Parse(tagged)
	if err != nil {
		return tagged
	}
	ours := "utm_source=" + utmSource
	kept, found := withoutParams(u.RawQuery, func(param string) bool { return param == ours })
	if !found {
		return tagged
	}
	u.RawQuery = kept
	return u.String()
}

// Tag returns raw with utm_source=freehire.me appended, after removing any utm_source
// the source published. Every other byte of the query is kept as the source wrote it:
// re-encoding through url.Values reorders parameters and escapes characters a source
// may read raw (ESS opens "?idp=3419120/" but shows an empty page for "?idp=3419120%2F").
// An empty or unparseable URL is returned unchanged.
func Tag(raw string) string {
	if raw == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	kept, _ := withoutParams(u.RawQuery, func(param string) bool {
		key, _, _ := strings.Cut(param, "=")
		decoded, err := url.QueryUnescape(key)
		return err == nil && decoded == "utm_source"
	})
	if kept != "" {
		kept += "&"
	}
	u.RawQuery = kept + "utm_source=" + utmSource
	return u.String()
}

// withoutParams drops the &-separated parameters of rawQuery that drop matches, leaving
// the rest byte for byte and in order, and reports whether any was dropped.
func withoutParams(rawQuery string, drop func(param string) bool) (string, bool) {
	if rawQuery == "" {
		return "", false
	}
	params := strings.Split(rawQuery, "&")
	kept := params[:0]
	for _, param := range params {
		if !drop(param) {
			kept = append(kept, param)
		}
	}
	return strings.Join(kept, "&"), len(kept) != len(params)
}
