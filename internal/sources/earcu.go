package sources

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// earcu adapts an eArcu career site. eArcu is a multi-tenant UK ATS whose clients each run
// on their own careers host and publish a keyless RSS feed at /jobs/rss. Each feed item
// carries the full posting body inline (a "Key: value, …" metadata prefix followed by a
// <div id="rssjobdesc"> body), so no per-posting request is needed in the common case; when
// an item has no inline body the adapter falls back to the detail page's schema.org
// JobPosting JSON-LD. Unlike Personio the board is the full careers host, not a subdomain.
//
// earcuHTTP is the transport earcu needs: an RSS feed plus HTML detail pages.
type earcuHTTP interface {
	XMLGetter
	HTMLGetter
}

type earcu struct {
	http earcuHTTP
}

// NewEarcu builds the eArcu adapter over the given HTTP client.
func NewEarcu(c earcuHTTP) Source { return earcu{http: c} }

func (earcu) Provider() string { return "earcu" }

// earcuRSS is a board's /jobs/rss document. Only the fields the adapter maps are decoded.
type earcuRSS struct {
	Items []earcuItem `xml:"channel>item"`
}

type earcuItem struct {
	Link        string `xml:"link"`
	Title       string `xml:"title"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
}

// earcuIDRe extracts the stable posting id from a vacancy detail URL of the form
// /jobs/vacancy/<slug>/<id>/description/. Anchored on the final /description so an
// earlier numeric path segment can't be mistaken for the id.
var earcuIDRe = regexp.MustCompile(`/(\d+)/description/?$`)

// earcuRefRe matches the trailing " (<ref>)" eArcu reference suffix in a feed title.
var earcuRefRe = regexp.MustCompile(`\s*\(\d+\)\s*$`)

// earcuLocationRe pulls the Location value out of the metadata prefix; the prefix always
// ends the Location field with ", eArcu Vacancy Reference:", so this tolerates commas inside
// earlier fields (e.g. a thousands-separated salary).
var earcuLocationRe = regexp.MustCompile(`Location:\s*(.+?),\s*eArcu Vacancy Reference:`)

// earcuCountryRe is the fallback when a feed carries no Location field.
var earcuCountryRe = regexp.MustCompile(`Country:\s*([^,]+)`)

func (a earcu) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	url := fmt.Sprintf("https://%s/jobs/rss", e.Board)

	var feed earcuRSS
	if err := a.http.GetXML(ctx, url, &feed); err != nil {
		return nil, fmt.Errorf("earcu: fetch board %s: %w", e.Board, err)
	}

	jobs := make([]Job, 0, len(feed.Items))
	for _, it := range feed.Items {
		m := earcuIDRe.FindStringSubmatch(it.Link)
		if m == nil {
			continue // no stable posting id — skip rather than emit an empty key
		}
		prefix, body := earcuSplit(it.Description)
		location := earcuLocation(prefix)
		description := sanitizeHTML(body)
		if description == "" {
			description = a.detailDescription(ctx, it.Link)
		}
		jobs = append(jobs, Job{
			ExternalID:  m[1],
			URL:         it.Link,
			Title:       strings.TrimSpace(earcuRefRe.ReplaceAllString(it.Title, "")),
			Company:     e.Company,
			Location:    location,
			Description: description,
			Remote:      isRemote(location), // the feed has no structured remote flag
			PostedAt:    earcuPubDate(it.PubDate),
		})
	}
	return jobs, nil
}

// earcuLocation returns the posting's location from the feed item's metadata prefix,
// preferring the Location field and falling back to Country; empty when neither is present.
func earcuLocation(desc string) string {
	if m := earcuLocationRe.FindStringSubmatch(desc); m != nil {
		return strings.TrimSpace(m[1])
	}
	if m := earcuCountryRe.FindStringSubmatch(desc); m != nil {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// earcuSplit divides a feed item's description into the metadata prefix (the leading
// "Key: value, …" text) and the inline body (the rssjobdesc block, "" when absent).
// Splitting once keeps location parsing off the HTML body, where a stray "Country:"/
// "Location:" in the posting text would otherwise be mistaken for metadata.
func earcuSplit(desc string) (prefix, body string) {
	if i := strings.Index(desc, `<div id="rssjobdesc"`); i >= 0 {
		return desc[:i], desc[i:]
	}
	return desc, ""
}

// detailDescription fetches a posting's detail page and returns its schema.org JobPosting
// body, sanitized; "" when the page fetch fails or carries no such block.
func (a earcu) detailDescription(ctx context.Context, url string) string {
	root, err := a.http.GetHTML(ctx, url)
	if err != nil {
		return ""
	}
	var ld struct {
		Description string `json:"description"`
	}
	if !ldJobPosting(root, &ld) || ld.Description == "" {
		return ""
	}
	return sanitizeHTML(html.UnescapeString(ld.Description))
}

// earcuPubDate parses an eArcu feed pubDate. eArcu emits an RFC822 date whose zone is a
// literal "Z" (UTC), sometimes with an unpadded day, so the primary layout uses a
// numeric-offset zone (which also accepts an explicit +hhmm) and an unpadded day; a feed
// using a named zone (GMT/MST) falls back to the standard RFC1123 forms.
func earcuPubDate(s string) *time.Time {
	if t := parseLayout("Mon, 2 Jan 2006 15:04:05 Z0700", s); t != nil {
		return t
	}
	return parsePubDate(s)
}
