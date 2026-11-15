package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// hireclickProber validates a HireClick board "<slug>" by counting the postings its job list
// names. It mirrors the first half of the adapter's crawl (internal/ingest/sources/hireclick.go):
// the job board page "<slug>.hireclick.com/jobboard/jobboard.aspx" carries the tenant's
// organisation GUID in #hdnOrgGuid, and "/api/Controllers/JobBoard/GetSettingsAndJobs?orgGUID="
// answers the whole list as a JSON string holding the JSON document.
// Two requests are still cheaper than the adapter fallback: a whole crawl, with every posting
// page weighing 1-3 MB (tenant logo inlined as base64). A subdomain no tenant owns lands on a
// maintenance page with no GUID, which reads as a dead board.
// It reads over its OWN long-timeout client, not the harvest's: the board page is served
// uncompressed and slowly (13-40s), past the standard 15s timeout, and a timeout would report a
// live board as dead. These requests fall outside the run's paced, counting client (see
// adapterProber's doc comment). The registry entry builds it with the client set; a zero value
// has none. It reports no company name: the list carries none, and the seed's name labels the
// board.
type hireclickProber struct {
	client interface {
		sources.HTMLGetter
		sources.JSONGetter
	}
}

// hireclickJobLink matches a posting URL in the list, mirroring the adapter's own id rule.
var hireclickJobLink = regexp.MustCompile(`/jb/[^/]+/view/(\d+)`)

func (p hireclickProber) probe(ctx context.Context, _ httpClient, slug string) (string, int, error) {
	c := p.client
	origin := fmt.Sprintf("https://%s.hireclick.com", slug)
	root, err := c.GetHTML(ctx, origin+"/jobboard/jobboard.aspx")
	if err != nil {
		return "", 0, nil
	}
	guid := hireclickOrgGUID(root)
	if guid == "" {
		return "", 0, nil
	}
	var raw string
	if err := c.GetJSON(ctx, origin+"/api/Controllers/JobBoard/GetSettingsAndJobs?orgGUID="+url.QueryEscape(guid), &raw); err != nil {
		return "", 0, nil
	}
	var doc struct {
		JobList []struct {
			JobURL string `json:"JobURL"`
		} `json:"JobList"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return "", 0, nil
	}
	ids := map[string]bool{}
	for _, j := range doc.JobList {
		if m := hireclickJobLink.FindStringSubmatch(j.JobURL); m != nil {
			ids[m[1]] = true
		}
	}
	return "", len(ids), nil
}

// hireclickOrgGUID returns the value of the page's #hdnOrgGuid hidden input, or "".
func hireclickOrgGUID(n *html.Node) string {
	if n.Type == html.ElementNode && n.Data == "input" {
		var id, value string
		for _, a := range n.Attr {
			switch a.Key {
			case "id":
				id = a.Val
			case "value":
				value = a.Val
			}
		}
		if id == "hdnOrgGuid" {
			return value
		}
	}
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		if guid := hireclickOrgGUID(ch); guid != "" {
			return guid
		}
	}
	return ""
}
