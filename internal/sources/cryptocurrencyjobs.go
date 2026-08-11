package sources

import (
	"context"
	"encoding/xml"
	"fmt"
	"regexp"
	"strings"
	"time"

	xhtml "golang.org/x/net/html"
)

// cryptocurrencyjobs adapts cryptocurrencyjobs.co, a Web3/crypto-niche jobs board. Boardless
// (one public RSS feed, no per-tenant board) and multi-company, so it stays in the source
// facet and takes each posting's company from the feed.
//
// The feed's <description> is only the opening blurb (confirmed live: ~750 chars vs. the
// ~7000-char full posting), not the body — so each item gets a per-posting detail fetch for
// its page's schema.org JobPosting ld+json, same as breezy/teamtailor. A failed or
// description-less detail fetch falls back to the feed's short blurb rather than dropping the
// posting, since unlike breezy's listing (which carries nothing) the feed already gives a
// non-empty description.
//
// The board is mostly but not entirely remote: a posting the board restricts to one office
// (confirmed live, e.g. "Product Designer (New York only)" at Loopscale, whose page reads
// "This role requires working full-time from our New York City office") gets that city
// appended to its title in parens as "(<City> only)", with no "remote" wording anywhere in
// the feed's blurb — unlike a geo-eligibility-restricted remote posting, which this board
// phrases as e.g. "100% remote ... anywhere in Europe" rather than a title suffix.
// cryptocurrencyjobsOnsiteTitle strips that suffix and treats it as an onsite location instead
// of defaulting every posting to remote.
//
// The feed is fetched via GetText rather than GetXML and decoded leniently (Strict=false),
// same as the nodesk adapter: this board runs the same underlying feed generator (identical
// "Role at Company" / plain-text-description / guid-equals-link shape, same host family as
// nodesk.co), which is known to embed raw named entities such as "&rsquo;" outside CDATA —
// invalid XML that the strict decoder rejects. html.UnescapeString then resolves whatever the
// lenient decode left as literal entity text.
type cryptocurrencyjobs struct {
	http cryptocurrencyjobsHTTP
}

// cryptocurrencyjobsHTTP is the transport cryptocurrencyjobs needs: the text feed plus HTML
// detail pages for the full description.
type cryptocurrencyjobsHTTP interface {
	TextGetter
	HTMLGetter
}

const cryptocurrencyjobsFeedURL = "https://cryptocurrencyjobs.co/index.xml"

// cryptocurrencyjobsOnsiteTitle matches the board's "(<City> only)" title suffix that marks a
// posting restricted to one office, e.g. "Product Designer (New York only)" -> "New York".
var cryptocurrencyjobsOnsiteTitle = regexp.MustCompile(`\s*\(([^()]+?)\s+only\)\s*$`)

// NewCryptocurrencyJobs builds the Cryptocurrency Jobs adapter over the given HTTP client.
func NewCryptocurrencyJobs(c cryptocurrencyjobsHTTP) Source { return cryptocurrencyjobs{http: c} }

func (cryptocurrencyjobs) Provider() string { return "cryptocurrencyjobs" }

func (cryptocurrencyjobs) boardless() {}

func (cryptocurrencyjobs) aggregator() {}

// cryptocurrencyjobsItem is one RSS <item>: the title is "Role at Company", description is
// the plain-text body, and guid is the native posting id (same value as link).
type cryptocurrencyjobsItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	GUID        string `xml:"guid"`
	PubDate     string `xml:"pubDate"`
	Description string `xml:"description"`
}

func (s cryptocurrencyjobs) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	raw, err := s.http.GetText(ctx, cryptocurrencyjobsFeedURL)
	if err != nil {
		return nil, fmt.Errorf("cryptocurrencyjobs: feed: %w", err)
	}
	var feed struct {
		Channel struct {
			Items []cryptocurrencyjobsItem `xml:"item"`
		} `xml:"channel"`
	}
	dec := xml.NewDecoder(strings.NewReader(raw))
	dec.Strict = false
	if err := dec.Decode(&feed); err != nil {
		return nil, fmt.Errorf("cryptocurrencyjobs: feed: %w", err)
	}
	return fetchDetails(feed.Channel.Items, defaultDetailWorkers, func(it cryptocurrencyjobsItem) (Job, bool) {
		return s.detail(ctx, it)
	}), nil
}

// detail maps one RSS item to a Job, then fetches its page for the full description —
// falling back to the feed's blurb (already set by toJob) when the page fetch fails or
// carries no JobPosting ld+json.
func (s cryptocurrencyjobs) detail(ctx context.Context, it cryptocurrencyjobsItem) (Job, bool) {
	job, ok := it.toJob()
	if !ok {
		return Job{}, false
	}
	root, err := s.http.GetHTML(ctx, it.Link)
	if err != nil {
		return job, true
	}
	var p struct {
		Description string `json:"description"`
	}
	if ldJobPosting(root, &p) && p.Description != "" {
		job.Description = sanitizeHTML(xhtml.UnescapeString(p.Description))
	}
	return job, true
}

// toJob maps an RSS item to a Job, returning ok=false for an unusable item (no guid to key
// on, or no " at " split which would leave the company empty and break the slug).
func (it cryptocurrencyjobsItem) toJob() (Job, bool) {
	title, company, ok := strings.Cut(it.Title, " at ")
	if it.GUID == "" || !ok || company == "" {
		return Job{}, false
	}
	title = strings.TrimSpace(title)

	location, remote, workMode := "Remote", true, "remote"
	if m := cryptocurrencyjobsOnsiteTitle.FindStringSubmatch(title); m != nil {
		location, remote, workMode = m[1], false, "onsite"
		title = strings.TrimSuffix(title, m[0])
	}

	return Job{
		ExternalID:  it.GUID,
		URL:         it.Link,
		Title:       xhtml.UnescapeString(title),
		Company:     xhtml.UnescapeString(strings.TrimSpace(company)),
		Location:    location,
		Description: sanitizeHTML(xhtml.UnescapeString(it.Description)),
		Remote:      remote,
		WorkMode:    workMode,
		PostedAt:    parseLayout(time.RFC1123Z, it.PubDate),
	}, true
}
