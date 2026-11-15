package sources

import (
	"context"
	"fmt"
	"strings"
)

// gethired adapts the GetHired ATS, whose career sites live on easyapply.co. Each employer is
// a tenant subdomain (<board>.easyapply.co) that publishes every open position in one RSS
// document at /rss, body inline per item — one GET per board, no pagination and no detail
// request. The feed is the complete listing even when the HTML career page paginates.
//
// A posting's identity is the slug in its <guid> (https://easyapply.co/job/<slug>). The
// platform mints it once, globally unique across tenants (a repeated title gets a numeric
// suffix, "administrative-assistant-3213"), and does not rewrite it when the title is edited
// — a live feed carries "…veterinarian-west-1" under the title "…Veterinarian East". The
// link 302s to the posting's apply page (/a/<uuid>), which is the URL other sites carry.
//
// The <category> is the posting's location ("Calgary, AB", or "Remote"); the feed has no
// country, salary, employment type or workplace enum, so those are left to the pipeline's
// dictionaries.
type gethired struct {
	http XMLResolvedGetter
}

// NewGetHired builds the GetHired adapter over the given HTTP client.
func NewGetHired(c XMLResolvedGetter) Source { return gethired{http: c} }

func (gethired) Provider() string { return "gethired" }

// gethiredFeed is the RSS document a tenant publishes: a single channel of items.
type gethiredFeed struct {
	Items []gethiredItem `xml:"channel>item"`
}

// gethiredItem is one open position. The description is HTML inside CDATA, so it arrives
// unescaped.
type gethiredItem struct {
	Title       string `xml:"title"`
	GUID        string `xml:"guid"`
	PubDate     string `xml:"pubDate"`
	Description string `xml:"description"`
	Location    string `xml:"category"`
}

func (g gethired) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	url := gethiredFeedURL(e.Board)

	var feed gethiredFeed
	final, err := g.http.GetXMLResolved(ctx, url, &feed)
	if err != nil {
		// A tenant the platform does not serve 302s to its sign-in page, so the decode
		// fails on HTML. The redirect is checked first because the decode error is what it
		// CAUSES and says nothing about the board.
		if gethiredBoardGone(e.Board, final) {
			return nil, fmt.Errorf("gethired: board %q: %w (redirected to %s)", e.Board, ErrBoardGone, final)
		}
		return nil, fmt.Errorf("gethired: fetch board %s: %w", e.Board, err)
	}

	jobs := make([]Job, 0, len(feed.Items))
	for _, it := range feed.Items {
		id := gethiredJobSlug(it.GUID)
		if id == "" {
			continue
		}
		location := strings.TrimSpace(it.Location)
		jobs = append(jobs, Job{
			ExternalID:  id,
			URL:         "https://easyapply.co/job/" + id,
			Title:       strings.TrimSpace(it.Title),
			Company:     e.Company,
			Location:    location,
			Description: sanitizeHTML(it.Description),
			Remote:      isRemote(location),
			PostedAt:    parsePubDate(it.PubDate),
		})
	}
	return jobs, nil
}

// gethiredFeedURL is the RSS feed of one tenant.
func gethiredFeedURL(board string) string {
	return fmt.Sprintf("https://%s.easyapply.co/rss", board)
}

// gethiredBoardGone reports whether the feed request was redirected off the tenant's feed —
// the platform's answer for a subdomain it does not serve (it lands on /login).
func gethiredBoardGone(board, final string) bool {
	return final != "" && !strings.HasPrefix(final, gethiredFeedURL(board))
}

// gethiredJobSlug extracts the posting slug from a GetHired job link
// (https://easyapply.co/job/produce-clerk-120 → produce-clerk-120), or "" when the link has
// no /job/ segment.
func gethiredJobSlug(link string) string {
	_, slug, ok := strings.Cut(strings.TrimSpace(link), "/job/")
	if !ok {
		return ""
	}
	return strings.Trim(slug, "/")
}
