package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"

	"golang.org/x/net/html"
)

// jazzhr adapts JazzHR career sites. The board is the JazzHR slug (e.g. "proautomated"),
// forming the host "{board}.applytojob.com". The /apply listing HTML enumerates the
// postings; each job page is server-rendered HTML carrying a schema.org JobPosting ld+json
// block, so the description comes from a per-job detail fetch (bounded-concurrency), like
// the other HTML detail adapters (teamtailor, radancy).
type jazzhr struct {
	http HTMLGetter
}

// NewJazzHR builds the JazzHR adapter over the given HTTP client.
func NewJazzHR(c HTMLGetter) Source { return jazzhr{http: c} }

func (jazzhr) Provider() string { return "jazzhr" }

func (s jazzhr) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	host := fmt.Sprintf("%s.applytojob.com", e.Board)
	base, err := url.Parse(fmt.Sprintf("https://%s/", host))
	if err != nil {
		return nil, fmt.Errorf("jazzhr: board %q: %w", e.Board, err)
	}

	// The /apply listing is a single page that links every open posting (no pagination).
	root, err := s.http.GetHTML(ctx, fmt.Sprintf("https://%s/apply", host))
	if err != nil {
		return nil, fmt.Errorf("jazzhr: listing %s: %w", e.Board, err)
	}

	// Each job's posting comes from its own page fetch, fanned out under a bounded pool.
	return fetchDetails(jazzhrJobLinks(base, root), defaultDetailWorkers, func(u string) (Job, bool) {
		return s.detail(ctx, e, u)
	}), nil
}

// detail fetches one job page and maps its JobPosting ld+json to a Job, returning ok=false
// when the page fetch fails, carries no JobPosting, or has no parseable id, so the caller
// skips just that posting.
func (s jazzhr) detail(ctx context.Context, e CompanyEntry, jobURL string) (Job, bool) {
	id := jazzhrJobID(jobURL)
	if id == "" {
		return Job{}, false // no native id → would collide on the dedup key; skip it
	}
	root, err := s.http.GetHTML(ctx, jobURL)
	if err != nil {
		return Job{}, false
	}
	var p jazzhrPosting
	if !ldJobPosting(root, &p) {
		return Job{}, false
	}

	location := p.JobLocation.Address.Location()

	return Job{
		ExternalID:  id,
		URL:         jobURL,
		Title:       p.Title,
		Company:     firstNonEmpty(p.HiringOrganization.Name, e.Company),
		Location:    location,
		Description: sanitizeHTML(html.UnescapeString(p.Description)),
		// JazzHR exposes no structured remote flag, so isRemote(location) is the only
		// signal (never the title, which false-positives on "Remote …" role names).
		Remote:   isRemote(location),
		PostedAt: parseDate(p.DatePosted),
	}, true
}

// jazzhrPosting is the schema.org JobPosting decoded from a JazzHR job page's
// application/ld+json block. jobLocation is a single Place (not an array, unlike iCIMS).
type jazzhrPosting struct {
	Title              string      `json:"title"`
	Description        string      `json:"description"`
	DatePosted         string      `json:"datePosted"`
	JobLocation        schemaPlace `json:"jobLocation"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
}

// jazzhrJobIDPattern captures the JazzHR posting token from an /apply/<token>/ URL (the
// stable public permalink id, e.g. ".../apply/bhCE7nHkv6/Some-Role" → "bhCE7nHkv6").
var jazzhrJobIDPattern = regexp.MustCompile(`/apply/([A-Za-z0-9]+)/`)

// jazzhrJobID extracts the native posting token from a job page URL.
func jazzhrJobID(u string) string {
	return firstSubmatch(jazzhrJobIDPattern, u)
}

// jazzhrJobLinks returns the absolute hrefs of all anchors linking an /apply/<token>/ job
// page, resolved against base, de-duplicated in first-seen order. A link is a job exactly
// when it carries a parseable token, so enumeration keys off the public permalink shape
// rather than CSS classes.
func jazzhrJobLinks(base *url.URL, root *html.Node) []string {
	return jobLinks(base, root, func(href string) bool { return jazzhrJobID(href) != "" })
}
