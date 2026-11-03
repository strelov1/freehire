package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/net/html"
)

// softgarden adapts softgarden career sites. The board is the tenant subdomain (e.g.
// "bundesdruckerei").
//
// The primary path is the tenant's machine feed, "<board>.career.softgarden.de/jobs.feed.json":
// a schema.org DataFeed of JobPosting items that carries every open posting WITH its full
// description in one request, so a tenant costs one fetch however many postings it holds and
// is never truncated to a first page.
//
// A tenant whose feed is not served (400/404 — the feed host does not always carry the same
// tenant name as "<board>.softgarden.io") falls back to the legacy HTML path: the root page
// of "<board>.softgarden.io" is a server-rendered (Apache Wicket) listing whose anchors link
// each posting at /job/<id>/<slug>?jobDbPVId=<n>, and each job page carries a schema.org
// JobPosting ld+json block, so the description comes from a per-job detail fetch
// (bounded-concurrency). That listing is a single page: softgarden paginates via a Wicket
// AJAX callback rather than a stable ?page=N URL, so a large tenant is truncated to it.
//
// Both paths yield the same native posting id, so a board moving between them updates its
// stored rows in place.
type softgarden struct {
	http softgardenHTTP
}

// softgardenHTTP is the transport roles the adapter needs: the JSON feed, and HTML for the
// legacy fallback.
type softgardenHTTP interface {
	HTMLGetter
	JSONGetter
}

// NewSoftgarden builds the softgarden adapter over the given HTTP client.
func NewSoftgarden(c softgardenHTTP) Source { return softgarden{http: c} }

func (softgarden) Provider() string { return "softgarden" }

func (s softgarden) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	jobs, err := s.fetchFeed(ctx, e)
	var se *StatusError
	if errors.As(err, &se) && (se.Code == http.StatusBadRequest || se.Code == http.StatusNotFound) {
		return s.fetchListing(ctx, e) // this tenant has no feed under that name
	}
	return jobs, err
}

// fetchListing is the legacy path: the HTML listing, then one detail page per posting.
func (s softgarden) fetchListing(ctx context.Context, e CompanyEntry) ([]Job, error) {
	// base carries the scheme+host; the listing's relative job hrefs ("../job/…") resolve
	// against it into fetchable absolute URLs.
	base, err := url.Parse(fmt.Sprintf("https://%s.softgarden.io/", e.Board))
	if err != nil {
		return nil, fmt.Errorf("softgarden: board %q: %w", e.Board, err)
	}

	root, err := s.http.GetHTML(ctx, base.String())
	if err != nil {
		return nil, fmt.Errorf("softgarden: listing %s: %w", e.Board, err)
	}
	urls := jobLinks(base, root, func(href string) bool { return sgJobID(href) != "" })

	// Each job's posting comes from its own page fetch, fanned out under a bounded pool.
	return fetchDetails(urls, defaultDetailWorkers, func(u string) (Job, bool) {
		return s.detail(ctx, e, u)
	}), nil
}

// detail fetches one job page and maps its JobPosting ld+json to a Job, returning ok=false
// when the page fetch fails, carries no JobPosting, or has no parseable id, so the caller
// skips just that posting.
func (s softgarden) detail(ctx context.Context, e CompanyEntry, jobURL string) (Job, bool) {
	id := sgJobID(jobURL)
	if id == "" {
		return Job{}, false // no native id → would collide on the dedup key; skip it
	}
	root, err := s.http.GetHTML(ctx, jobURL)
	if err != nil {
		return Job{}, false
	}
	var p sgPosting
	if !ldJobPosting(root, &p) {
		return Job{}, false
	}

	location := p.JobLocation.Address.Location()
	// softgarden's JobPosting carries no explicit remote flag, so remote is inferred from the
	// location only (never the title, which false-positives on "Remote …" role names).
	remote := isRemote(location)

	return Job{
		ExternalID:  id,
		URL:         jobURL,
		Title:       p.Title,
		Company:     e.Company,
		Location:    location,
		Description: sanitizeHTML(html.UnescapeString(p.Description)),
		Remote:      remote,
		WorkMode:    workModeFromRemote(remote),
		PostedAt:    parseRFC3339(p.DatePosted),
	}, true
}

// sgJobIDPattern captures the native posting id from a job URL's /job/<id> segment. The
// permalink is /job/<id>/<slug>?jobDbPVId=<n>; the id is the numeric segment right after
// /job/, and the trailing boundary keeps non-job paths from matching.
var sgJobIDPattern = regexp.MustCompile(`/job/(\d+)(?:[/?#]|$)`)

// sgJobID extracts the native posting id from a job page URL, or "" when the URL is not a
// job permalink.
func sgJobID(u string) string { return firstSubmatch(sgJobIDPattern, u) }

// sgPosting is the schema.org JobPosting decoded from a softgarden job page's
// application/ld+json block. jobLocation is a single Place object.
type sgPosting struct {
	Title       string      `json:"title"`
	Description string      `json:"description"`
	DatePosted  string      `json:"datePosted"`
	JobLocation schemaPlace `json:"jobLocation"`
}

// sgFeed is the part of a softgarden jobs.feed.json the adapter reads: a schema.org DataFeed
// whose items are JobPostings.
type sgFeed struct {
	Items []struct {
		Item sgFeedPosting `json:"item"`
	} `json:"dataFeedElement"`
}

type sgFeedPosting struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
	DatePosted  string `json:"datePosted"`
	Identifier  struct {
		Value json.Number `json:"value"`
	} `json:"identifier"`
	// jobLocation is one Place or an array of them.
	JobLocation schemaPlaces `json:"jobLocation"`
}

// fetchFeed reads the tenant's whole board from its jobs.feed.json. A posting without a
// native id is skipped — it would collide on the dedup key.
func (s softgarden) fetchFeed(ctx context.Context, e CompanyEntry) ([]Job, error) {
	if !sgTenant.MatchString(e.Board) {
		return nil, fmt.Errorf("softgarden: invalid board %q", e.Board)
	}
	var feed sgFeed
	if err := s.http.GetJSON(ctx, fmt.Sprintf("https://%s.career.softgarden.de/jobs.feed.json", e.Board), &feed); err != nil {
		return nil, fmt.Errorf("softgarden: feed %s: %w", e.Board, err)
	}

	jobs := make([]Job, 0, len(feed.Items))
	for _, it := range feed.Items {
		p := it.Item
		id := p.Identifier.Value.String()
		if id == "" {
			continue
		}
		var locations []string
		for _, pl := range p.JobLocation {
			if loc := pl.Address.Location(); loc != "" && !slices.Contains(locations, loc) {
				locations = append(locations, loc)
			}
		}
		location := strings.Join(locations, "; ")
		// As on the listing path: remote is inferred from the location only, never the title.
		remote := isRemote(location)
		jobs = append(jobs, Job{
			ExternalID:  id,
			URL:         p.URL,
			Title:       p.Title,
			Company:     e.Company,
			Location:    location,
			Description: sanitizeHTML(p.Description),
			Remote:      remote,
			WorkMode:    workModeFromRemote(remote),
			PostedAt:    parseRFC3339(p.DatePosted),
		})
	}
	return jobs, nil
}

// sgTenant bounds a board to a DNS label, since it is spliced into a hostname.
var sgTenant = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
