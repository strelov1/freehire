// Package outboundurl decorates the outbound job-posting URLs freehire serves so
// the destination ATS/board can attribute the click back to us. It touches only
// the served (public) representation — the raw jobs.url column stays untagged, so
// dedup, content-hashing, and liveness probing keep working on the canonical URL.
package outboundurl

import (
	"net/url"
	"strings"
)

// utmSource is the fixed utm_source value stamped on every outbound link, mirroring
// how the notification builders hardcode "telegram-bot"/"email" for internal links.
// It is the canonical brand domain (freehire.me since the .dev -> .me migration).
const utmSource = "freehire.me"

// Untag removes the utm_source this package adds, returning the URL as the source
// published it. It exists because the tag is applied on the way OUT of jobview, so a
// consumer holding a served Job has no other route back to the canonical URL — and some
// consumers must have it. The OJCP projection is the first: that standard's
// `official_job_url` is defined as the canonical page on the employer's own site and is
// what agents deduplicate and domain-verify against, so a tracking parameter there breaks
// both.
//
// Only OUR parameter is removed. A utm_source the source itself published is already
// overwritten by Tag, so it cannot be recovered and is not pretended to be; every other
// query parameter is left exactly as it was, since it may be the posting's identifier.
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

// Tag returns raw with utm_source=freehire.me appended as a query parameter, after
// removing any utm_source the source published, so attribution is consistently ours.
// Every other byte of the query is kept as the source wrote it: re-encoding it through
// url.Values reorders parameters and escapes characters a source may read raw — ESS
// opens a posting from "?idp=3419120/" and shows an empty page for "?idp=3419120%2F"
// (freehire#4003). An empty or unparseable URL is returned unchanged rather than mangled.
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
		return key == "utm_source"
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
