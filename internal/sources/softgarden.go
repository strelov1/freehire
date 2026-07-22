package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"

	"golang.org/x/net/html"
)

// softgarden adapts softgarden career sites. The board is the tenant subdomain (e.g.
// "bundesdruckerei"), so the career site is "<board>.softgarden.io". The root page is a
// server-rendered (Apache Wicket) listing whose anchors link each posting at
// /job/<id>/<slug>?jobDbPVId=<n>; each job page carries a schema.org JobPosting ld+json
// block, so the description comes from a per-job detail fetch (bounded-concurrency), like
// the other schema.org detail adapters.
//
// The listing is a single page: softgarden paginates via a Wicket AJAX callback rather than
// a stable ?page=N URL, so a tenant with more postings than the first page holds is truncated
// to that page. In practice softgarden tenants are small; the seam for AJAX pagination is here
// if a large tenant ever needs it.
type softgarden struct {
	http HTMLGetter
}

// NewSoftgarden builds the softgarden adapter over the given HTTP client.
func NewSoftgarden(c HTMLGetter) Source { return softgarden{http: c} }

func (softgarden) Provider() string { return "softgarden" }

func (s softgarden) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
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
