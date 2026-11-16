package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// LinkedInCompanyWebsite reads the public Organization's website, the same
// metadata used by the LinkedIn board-harvest workflow. It never derives a domain
// from a company name or treats an unrelated page link as the employer website.
func LinkedInCompanyWebsite(root *html.Node) string {
	var website string
	walk(root, func(n *html.Node) bool {
		if website != "" {
			return false
		}
		if n.Type != html.ElementNode || n.Data != "script" || Attr(n, "type") != "application/ld+json" {
			return true
		}
		var doc struct {
			Type   string          `json:"@type"`
			SameAs json.RawMessage `json:"sameAs"`
			Graph  []struct {
				Type   string          `json:"@type"`
				SameAs json.RawMessage `json:"sameAs"`
			} `json:"@graph"`
		}
		if json.Unmarshal([]byte(textContent(n)), &doc) != nil {
			return true
		}
		read := func(kind string, raw json.RawMessage) {
			if kind != "Organization" || website != "" {
				return
			}
			var value string
			var values []string
			if json.Unmarshal(raw, &value) == nil {
				values = []string{value}
			} else if json.Unmarshal(raw, &values) != nil {
				return
			}
			for _, value := range values {
				u, err := url.Parse(value)
				if err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Hostname() != "" && u.User == nil && !linkedInHost(u.Hostname()) {
					website = u.String()
					return
				}
			}
		}
		read(doc.Type, doc.SameAs)
		for _, node := range doc.Graph {
			read(node.Type, node.SameAs)
		}
		return true
	})
	return website
}

func linkedInHost(host string) bool {
	host = strings.ToLower(host)
	return host == "linkedin.com" || strings.HasSuffix(host, ".linkedin.com")
}

// CompanyWebsite shares the adapter's request limiter with listing and detail
// reads, so company discovery cannot multiply the LinkedIn request budget.
func (s linkedIn) CompanyWebsite(ctx context.Context, raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || !linkedInHost(u.Hostname()) || !strings.HasPrefix(u.Path, "/company/") || u.User != nil || u.Port() != "" {
		return "", fmt.Errorf("linkedin: invalid employer profile URL")
	}
	root, final, err := s.get(ctx, raw)
	if err != nil {
		return "", err
	}
	resolved, err := url.Parse(final)
	if err != nil || resolved.Scheme != "https" || resolved.User != nil || resolved.Port() != "" || !linkedInHost(resolved.Hostname()) || strings.TrimRight(resolved.Path, "/") != strings.TrimRight(u.Path, "/") {
		return "", fmt.Errorf("linkedin: employer profile redirected away")
	}
	return LinkedInCompanyWebsite(root), nil
}
