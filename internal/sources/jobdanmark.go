package sources

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// jobdanmark adapts JobiDanmark (jobdanmark.dk), a Danish job portal. Its keyless search API
// (POST /api/jobsearch/search/{page}) returns a paginated catalogue where every item carries its
// own employer, but NOT the job body — so, like the other list-without-description adapters
// (SmartRecruiters/Rippling), each item is completed from its detail page, whose server-rendered
// schema.org JobPosting (a JSON-LD block) supplies the full description and a clean datePosted.
// Like the other national feeds it is boardless (one API, no per-tenant board) and an aggregator
// (many employers; the company comes from the item). It covers every sector, not just IT — the
// same as jobtech/jobnet — so the downstream dictionaries and the enrich non-tech gate decide
// relevance, not the adapter.
type jobdanmark struct {
	http jobdanmarkClient
}

// jobdanmarkClient is the transport role jobdanmark needs: a POST list plus an HTML detail page.
type jobdanmarkClient interface {
	JSONPoster
	HTMLGetter
}

const (
	jobdanmarkBaseURL = "https://jobdanmark.dk"
	// jobdanmarkSearchURL is the POST search, paged in the path. An empty filter body returns
	// the whole catalogue freshest-first.
	jobdanmarkSearchURL = "https://jobdanmark.dk/api/jobsearch/search/%d"
	// jobdanmarkMaxPages bounds pagination so a wrong or missing totalPages cannot loop. At
	// 30/page it covers ~30k ads, well above the live catalogue (~14k); the empty page the API
	// returns past the end is the real terminator.
	jobdanmarkMaxPages = 1000
)

// NewJobdanmark builds the JobiDanmark adapter over the given POST+HTML client.
func NewJobdanmark(c jobdanmarkClient) Source { return jobdanmark{http: c} }

func (jobdanmark) Provider() string { return "jobdanmark" }

// jobdanmark is a national portal with one global feed, so its config entry carries no board.
func (jobdanmark) boardless() {}

// jobdanmark aggregates postings from many employers, so it stays in the source facet.
func (jobdanmark) aggregator() {}

// jobdanmarkSearchBody is the POST search request. An empty JobTypes/Filters with the Text
// location mode returns the unfiltered catalogue.
type jobdanmarkSearchBody struct {
	JobTypes     []string `json:"jobTypes"`
	Filters      []any    `json:"filters"`
	LocationMode string   `json:"locationMode"`
	Distance     int      `json:"distance"`
}

// jobdanmarkSearchResponse is one search page: TotalPages bounds pagination; Items is the page.
type jobdanmarkSearchResponse struct {
	Items      []jobdanmarkItem `json:"items"`
	TotalPages int              `json:"totalPages"`
}

// jobdanmarkItem is one list posting. The list omits the body; CompanyAddress is a free-text
// address without a country (the portal is Danish, so the country is appended for the geo
// dictionary); URL is a site-relative "/job/<slug>".
type jobdanmarkItem struct {
	Title          string `json:"title"`
	CompanyName    string `json:"companyName"`
	CompanyAddress string `json:"companyAddress"`
	URL            string `json:"url"`
	PublishedDate  string `json:"publishedDate"`
}

// Fetch pages the whole catalogue, then fetches each item's detail page concurrently for the
// description (bounded worker pool). A first-page failure is a board error; a later page failing
// ends enumeration with the items gathered so far.
func (s jobdanmark) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	items, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	return fetchDetails(items, defaultDetailWorkers, func(it jobdanmarkItem) (Job, bool) {
		return s.toJob(ctx, it)
	}), nil
}

// list pages the POST search freshest-first, stopping on an empty page or when the last page is
// reached.
func (s jobdanmark) list(ctx context.Context) ([]jobdanmarkItem, error) {
	body := jobdanmarkSearchBody{JobTypes: []string{}, Filters: []any{}, LocationMode: "Text", Distance: 50}
	var items []jobdanmarkItem
	for page := 1; page <= jobdanmarkMaxPages; page++ {
		var resp jobdanmarkSearchResponse
		if err := s.http.PostJSON(ctx, fmt.Sprintf(jobdanmarkSearchURL, page), body, &resp); err != nil {
			if page == 1 {
				return nil, fmt.Errorf("jobdanmark: search page %d: %w", page, err)
			}
			break // a later page failing ends enumeration with the items gathered so far
		}
		if len(resp.Items) == 0 {
			break
		}
		items = append(items, resp.Items...)
		if resp.TotalPages > 0 && page >= resp.TotalPages {
			break
		}
	}
	return items, nil
}

// toJob maps a list item plus its detail page to a Job, returning ok=false for an item with no
// title, employer (which would break the company slug), or url to key on. The detail supplies the
// body and a clean datePosted; a failed detail leaves the description empty and falls back to the
// list's date rather than dropping the (otherwise complete) posting.
func (s jobdanmark) toJob(ctx context.Context, it jobdanmarkItem) (Job, bool) {
	// Strip any query/fragment for both the slug and the stored URL; trim a trailing slash so a
	// directory-style path still yields the last segment as the id.
	path := strings.TrimRight(trimURLSuffix(it.URL), "/")
	if it.Title == "" || it.CompanyName == "" || path == "" {
		return Job{}, false
	}
	slug := path[strings.LastIndex(path, "/")+1:]
	if slug == "" {
		return Job{}, false
	}
	jobURL := path
	if !strings.HasPrefix(jobURL, "http") {
		jobURL = jobdanmarkBaseURL + jobURL
	}
	desc, posted := s.detail(ctx, jobURL)
	if posted == nil {
		posted = parseLayout("02-01-2006", it.PublishedDate) // list date is DD-MM-YYYY
	}
	return Job{
		ExternalID:  slug,
		URL:         jobURL,
		Title:       it.Title,
		Company:     it.CompanyName,
		Description: sanitizeHTML(desc),
		// The address omits the country; jobdanmark is a Danish portal, so append it for the
		// geo dictionary (a rare cross-border address still resolves its own country too).
		Location: joinNonEmpty(it.CompanyAddress, "Danmark"),
		PostedAt: posted,
	}, true
}

// detail fetches the item's detail page and reads the schema.org JobPosting JSON-LD, returning
// its description and datePosted. A failed request or a page with no JobPosting yields an empty
// description and a nil date (the caller falls back to the list date) — a posting is never dropped
// over a missing detail.
func (s jobdanmark) detail(ctx context.Context, jobURL string) (string, *time.Time) {
	root, err := s.http.GetHTML(ctx, jobURL)
	if err != nil {
		return "", nil
	}
	var ld struct {
		Description string `json:"description"`
		DatePosted  string `json:"datePosted"`
	}
	if !ldJobPosting(root, &ld) {
		return "", nil
	}
	// datePosted is date-only ("2026-07-07") on the live page; tolerate a full ISO timestamp
	// too so the detail date keeps winning if the format ever gains a time component.
	posted := parseDate(ld.DatePosted)
	if posted == nil {
		posted = parseRFC3339(ld.DatePosted)
	}
	return ld.Description, posted
}
