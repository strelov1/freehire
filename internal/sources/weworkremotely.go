package sources

import (
	"context"
	"fmt"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// weworkremotely adapts weworkremotely.com, a remote-jobs board. Boardless (one public RSS
// feed, no per-tenant board) and multi-company, so it stays in the source facet and takes
// each posting's company from the feed. The RSS carries every posting's body inline (no
// detail call); the feed is the recent window, not the full backlog.
type weworkremotely struct {
	http XMLGetter
}

const weworkremotelyFeedURL = "https://weworkremotely.com/remote-jobs.rss"

// NewWeWorkRemotely builds the WeWorkRemotely adapter over the given HTTP client.
func NewWeWorkRemotely(c XMLGetter) Source { return weworkremotely{http: c} }

func (weworkremotely) Provider() string { return "weworkremotely" }

func (weworkremotely) boardless() {}

func (weworkremotely) aggregator() {}

// wwrItem is one RSS <item>: the title is "Company: Role", region is the location, and the
// description carries the HTML body inline.
type wwrItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Region      string `xml:"region"`
	PubDate     string `xml:"pubDate"`
	Description string `xml:"description"`
}

func (s weworkremotely) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	var feed struct {
		Channel struct {
			Items []wwrItem `xml:"item"`
		} `xml:"channel"`
	}
	if err := s.http.GetXML(ctx, weworkremotelyFeedURL, &feed); err != nil {
		return nil, fmt.Errorf("weworkremotely: feed: %w", err)
	}
	jobs := make([]Job, 0, len(feed.Channel.Items))
	for _, it := range feed.Channel.Items {
		if job, ok := it.toJob(); ok {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

// wwrJobID is the native posting id: the last path segment of the item link (the slug,
// e.g. ".../remote-jobs/proxify-ab-senior-fullstack-developer-python-3").
func wwrJobID(link string) string {
	link = strings.TrimRight(link, "/")
	if i := strings.LastIndex(link, "/"); i >= 0 {
		return link[i+1:]
	}
	return ""
}

// toJob maps an RSS item to a Job, returning ok=false for an unusable item (no id from the
// link, or no "Company: Role" split which would leave the company empty and break the slug).
func (it wwrItem) toJob() (Job, bool) {
	id := wwrJobID(it.Link)
	company, title, ok := strings.Cut(it.Title, ": ")
	if id == "" || !ok || company == "" {
		return Job{}, false
	}
	return Job{
		ExternalID:  id,
		URL:         it.Link,
		Title:       strings.TrimSpace(title),
		Company:     strings.TrimSpace(company),
		Location:    it.Region,
		Description: sanitizeHTML(html.UnescapeString(it.Description)),
		// WeWorkRemotely lists only remote jobs.
		Remote:   true,
		WorkMode: "remote",
		PostedAt: parseLayout(time.RFC1123Z, it.PubDate),
	}, true
}
