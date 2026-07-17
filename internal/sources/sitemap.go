package sources

import (
	"context"
	"fmt"
	"strings"
)

// Shared sitemaps.org decode helpers. Adapters that enumerate a platform's postings from its
// sitemap.xml decode into these types and funnel through these helpers rather than each
// redeclaring the same leaf struct and <loc>-scan loop.

// sitemapLoc is one <url> or <sitemap> entry: the URL and its optional last-modified date.
type sitemapLoc struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod"`
}

// sitemapDoc decodes either sitemap shape — a flat <urlset> (child <url>) or a <sitemapindex>
// (child <sitemap>). Both nest a <loc>, so one type serves both.
type sitemapDoc struct {
	URLs     []sitemapLoc `xml:"url"`
	Sitemaps []sitemapLoc `xml:"sitemap"`
}

// getSitemap fetches and decodes a sitemap document.
func getSitemap(ctx context.Context, c XMLGetter, url string) (sitemapDoc, error) {
	var doc sitemapDoc
	if err := c.GetXML(ctx, url, &doc); err != nil {
		return sitemapDoc{}, err
	}
	return doc, nil
}

// sitemapJobLocs fetches a flat sitemap and returns each <url> loc that id maps to a non-empty
// posting id — the shared body of the sitemap-enumerating adapters, which differ only in their
// id extractor. The error is returned unwrapped so the caller can add its own board context.
func sitemapJobLocs(ctx context.Context, c XMLGetter, url string, id func(string) string) ([]string, error) {
	doc, err := getSitemap(ctx, c, url)
	if err != nil {
		return nil, err
	}
	var locs []string
	for _, u := range doc.URLs {
		if id(u.Loc) != "" {
			locs = append(locs, u.Loc)
		}
	}
	return locs, nil
}

// resolveSubSitemap fetches a sitemap index and returns the first sub-sitemap loc whose URL
// contains needle, or "" when none matches. Shared by the sitemap-index adapters.
func resolveSubSitemap(ctx context.Context, c XMLGetter, indexURL, needle string) (string, error) {
	doc, err := getSitemap(ctx, c, indexURL)
	if err != nil {
		return "", fmt.Errorf("sitemap index: %w", err)
	}
	for _, sm := range doc.Sitemaps {
		if strings.Contains(sm.Loc, needle) {
			return sm.Loc, nil
		}
	}
	return "", nil
}
