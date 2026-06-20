package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"

	"golang.org/x/net/html"
)

// teamtailor adapts Teamtailor career sites. The board is the career-site host (e.g.
// "jobs.tibber.com"). The /jobs listing HTML enumerates the postings; each job page is
// server-rendered HTML carrying a schema.org JobPosting ld+json block, so the description
// comes from a per-job detail fetch (bounded-concurrency), like the other detail adapters.
type teamtailor struct {
	http HTMLGetter
}

// NewTeamtailor builds the Teamtailor adapter over the given HTTP client.
func NewTeamtailor(c HTMLGetter) Source { return teamtailor{http: c} }

func (teamtailor) Provider() string { return "teamtailor" }

// ttMaxPages bounds listing pagination so a board that never returns an empty page
// cannot loop forever.
const (
	ttMaxPages = 100
)

func (t teamtailor) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	// base carries the scheme+host; relative job hrefs resolve against it (an absolute
	// href resolves to itself), so it is parsed once rather than per listing page.
	base, err := url.Parse(fmt.Sprintf("https://%s/", e.Board))
	if err != nil {
		return nil, fmt.Errorf("teamtailor: board %q: %w", e.Board, err)
	}

	var urls []string
	seen := make(map[string]bool)
	for page := 1; page <= ttMaxPages; page++ {
		listURL := fmt.Sprintf("https://%s/jobs?page=%d", e.Board, page)
		root, err := t.http.GetHTML(ctx, listURL)
		if err != nil {
			if page == 1 {
				return nil, fmt.Errorf("teamtailor: listing %s: %w", e.Board, err)
			}
			break // a later page failing ends enumeration with the jobs gathered so far
		}
		// Stop on the first page that adds no new links: an empty page, or a board that
		// serves the same page for any ?page=N (de-dup turns the repeat into zero new).
		newLinks := 0
		for _, link := range ttJobLinks(base, root) {
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
		return t.detail(ctx, e, u)
	}), nil
}

// detail fetches one job page and maps its JobPosting ld+json to a Job, returning ok=false
// when the page fetch fails, carries no JobPosting, or has no parseable id, so the caller
// skips just that posting.
func (t teamtailor) detail(ctx context.Context, e CompanyEntry, jobURL string) (Job, bool) {
	id := ttJobID(jobURL)
	if id == "" {
		return Job{}, false // no native id → would collide on the dedup key; skip it
	}
	root, err := t.http.GetHTML(ctx, jobURL)
	if err != nil {
		return Job{}, false
	}
	p, ok := ttJobPosting(root)
	if !ok {
		return Job{}, false
	}

	var city, country string
	if len(p.JobLocation) > 0 {
		city = p.JobLocation[0].Address.AddressLocality
		country = p.JobLocation[0].Address.AddressCountry
	}
	location := joinNonEmpty(city, country)

	return Job{
		ExternalID:  id,
		URL:         jobURL,
		Title:       p.Title,
		Company:     e.Company,
		Location:    location,
		Description: sanitizeHTML(html.UnescapeString(p.Description)),
		// jobLocationType is the authoritative remote signal; isRemote(location) is only a
		// fallback (never the title, which false-positives on "Remote …" role names).
		Remote:   p.JobLocationType == "TELECOMMUTE" || isRemote(location),
		PostedAt: parseRFC3339(p.DatePosted),
	}, true
}

// ttJobIDPattern captures the numeric posting id from a job URL's /jobs/<id> segment.
var ttJobIDPattern = regexp.MustCompile(`/jobs/(\d+)`)

// ttJobID extracts the native numeric posting id from a job page URL.
func ttJobID(u string) string {
	if m := ttJobIDPattern.FindStringSubmatch(u); m != nil {
		return m[1]
	}
	return ""
}

// ttPosting is the schema.org JobPosting decoded from a Teamtailor job page's
// application/ld+json block.
type ttPosting struct {
	Title           string    `json:"title"`
	Description     string    `json:"description"`
	DatePosted      string    `json:"datePosted"`
	JobLocationType string    `json:"jobLocationType"`
	JobLocation     []ttPlace `json:"jobLocation"`
}

// ttPlace is one entry of JobPosting.jobLocation (Teamtailor always emits an array).
type ttPlace struct {
	Address struct {
		AddressLocality string `json:"addressLocality"`
		AddressCountry  string `json:"addressCountry"`
	} `json:"address"`
}

// ttJobLinks returns the absolute hrefs of all anchors linking a /jobs/<id> job page,
// resolved against base (the listing URL) so a board that emits relative hrefs still
// yields fetchable URLs, de-duplicated in first-seen order (a card links the same job from
// its title and apply button). A link is a job exactly when it carries a parseable native
// id, so enumeration keys off the stable public permalink shape rather than CSS classes.
func ttJobLinks(base *url.URL, root *html.Node) []string {
	return jobLinks(base, root, func(href string) bool { return ttJobID(href) != "" })
}

// ttJobPosting decodes the first application/ld+json JobPosting on the page, returning
// ok=false when no such block is present.
func ttJobPosting(root *html.Node) (ttPosting, bool) {
	var p ttPosting
	return p, ldJobPosting(root, &p)
}
