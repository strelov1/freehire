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
// detail call). The all-jobs feed is capped at the newest 100, but the per-category feeds are
// not (1070 distinct postings across them on 2026-11-24, against 100 in the all-jobs feed),
// so the crawl reads every category feed and de-duplicates by the posting slug.
type weworkremotely struct {
	http XMLGetter
}

const weworkremotelyFeedURL = "https://weworkremotely.com/remote-jobs.rss"

// weworkremotelyCategoryFeeds are the site's category feeds (the footer of /remote-jobs lists
// these ten). A new category would be missed until added, but the all-jobs feed is read too,
// so its newest 100 never are.
var weworkremotelyCategoryFeeds = []string{
	"all-other-remote-jobs", "remote-back-end-programming-jobs", "remote-customer-support-jobs",
	"remote-design-jobs", "remote-devops-sysadmin-jobs", "remote-front-end-programming-jobs",
	"remote-full-stack-programming-jobs", "remote-management-and-finance-jobs",
	"remote-product-jobs", "remote-sales-and-marketing-jobs",
}

const weworkremotelyCategoryURL = "https://weworkremotely.com/categories/%s.rss"

// NewWeWorkRemotely builds the WeWorkRemotely adapter over the given HTTP client.
func NewWeWorkRemotely(c XMLGetter) Source { return weworkremotely{http: c} }

func (weworkremotely) Provider() string { return "weworkremotely" }

func (weworkremotely) Boardless() {}

func (weworkremotely) Aggregator() {}

// wwrItem is one RSS <item>: the title is "Company: Role", region is the location, and the
// description carries the HTML body inline. Type is the contract kind. Country/State are the
// EMPLOYER's place (empty on most items, and at odds with Region on others), and ExpiresAt is
// a listing lapse, not an application deadline: none of the three is mapped.
type wwrItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Region      string `xml:"region"`
	Country     string `xml:"country"`
	State       string `xml:"state"`
	Type        string `xml:"type"`
	PubDate     string `xml:"pubDate"`
	ExpiresAt   string `xml:"expires_at"`
	Description string `xml:"description"`
}

func (s weworkremotely) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	urls := []string{weworkremotelyFeedURL}
	for _, c := range weworkremotelyCategoryFeeds {
		urls = append(urls, fmt.Sprintf(weworkremotelyCategoryURL, c))
	}
	var jobs []Job
	seen := map[string]bool{}
	for i, u := range urls {
		var feed struct {
			Channel struct {
				Items []wwrItem `xml:"item"`
			} `xml:"channel"`
		}
		if err := s.http.GetXML(ctx, u, &feed); err != nil {
			// A half-read set would make every posting of the missed category look gone.
			return nil, fmt.Errorf("weworkremotely: feed %d: %w", i, err)
		}
		for _, it := range feed.Channel.Items {
			if job, ok := it.toJob(); ok && !seen[job.ExternalID] {
				seen[job.ExternalID] = true
				jobs = append(jobs, job)
			}
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
		Remote:         true,
		WorkMode:       "remote",
		PostedAt:       parseLayout(time.RFC1123Z, it.PubDate),
		EmploymentType: wwrEmploymentType(it.Type),
	}, true
}

// wwrEmploymentType maps the feed's <type> ("Full-Time", "Contract") into
// vocab.EmploymentTypeValues; any other value states no type.
func wwrEmploymentType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "full-time":
		return "full_time"
	case "part-time":
		return "part_time"
	case "contract":
		return "contract"
	}
	return ""
}
