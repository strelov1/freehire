package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// betterteam adapts Betterteam career sites. The board is the tenant subdomain (e.g.
// "110grill"), so the career site is "<board>.betterteam.com". The root page lists each
// posting at /<slug>-<id> (e.g. /bartender-21); each job page carries a schema.org
// JobPosting ld+json block, so the description comes from a per-job detail fetch
// (bounded-concurrency), like the other schema.org detail adapters (freshteam/softgarden).
//
// The listing is a single page: Betterteam renders the whole board on the site root, so a
// tenant with more postings than the root holds is truncated. In practice Betterteam
// tenants (small businesses) are small.
type betterteam struct {
	http HTMLGetter
}

// NewBetterteam builds the Betterteam adapter over the given HTTP client.
func NewBetterteam(c HTMLGetter) Source { return betterteam{http: c} }

func (betterteam) Provider() string { return "betterteam" }

func (b betterteam) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	// base carries the scheme+host; the listing's relative "/<slug>-<id>" hrefs resolve
	// against it into fetchable absolute URLs.
	base, err := url.Parse(fmt.Sprintf("https://%s.betterteam.com/", e.Board))
	if err != nil {
		return nil, fmt.Errorf("betterteam: board %q: %w", e.Board, err)
	}

	root, err := b.http.GetHTML(ctx, base.String())
	if err != nil {
		return nil, fmt.Errorf("betterteam: listing %s: %w", e.Board, err)
	}
	urls := jobLinks(base, root, func(href string) bool { return btJobID(href) != "" })

	// Each job's posting comes from its own page fetch, fanned out under a bounded pool.
	return fetchDetails(urls, defaultDetailWorkers, func(u string) (Job, bool) {
		return b.detail(ctx, e, u)
	}), nil
}

// detail fetches one job page and maps its JobPosting ld+json to a Job, returning ok=false
// when the page fetch fails, carries no JobPosting, or has no parseable id, so the caller
// skips just that posting.
func (b betterteam) detail(ctx context.Context, e CompanyEntry, jobURL string) (Job, bool) {
	id := btJobID(jobURL)
	if id == "" {
		return Job{}, false // no native id → would collide on the dedup key; skip it
	}
	root, err := b.http.GetHTML(ctx, jobURL)
	if err != nil {
		return Job{}, false
	}
	var p btPosting
	if !ldJobPosting(root, &p) {
		return Job{}, false
	}

	location := p.JobLocation.Address.Location()
	// Betterteam's JobPosting carries no explicit remote flag, so remote is inferred from the
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

// btJobIDPattern captures the native posting id from a job URL's path. A posting permalink
// is a single "/<slug>-<id>" segment ending in the numeric id (e.g. /bartender-21); the id
// stored is the whole slug-id segment, which Betterteam echoes as the ld+json identifier.
var btJobIDPattern = regexp.MustCompile(`^/([a-z0-9][a-z0-9-]*-\d+)$`)

// btJobID extracts the native posting id (the slug-id path segment) from a job URL, or ""
// when the URL is not a job permalink. Only the path is matched, so absolute and relative
// hrefs on the tenant host both resolve.
func btJobID(u string) string {
	p := u
	if parsed, err := url.Parse(u); err == nil {
		p = parsed.Path
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return firstSubmatch(btJobIDPattern, p)
}

// btPosting is the schema.org JobPosting decoded from a Betterteam job page's
// application/ld+json block. jobLocation is a single Place object.
type btPosting struct {
	Title       string      `json:"title"`
	Description string      `json:"description"`
	DatePosted  string      `json:"datePosted"`
	JobLocation schemaPlace `json:"jobLocation"`
}
