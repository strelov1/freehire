package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// freshteam adapts Freshteam (Freshworks ATS) career sites. The board is the tenant
// subdomain (e.g. "simera-talent"), so the career site is "<board>.freshteam.com". The
// /jobs listing HTML enumerates the postings (paginated via ?page=N); each job page is
// server-rendered HTML carrying a schema.org JobPosting ld+json block, so the description
// comes from a per-job detail fetch (bounded-concurrency), like the other detail adapters.
type freshteam struct {
	http HTMLGetter
}

// NewFreshteam builds the Freshteam adapter over the given HTTP client.
func NewFreshteam(c HTMLGetter) Source { return freshteam{http: c} }

func (freshteam) Provider() string { return "freshteam" }

// ftMaxPages bounds listing pagination so a board that never returns an empty page cannot
// loop forever.
const ftMaxPages = 100

func (f freshteam) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	// base carries the scheme+host; relative job hrefs resolve against it (an absolute href
	// resolves to itself), so it is parsed once rather than per listing page.
	base, err := url.Parse(fmt.Sprintf("https://%s.freshteam.com/", e.Board))
	if err != nil {
		return nil, fmt.Errorf("freshteam: board %q: %w", e.Board, err)
	}

	var urls []string
	seen := make(map[string]bool)
	for page := 1; page <= ftMaxPages; page++ {
		listURL := fmt.Sprintf("https://%s.freshteam.com/jobs?page=%d", e.Board, page)
		root, err := f.http.GetHTML(ctx, listURL)
		if err != nil {
			if page == 1 {
				return nil, fmt.Errorf("freshteam: listing %s: %w", e.Board, err)
			}
			break // a later page failing ends enumeration with the jobs gathered so far
		}
		// Stop on the first page that adds no new links: an empty page, or a board that
		// serves the same page for any ?page=N (de-dup turns the repeat into zero new).
		newLinks := 0
		for _, link := range ftJobLinks(base, root) {
			if !seen[link] {
				seen[link] = true
				urls = append(urls, link)
				newLinks++
			}
		}
		if newLinks == 0 {
			break
		}
	}

	// Each job's posting comes from its own page fetch, fanned out under a bounded pool.
	return fetchDetails(urls, defaultDetailWorkers, func(u string) (Job, bool) {
		return f.detail(ctx, e, u)
	}), nil
}

// detail fetches one job page and maps its JobPosting ld+json to a Job, returning ok=false
// when the page fetch fails, carries no JobPosting, or has no parseable id, so the caller
// skips just that posting.
func (f freshteam) detail(ctx context.Context, e CompanyEntry, jobURL string) (Job, bool) {
	id := ftJobID(jobURL)
	if id == "" {
		return Job{}, false // no native id → would collide on the dedup key; skip it
	}
	root, err := f.http.GetHTML(ctx, jobURL)
	if err != nil {
		return Job{}, false
	}
	var p ftPosting
	if !ldJobPosting(root, &p) {
		return Job{}, false
	}

	location := joinNonEmpty(
		p.JobLocation.Address.AddressLocality,
		p.JobLocation.Address.AddressRegion,
		p.JobLocation.Address.AddressCountry,
	)

	// Freshteam carries an explicit remote flag as a string ("true"/"false"); isRemote
	// (location) is only a fallback (never the title, which false-positives on "Remote …"
	// role names).
	remote := strings.EqualFold(strings.TrimSpace(p.Remote), "true") || isRemote(location)

	return Job{
		ExternalID:  id,
		URL:         jobURL,
		Title:       p.Title,
		Company:     e.Company,
		Location:    location,
		Description: sanitizeHTML(html.UnescapeString(p.Description)),
		Remote:      remote,
		WorkMode:    workModeFromRemote(remote),
		PostedAt:    parseSpaceTime(p.DatePosted),
	}, true
}

// ftJobIDPattern captures the native posting id from a job URL's /jobs/<id> segment. The
// live listing slugs the permalink as /jobs/<id>/<slug>, so the id is the leading 12-char
// [A-Za-z0-9_-] segment followed by a path boundary; the same id also appears bare in the
// apply page's ?jobId. Anchoring to the boundary keeps non-job paths (/jobs, /jobs/search)
// from matching.
var ftJobIDPattern = regexp.MustCompile(`/jobs/([A-Za-z0-9_-]{12})(?:[/?#]|$)`)

// ftJobID extracts the native posting id from a job page URL, or "" when the URL is not a
// job permalink.
func ftJobID(u string) string {
	if m := ftJobIDPattern.FindStringSubmatch(u); m != nil {
		return m[1]
	}
	return ""
}

// ftPosting is the schema.org JobPosting decoded from a Freshteam job page's
// application/ld+json block. Unlike Teamtailor, jobLocation is a single object.
type ftPosting struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	DatePosted  string `json:"datePosted"`
	// Freshteam serializes remote as a JSON string ("true"/"false"), not a bool.
	Remote      string `json:"remote"`
	JobLocation struct {
		Address struct {
			AddressLocality string `json:"addressLocality"`
			AddressRegion   string `json:"addressRegion"`
			AddressCountry  string `json:"addressCountry"`
		} `json:"address"`
	} `json:"jobLocation"`
}

// ftJobLinks returns the absolute hrefs of all anchors linking a /jobs/<id> job page,
// resolved against base (the listing URL) so a board that emits relative hrefs still yields
// fetchable URLs, de-duplicated in first-seen order (a card links the same job from its
// title and apply button).
func ftJobLinks(base *url.URL, root *html.Node) []string {
	return jobLinks(base, root, func(href string) bool { return ftJobID(href) != "" })
}
