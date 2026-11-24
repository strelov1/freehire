// Package outboundurl owns the links freehire sends a visitor to an employer's site by.
//
// Every such link goes through one redirect, GET /go/<slug>?src=<surface>, which records the
// click and sends the visitor on. ApplyURL builds that link; Tag stamps the destination the
// redirect forwards to, so the employer's ATS can attribute the visit to us. The raw jobs.url
// stays canonical for dedup, hashing and liveness probing, and the served `url` carries it
// untagged for the same reason.
package outboundurl

import (
	"net/url"
	"slices"
	"strings"
)

// utmSource is the utm_source value stamped on every outbound link: the canonical
// brand domain.
const utmSource = "freehire.me"

// origin is the public site the redirect lives on. Fixed like utmSource: the brand domain, not a
// per-deployment setting, because the link is handed to third parties (mail, agents, other sites).
const origin = "https://freehire.me"

// Src names the surface that handed out a redirect link. It is stored with every click, so the
// employer-facing numbers can say where the traffic came from.
type Src string

const (
	SrcWeb       Src = "web"
	SrcMCP       Src = "mcp"
	SrcOJCP      Src = "ojcp"
	SrcAssistant Src = "assistant"
	SrcEmail     Src = "email"
	SrcTelegram  Src = "telegram"
	SrcAPI       Src = "api"
	// SrcOther is what a value outside the list above is stored as. It is never built into a
	// link: a surface that exists gets its own constant.
	SrcOther Src = "other"
)

var knownSrcs = []Src{SrcWeb, SrcMCP, SrcOJCP, SrcAssistant, SrcEmail, SrcTelegram, SrcAPI}

// ParseSrc reads the ?src= of a request. A missing or unknown value is SrcOther, not an error:
// the redirect must still work for a link someone typed or shortened by hand.
func ParseSrc(raw string) Src {
	if s := Src(raw); slices.Contains(knownSrcs, s) {
		return s
	}
	return SrcOther
}

// ApplyURL is the redirect link for a job: what every surface hands out instead of the employer
// URL. The slug is a public_slug, which is URL-safe by construction.
func ApplyURL(slug string, src Src) string {
	return origin + "/go/" + slug + "?src=" + string(src)
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
	kept := withoutParams(u.RawQuery, func(param string) bool {
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
// the rest byte for byte and in order.
func withoutParams(rawQuery string, drop func(param string) bool) string {
	if rawQuery == "" {
		return ""
	}
	params := strings.Split(rawQuery, "&")
	kept := params[:0]
	for _, param := range params {
		if !drop(param) {
			kept = append(kept, param)
		}
	}
	return strings.Join(kept, "&")
}
