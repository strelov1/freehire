package sources

import (
	"context"
	"fmt"
	"html"
	"strings"
)

// trakstar adapts the Trakstar Hire (formerly Recruiterbox) public RSS job feed. Each
// board is its own tenant subdomain and publishes every open position in one RSS
// document with the body inline per item — so one GET per board, no pagination and no
// per-posting detail request. The job: namespace carries structured location and
// position-type fields alongside the standard RSS item fields.
type trakstar struct {
	http XMLGetter
}

// NewTrakstar builds the Trakstar Hire adapter over the given HTTP client.
func NewTrakstar(c XMLGetter) Source { return trakstar{http: c} }

func (trakstar) Provider() string { return "trakstar" }

// trakstarFeed is the RSS document a board publishes: a single channel of items.
type trakstarFeed struct {
	Items []trakstarItem `xml:"channel>item"`
}

// trakstarItem is one open position. The job: namespaced elements are bound by their
// full namespace URI (https://recruiterbox.com/rss/job/) so encoding/xml matches them
// regardless of the prefix the feed declares.
type trakstarItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	PubDate     string `xml:"pubDate"`
	Description string `xml:"description"`
	City        string `xml:"https://recruiterbox.com/rss/job/ locationCity"`
	State       string `xml:"https://recruiterbox.com/rss/job/ locationState"`
	Country     string `xml:"https://recruiterbox.com/rss/job/ locationCountry"`
}

func (t trakstar) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	url := fmt.Sprintf("https://%s.hire.trakstar.com/jobfeeds/%s", e.Board, e.Board)

	var feed trakstarFeed
	if err := t.http.GetXML(ctx, url, &feed); err != nil {
		return nil, fmt.Errorf("trakstar: fetch board %s: %w", e.Board, err)
	}

	jobs := make([]Job, 0, len(feed.Items))
	for _, it := range feed.Items {
		jobs = append(jobs, Job{
			ExternalID:  jobIDFromLink(it.Link),
			URL:         strings.Replace(it.Link, "http://", "https://", 1),
			Title:       it.Title,
			Company:     e.Company,
			Location:    joinNonEmpty(it.City, it.State, it.Country),
			Description: sanitizeHTML(html.UnescapeString(it.Description)),
			Remote:      isRemote(it.City),
			PostedAt:    parsePubDate(it.PubDate),
		})
	}
	return jobs, nil
}

// jobIDFromLink extracts the posting id from a Trakstar job URL's last path segment
// (e.g. http://<board>.hire.trakstar.com/jobs/fk0zjzb → fk0zjzb).
func jobIDFromLink(link string) string {
	link = strings.TrimRight(link, "/")
	if i := strings.LastIndex(link, "/"); i >= 0 {
		return link[i+1:]
	}
	return link
}
