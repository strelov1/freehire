package sources

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// avature adapts Avature career sites (e.g. EA's jobs.ea.com, canonically ea.avature.net).
// The board is the career-site host. A sitemap index advertises one sitemap per locale; the
// adapter crawls the en_US one (the other locales list the same postings) and reads its
// JobDetail URLs. Each job page is server-rendered HTML with no schema.org JSON-LD, so the
// title comes from og:title and the location/work-model/description from the page's
// article__content__view__field label/value blocks (description = the richest value).
//
// Almost every tenant names its public portal "careers", so that path is tried first at no
// extra request. A handful front their listing behind a differently-named portal instead (Qatar
// Airways: "global") — robots.txt enumerates every portal Avature registered for the host, so
// when /careers/ carries no postings each same-host Sitemap: line it advertises is tried in
// turn until one does. A tenant whose portals all wall off requisitions (Bain: real portals,
// zero listed postings in any of them) legitimately exhausts every candidate and errors.
type avatureHTTP interface {
	XMLGetter
	HTMLGetter
	TextGetter
}

type avature struct {
	http avatureHTTP
}

// NewAvature builds the Avature adapter over the given HTTP client.
func NewAvature(c avatureHTTP) Source { return avature{http: c} }

func (avature) Provider() string { return "avature" }

func (s avature) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	careersURL := fmt.Sprintf("https://%s/careers/sitemap_index.xml", e.Board)
	entries, err := avatureJobLocs(ctx, s.http, careersURL)
	if len(entries) == 0 {
		if alt, altErr := avatureFallbackLocs(ctx, s.http, e.Board); len(alt) > 0 {
			entries, err = alt, nil
		} else if altErr != nil {
			err = altErr
		}
	}
	if err != nil {
		return nil, fmt.Errorf("avature: %s: %w", e.Board, err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("avature: %s: no JobDetail postings found", e.Board)
	}

	return fetchDetails(entries, defaultDetailWorkers, func(entry sitemapLoc) (Job, bool) {
		return s.detail(ctx, e, entry)
	}), nil
}

// avatureLocaleSegment matches a locale code path segment (e.g. "/en_US/", "/es_ES/").
var avatureLocaleSegment = regexp.MustCompile(`/[a-z]{2}_[A-Z]{2}/`)

// avatureJobLocs resolves one sitemap index into its JobDetail entries, filtered to posting
// pages — utility routes like AgentCreate are excluded. Most tenants split their index by
// locale, so the en_US sub-sitemap is preferred (the other locales list the same postings); a
// tenant that advertises a locale split but no en_US within it returns (nil, nil) rather than
// guessing a different language. A tenant with no locale split at all (Qatar Airways' "global"
// portal, one sub-sitemap with no locale segment) uses that sole sub-sitemap as-is.
func avatureJobLocs(ctx context.Context, c avatureHTTP, indexURL string) ([]sitemapLoc, error) {
	index, err := getSitemap(ctx, c, indexURL)
	if err != nil {
		return nil, fmt.Errorf("sitemap index %s: %w", indexURL, err)
	}

	listURL, localeSplit := "", false
	for _, sm := range index.Sitemaps {
		if avatureLocaleSegment.MatchString(sm.Loc) {
			localeSplit = true
			if strings.Contains(sm.Loc, "/en_US/") {
				listURL = sm.Loc
				break
			}
		}
	}
	if listURL == "" && !localeSplit && len(index.Sitemaps) == 1 {
		listURL = index.Sitemaps[0].Loc
	}
	if listURL == "" {
		return nil, nil
	}

	urlset, err := getSitemap(ctx, c, listURL)
	if err != nil {
		return nil, fmt.Errorf("sitemap %s: %w", listURL, err)
	}

	var entries []sitemapLoc
	for _, u := range urlset.URLs {
		if strings.Contains(u.Loc, "/JobDetail/") {
			entries = append(entries, u)
		}
	}
	return entries, nil
}

// avatureFallbackLocs is reached only when the conventional /careers/ portal carries no
// postings. It fetches robots.txt from the board host and tries every other Sitemap: line
// targeting that same host (a tenant can run several portals — applicant login, hiring-manager
// tools, a public listing under a differently-named one) until one yields JobDetail entries.
func avatureFallbackLocs(ctx context.Context, c avatureHTTP, board string) ([]sitemapLoc, error) {
	robots, err := c.GetText(ctx, fmt.Sprintf("https://%s/robots.txt", board))
	if err != nil {
		return nil, fmt.Errorf("robots.txt: %w", err)
	}

	prefix := "https://" + board + "/"
	tried := map[string]bool{prefix + "careers/sitemap_index.xml": true}
	var lastErr error
	for _, line := range strings.Split(robots, "\n") {
		u, ok := strings.CutPrefix(strings.TrimSpace(line), "Sitemap:")
		if !ok {
			continue
		}
		u = strings.TrimSpace(u)
		if !strings.HasPrefix(u, prefix) || tried[u] {
			continue // a different host's portal (e.g. the tenant's own *.avature.net) or a repeat
		}
		tried[u] = true

		entries, err := avatureJobLocs(ctx, c, u)
		if err != nil {
			lastErr = err
			continue
		}
		if len(entries) > 0 {
			return entries, nil
		}
	}
	return nil, lastErr
}

// detail fetches one job page and maps it to a Job, returning ok=false when the page fetch
// fails, carries no parseable id, or renders no description — so the caller skips just that
// posting.
func (s avature) detail(ctx context.Context, e CompanyEntry, entry sitemapLoc) (Job, bool) {
	id := avatureJobID(entry.Loc)
	if id == "" {
		return Job{}, false // no native id → would collide on the dedup key; skip it
	}

	root, err := s.http.GetHTML(ctx, entry.Loc)
	if err != nil {
		return Job{}, false
	}

	title := metaProperty(root, "og:title")
	description := sanitizeHTML(avatureDescription(root))
	if title == "" || description == "" {
		return Job{}, false // an unrendered/blocked page → skip rather than persist a husk
	}

	workMode := avatureWorkMode(avatureFieldValue(root, "Work Model"))
	return Job{
		ExternalID:  id,
		URL:         entry.Loc,
		Title:       title,
		Company:     e.Company,
		Location:    avatureLabeledValue(root, "Locations"),
		Description: description,
		Remote:      workMode == "remote",
		WorkMode:    workMode,
		PostedAt:    parseDate(entry.LastMod),
	}, true
}

// avatureJobIDPattern captures the trailing numeric posting id of a JobDetail URL
// (".../JobDetail/<title-slug>/<id>", optional trailing slash).
var avatureJobIDPattern = regexp.MustCompile(`/JobDetail/[^/]+/(\d+)/?$`)

// avatureJobID extracts the native numeric posting id from a job page URL.
func avatureJobID(loc string) string {
	return firstSubmatch(avatureJobIDPattern, loc)
}

// avatureWorkMode maps an Avature "Work Model" field value to the controlled work-mode
// vocabulary, returning "" for an absent or unrecognized value (no structured signal).
func avatureWorkMode(v string) string {
	switch l := strings.ToLower(strings.TrimSpace(v)); {
	case strings.Contains(l, "remote"):
		return "remote"
	case strings.Contains(l, "hybrid"):
		return "hybrid"
	case strings.Contains(l, "on-site"), strings.Contains(l, "on site"),
		strings.Contains(l, "onsite"), strings.Contains(l, "in-office"),
		strings.Contains(l, "in office"):
		return "onsite"
	default:
		return ""
	}
}

// avatureFieldValue returns the trimmed text of the article__content__view__field whose
// label matches label (case-insensitively), or "" when no such field exists.
func avatureFieldValue(root *html.Node, label string) string {
	var out string
	walk(root, func(n *html.Node) bool {
		if out != "" {
			return false
		}
		if n.Type != html.ElementNode || !hasClass(n, "article__content__view__field") {
			return true
		}
		var lab, val string
		walk(n, func(c *html.Node) bool {
			if c.Type != html.ElementNode {
				return true
			}
			switch {
			case hasClass(c, "article__content__view__field__label"):
				lab = strings.TrimSpace(textContent(c))
			case hasClass(c, "article__content__view__field__value"):
				val = strings.TrimSpace(textContent(c))
			}
			return true
		})
		if strings.EqualFold(lab, label) {
			out = val
			return false
		}
		return true
	})
	return out
}

// avatureLabeledValue returns the value of an inline-labeled field — an
// article__content__view__field__value rendered as "<strong>Label</strong>: value"
// (how Avature shows Locations) — for the given label, or "" when no such field exists.
// It is distinct from avatureFieldValue, which reads the sidebar __label/__value pairs.
func avatureLabeledValue(root *html.Node, label string) string {
	var out string
	found := false
	walk(root, func(n *html.Node) bool {
		if found {
			return false
		}
		if n.Type != html.ElementNode || !hasClass(n, "article__content__view__field__value") {
			return true
		}
		var strongText string
		walk(n, func(c *html.Node) bool {
			if strongText == "" && c.Type == html.ElementNode && c.Data == "strong" {
				strongText = strings.TrimSpace(textContent(c))
				return false
			}
			return true
		})
		if !strings.EqualFold(strongText, label) {
			return true
		}
		found = true
		v := textContent(n)
		// Avature appends a hidden multi-location list after the primary value, separated
		// by a non-breaking space; keep only the primary value.
		if i := strings.IndexRune(v, '\u00A0'); i >= 0 {
			v = v[:i]
		}
		v = strings.TrimSpace(v)
		v = strings.TrimSpace(strings.TrimPrefix(v, strongText))
		out = strings.TrimSpace(strings.TrimLeft(v, ":  "))
		return false
	})
	return out
}

// avatureDescription returns the inner HTML of the job ad body — the richest (most text)
// field value that carries prose markup (a paragraph/heading/list). The prose requirement
// keeps a long inline value such as the Locations field's hidden multi-location dump (plain
// text, no block tags) from being mistaken for the description. It falls back to the richest
// value overall if none carries prose markup.
func avatureDescription(root *html.Node) string {
	var prose, any string
	proseLen, anyLen := 0, 0
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || !hasClass(n, "article__content__view__field__value") {
			return true
		}
		l := len(textContent(n))
		if l > anyLen {
			anyLen, any = l, innerHTML(n)
		}
		if l > proseLen && hasProseMarkup(n) {
			proseLen, prose = l, innerHTML(n)
		}
		return true
	})
	if prose != "" {
		return prose
	}
	return any
}

// hasProseMarkup reports whether n contains a block-level prose element (paragraph,
// heading, or list) — the structure of a job ad body, absent from inline-labeled fields.
func hasProseMarkup(n *html.Node) bool {
	found := false
	walk(n, func(c *html.Node) bool {
		if c.Type == html.ElementNode {
			switch c.Data {
			case "p", "ul", "ol", "li", "h1", "h2", "h3", "h4", "h5", "h6", "table":
				found = true
				return false
			}
		}
		return true
	})
	return found
}
