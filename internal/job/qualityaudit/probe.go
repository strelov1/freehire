package qualityaudit

import (
	"encoding/json"
	"net/url"
	"strings"
)

// probeURL selects the source's posting resource. AppOne's public Angular route returns
// 404 even for open postings; the adapter hydrates them from this JSON endpoint instead.
// The host stays unchanged so the same robots rules, pacing and budget govern both requests.
func probeURL(p Posting) string {
	u, err := url.Parse(p.URL)
	if err != nil || p.Source != "appone" || !strings.EqualFold(u.Hostname(), "apply.appone.com") {
		return p.URL
	}
	id := strings.TrimPrefix(u.Path, "/job/")
	nativeID := p.ExternalID[strings.LastIndex(p.ExternalID, ":")+1:]
	if id == "" || id != nativeID || strings.Contains(id, "/") {
		return p.URL
	}
	u.Path = "/api/apply/v2/jobposting/" + id
	u.RawPath, u.RawQuery, u.Fragment = "", "", ""
	return u.String()
}

// A successful detail response needs a posting body, as the ingest adapter requires. An
// HTML challenge or a generic JSON success must never turn into evidence of a live posting.
func appOneDetail(body string) bool {
	var detail struct {
		Description string `json:"description"`
	}
	return json.Unmarshal([]byte(body), &detail) == nil && strings.TrimSpace(detail.Description) != ""
}
