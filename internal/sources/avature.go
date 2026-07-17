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
type avatureHTTP interface {
	XMLGetter
	HTMLGetter
}

type avature struct {
	http avatureHTTP
}

// NewAvature builds the Avature adapter over the given HTTP client.
func NewAvature(c avatureHTTP) Source { return avature{http: c} }

func (avature) Provider() string { return "avature" }

// avatureEntry is one JobDetail URL plus its sitemap last-modified date (used as posted_at).
type avatureEntry struct {
	loc     string
	lastMod string
}

func (s avature) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var index struct {
		Sitemaps []struct {
			Loc string `xml:"loc"`
		} `xml:"sitemap"`
	}
	indexURL := fmt.Sprintf("https://%s/careers/sitemap_index.xml", e.Board)
	if err := s.http.GetXML(ctx, indexURL, &index); err != nil {
		return nil, fmt.Errorf("avature: sitemap index %s: %w", e.Board, err)
	}

	// One sitemap per locale lists the same postings; crawl en_US to avoid duplicates.
	var listURL string
	for _, sm := range index.Sitemaps {
		if strings.Contains(sm.Loc, "/en_US/") {
			listURL = sm.Loc
			break
		}
	}
	if listURL == "" {
		return nil, fmt.Errorf("avature: no en_US sitemap advertised for %s", e.Board)
	}

	var urlset struct {
		URLs []struct {
			Loc     string `xml:"loc"`
			LastMod string `xml:"lastmod"`
		} `xml:"url"`
	}
	if err := s.http.GetXML(ctx, listURL, &urlset); err != nil {
		return nil, fmt.Errorf("avature: sitemap %s: %w", listURL, err)
	}

	// The locale sitemap mixes JobDetail pages with utility pages (AgentCreate, …); keep
	// only the postings.
	var entries []avatureEntry
	for _, u := range urlset.URLs {
		if strings.Contains(u.Loc, "/careers/JobDetail/") {
			entries = append(entries, avatureEntry{loc: u.Loc, lastMod: u.LastMod})
		}
	}

	return fetchDetails(entries, defaultDetailWorkers, func(entry avatureEntry) (Job, bool) {
		return s.detail(ctx, e, entry)
	}), nil
}

// detail fetches one job page and maps it to a Job, returning ok=false when the page fetch
// fails, carries no parseable id, or renders no description — so the caller skips just that
// posting.
func (s avature) detail(ctx context.Context, e CompanyEntry, entry avatureEntry) (Job, bool) {
	id := avatureJobID(entry.loc)
	if id == "" {
		return Job{}, false // no native id → would collide on the dedup key; skip it
	}

	root, err := s.http.GetHTML(ctx, entry.loc)
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
		URL:         entry.loc,
		Title:       title,
		Company:     e.Company,
		Location:    avatureLabeledValue(root, "Locations"),
		Description: description,
		Remote:      workMode == "remote",
		WorkMode:    workMode,
		PostedAt:    parseDate(entry.lastMod),
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
