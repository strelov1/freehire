package main

import (
	"bytes"
	"regexp"
	"strings"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/ingest/atsdetect"
	"github.com/strelov1/freehire/internal/ingest/sources"
)

// locTag captures the URL inside each <loc> element. role.com's sitemaps have no namespace
// quirks, so a tag scan is enough.
var locTag = regexp.MustCompile(`<loc>([^<]+)</loc>`)

// sitemapLocs returns every <loc> URL in a sitemap (or sitemap index) document, in order.
func sitemapLocs(xml []byte) []string {
	matches := locTag.FindAllSubmatch(xml, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, string(m[1]))
	}
	return out
}

// strideSample returns up to n items evenly spaced across items (starting at the first), or
// all of them when n exceeds the length. A capped run thus samples the whole catalogue
// rather than draining one id-range.
func strideSample(items []string, n int) []string {
	if n <= 0 || len(items) == 0 {
		return nil
	}
	if n >= len(items) {
		return items
	}
	step := float64(len(items)) / float64(n)
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, items[int(float64(i)*step)])
	}
	return out
}

// classifyDetail parses a role.com job page and resolves its apply link to the (provider,
// board) of a crawlable adapter, plus the employer name from the page's JobPosting JSON-LD.
// ok is false when there is no apply link or it is not a supported ATS URL. The company is
// kept so providers whose API exposes no name (Oracle) still get an employer label.
func classifyDetail(page []byte) (provider, board, company string, ok bool) {
	root, err := html.Parse(bytes.NewReader(page))
	if err != nil {
		return "", "", "", false
	}
	apply := sources.ElementAttr(root, "a", "job-apply", "href")
	if apply == "" {
		return "", "", "", false
	}
	provider, board, ok = atsdetect.FromURL(apply)
	if !ok {
		return "", "", "", false
	}
	var ld struct {
		HiringOrganization struct {
			Name string `json:"name"`
		} `json:"hiringOrganization"`
	}
	sources.LDJobPosting(root, &ld)
	return provider, board, strings.TrimSpace(ld.HiringOrganization.Name), true
}
