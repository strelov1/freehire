package sources

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// likeit adapts a Likeit (Saarni Likeit Oy) recruiting board. Likeit is a Finnish HR/ERP
// platform for staffing agencies; each tenant runs its board on its own subdomain
// (<board>.likeit.fi) and publishes a keyless RSS feed of open adverts at
// /jsp/duuni/AdvertRss.jsp. Every item carries the full posting inline — title, an
// AdvertShow.jsp?id=<id> detail link (the id is the stable native posting id), the location
// in <category>, and the escaped HTML body in <description> — so one request per board
// yields every posting with no per-advert detail fetch. The board is the tenant subdomain,
// e.g. "rekrymesta" or "go-on"; Company is the staffing agency running the board.
type likeit struct {
	http XMLGetter
}

// NewLikeit builds the Likeit adapter over the given HTTP client.
func NewLikeit(c XMLGetter) Source { return likeit{http: c} }

func (likeit) Provider() string { return "likeit" }

// likeitRSS is a board's AdvertRss.jsp document. Only the fields the adapter maps are decoded;
// <category> holds the location (a "region, city" string), not a job category.
type likeitRSS struct {
	Items []likeitItem `xml:"channel>item"`
}

type likeitItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	Category    string `xml:"category"`
	PubDate     string `xml:"pubDate"`
	// Date is the dc:date element (matched on local name); it is the cleaner RFC3339 form
	// of pubDate and the preferred posted_at.
	Date string `xml:"date"`
}

func (a likeit) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	feedURL := fmt.Sprintf("https://%s.likeit.fi/jsp/duuni/AdvertRss.jsp", e.Board)

	var feed likeitRSS
	if err := a.http.GetXML(ctx, feedURL, &feed); err != nil {
		return nil, fmt.Errorf("likeit: fetch board %s: %w", e.Board, err)
	}

	jobs := make([]Job, 0, len(feed.Items))
	for _, it := range feed.Items {
		id := likeitID(it.Link)
		if id == "" {
			continue // no posting id (e.g. a listing link) — skip rather than emit an empty key
		}
		location := strings.TrimSpace(it.Category)
		jobs = append(jobs, Job{
			ExternalID:  id,
			URL:         it.Link,
			Title:       strings.TrimSpace(it.Title),
			Company:     e.Company,
			Location:    location,
			Description: sanitizeHTML(it.Description),
			Remote:      isRemote(location), // the feed has no structured remote flag
			PostedAt:    likeitDate(it.Date, it.PubDate),
		})
	}
	return jobs, nil
}

// likeitID returns the native posting id from an AdvertShow.jsp?id=<id> link, or "" when the
// link carries no id (e.g. the channel's AdvertList link). Reading the query key is
// order-independent, so a "?guest=1&id=…" link resolves the same as "?id=…&guest=1".
func likeitID(link string) string {
	u, err := url.Parse(link)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(u.Query().Get("id"))
}

// likeitDate parses a Likeit item's posted_at, preferring the RFC3339 dc:date and falling
// back to the RFC1123 pubDate.
func likeitDate(dcDate, pubDate string) *time.Time {
	if t := parseRFC3339(dcDate); t != nil {
		return t
	}
	return parsePubDate(pubDate)
}
