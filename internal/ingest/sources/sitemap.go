package sources

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
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

const sitemapMaxUncompressed = 32 << 20

func decodeSitemapBody(body []byte) (sitemapDoc, error) {
	raw, err := sitemapBytes(body)
	if err != nil {
		return sitemapDoc{}, err
	}
	var doc struct {
		XMLName  xml.Name
		URLs     []sitemapLoc `xml:"url"`
		Sitemaps []sitemapLoc `xml:"sitemap"`
	}
	if err := xml.Unmarshal(raw, &doc); err != nil {
		return sitemapDoc{}, err
	}
	switch strings.ToLower(doc.XMLName.Local) {
	case "urlset", "sitemapindex":
	default:
		return sitemapDoc{}, fmt.Errorf("sitemap root %q", doc.XMLName.Local)
	}
	return sitemapDoc{URLs: doc.URLs, Sitemaps: doc.Sitemaps}, nil
}

func sitemapBytes(body []byte) ([]byte, error) {
	if len(body) >= 2 && body[0] == 0x1f && body[1] == 0x8b {
		zr, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("gzip: %w", err)
		}
		defer func() { _ = zr.Close() }()
		raw, err := io.ReadAll(io.LimitReader(zr, sitemapMaxUncompressed+1))
		if err != nil {
			return nil, fmt.Errorf("gzip: %w", err)
		}
		if len(raw) > sitemapMaxUncompressed {
			return nil, fmt.Errorf("gzip sitemap exceeds %d bytes", sitemapMaxUncompressed)
		}
		body = raw
	}
	body = bytes.TrimSpace(body)
	return bytes.TrimPrefix(body, []byte("\xef\xbb\xbf")), nil
}
