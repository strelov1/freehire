package sources

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// jobspresso adapts jobspresso.co, a curated remote-jobs board running WordPress Job Manager.
// Boardless (one public RSS feed, no per-tenant board) and multi-company, so it stays in the
// source facet and takes each posting's company from the feed. The RSS carries every posting's
// body inline (no detail call); the feed is the recent window, not the full backlog. jobspresso
// lists only remote work, so every posting maps to remote.
type jobspresso struct {
	http XMLGetter
}

const jobspressoFeedURL = "https://jobspresso.co/feed/?post_type=job_listing"

// NewJobspresso builds the Jobspresso adapter over the given HTTP client.
func NewJobspresso(c XMLGetter) Source { return jobspresso{http: c} }

func (jobspresso) Provider() string { return "jobspresso" }

func (jobspresso) boardless() {}

func (jobspresso) aggregator() {}

// jobspressoItem is one RSS <item>. Company and location are packed into <dc:creator>, the
// numeric post id lives in the <guid>'s p= param, and the body is in <content:encoded> (with a
// truncated <description> summary as fallback). The dc/content-namespaced elements are matched by
// their local name, which Go's encoding/xml resolves regardless of prefix.
type jobspressoItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	GUID        string `xml:"guid"`
	Creator     string `xml:"creator"` // dc:creator
	Encoded     string `xml:"encoded"` // content:encoded
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
}

func (s jobspresso) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	var feed struct {
		Channel struct {
			Items []jobspressoItem `xml:"item"`
		} `xml:"channel"`
	}
	if err := s.http.GetXML(ctx, jobspressoFeedURL, &feed); err != nil {
		return nil, fmt.Errorf("jobspresso: feed: %w", err)
	}
	jobs := make([]Job, 0, len(feed.Channel.Items))
	for _, it := range feed.Channel.Items {
		if job, ok := it.toJob(); ok {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

// toJob maps an RSS item to a Job, returning ok=false for an unusable item — no id (neither a
// guid p= nor a link slug) would leave the dedup key empty, and an empty company would break the
// public slug.
func (it jobspressoItem) toJob() (Job, bool) {
	id := jobspressoID(it.GUID, it.Link)
	company, location := jobspressoCompanyLocation(it.Creator)
	if id == "" || company == "" {
		return Job{}, false
	}
	body := it.Encoded
	if strings.TrimSpace(body) == "" {
		body = it.Description
	}
	return Job{
		ExternalID:  id,
		URL:         strings.TrimSpace(it.Link),
		Title:       strings.TrimSpace(it.Title),
		Company:     company,
		Location:    location,
		Description: sanitizeHTML(html.UnescapeString(body)),
		Remote:      true,
		WorkMode:    "remote",
		PostedAt:    parsePubDate(it.PubDate),
	}, true
}

// jobspressoID is the native post id: the numeric p= param from the guid URL, falling back to
// the last path segment (post slug) of the item link when the guid carries no p=.
func jobspressoID(guid, link string) string {
	if u, err := url.Parse(strings.TrimSpace(guid)); err == nil {
		if p := strings.TrimSpace(u.Query().Get("p")); p != "" {
			return p
		}
	}
	link = strings.TrimRight(strings.TrimSpace(link), "/")
	if i := strings.LastIndex(link, "/"); i >= 0 {
		return link[i+1:]
	}
	return ""
}

// jobspressoCompanyLocation splits the <dc:creator> string, whose form is
// "Company<br>⚲&nbsp;Location". The left of the <br> is the company; the right is the location
// after dropping the ⚲ marker (a creator with no <br> is company-only). dc:creator is CDATA, so
// encoding/xml leaves its entities intact — both halves are HTML-unescaped here (e.g. a company
// "AT&amp;T" or the location's &nbsp;).
func jobspressoCompanyLocation(creator string) (company, location string) {
	company, rest, found := strings.Cut(creator, "<br>")
	company = strings.TrimSpace(html.UnescapeString(company))
	if !found {
		return company, ""
	}
	rest = strings.TrimPrefix(strings.TrimSpace(rest), "⚲")
	location = strings.TrimSpace(html.UnescapeString(rest))
	return company, location
}
