package sources

import (
	"context"
	"errors"
	"fmt"
	"net/http"
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
	urls, err := t.jobURLs(ctx, e)
	if err != nil {
		return nil, err
	}

	// Each job's posting comes from its own page fetch, fanned out under a bounded pool.
	return fetchDetails(urls, defaultDetailWorkers, func(u string) (Job, bool) {
		return t.detail(ctx, e, u)
	}), nil
}

// FetchNew is the hydrating crawl: it enumerates the whole board, but fetches a posting's detail
// page only for an id the catalogue does not already have. A seen posting is emitted as a
// liveness refresh (identity only, no detail request, no content rewrite); an unseen one is
// hydrated as before.
//
// This is the difference between a run that costs a request per POSTING and one that costs a
// request per NEW posting, and on this platform the two are worlds apart: measured on prod
// 2026-08-16, the board file holds ~40k live postings and about one an hour is genuinely new, so
// the old crawl spent ~36.7k detail fetches to discover ~100. That volume is what Teamtailor's
// edge turned away — nearly half the fleet 403'd — and what pacing could only spread out.
func (t teamtailor) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	urls, err := t.jobURLs(ctx, e)
	if err != nil {
		return nil, err
	}

	return fetchDetails(urls, defaultDetailWorkers, func(u string) (Job, bool) {
		id := ttJobID(u)
		if id == "" {
			return Job{}, false // no native id → would collide on the dedup key; skip it
		}
		// Already ingested: refresh liveness by identity only. Re-upserting it content-less
		// would wipe the description and the facets derived from it, so the pipeline routes a
		// SeenRefresh to a liveness touch instead of a write.
		if seen(id) {
			// Identity only: the pipeline resolves the row by (provider, board-namespaced id),
			// and it judges an empty-titled refresh on the STORED evidence rather than on this
			// content-less listing — so no title is the honest thing to send, not a defect.
			return Job{ExternalID: id, URL: u, Company: e.Company, SeenRefresh: true}, true
		}
		return t.detail(ctx, e, u)
	}), nil
}

// jobURLs enumerates every posting URL on a board — the listing walk shared by Fetch and
// FetchNew, which differ only in what they do with the result.
func (t teamtailor) jobURLs(ctx context.Context, e CompanyEntry) ([]string, error) {
	// base carries the scheme+host; relative job hrefs resolve against it (an absolute
	// href resolves to itself), so it is parsed once rather than per listing page.
	base, err := url.Parse(fmt.Sprintf("https://%s/", e.Board))
	if err != nil {
		return nil, fmt.Errorf("teamtailor: board %q: %w", e.Board, err)
	}

	// Most sites list postings under /jobs; a few (e.g. jobs.proxify.io) disable that path
	// and render the listing on the site root instead. Probe /jobs first and, when page 1
	// 404s, fall back to the root for this board — a standard site answers /jobs with 200 and
	// never enters the fallback, so their enumeration is unchanged.
	listPath := "jobs"
	var urls []string
	seen := make(map[string]bool)
	for page := 1; page <= ttMaxPages; page++ {
		listURL := fmt.Sprintf("https://%s/%s?page=%d", e.Board, listPath, page)
		root, err := t.http.GetHTML(ctx, listURL)
		if err != nil {
			var se *StatusError
			if page == 1 && listPath == "jobs" && errors.As(err, &se) && se.Code == http.StatusNotFound {
				listPath = ""
				listURL = fmt.Sprintf("https://%s/?page=%d", e.Board, page)
				root, err = t.http.GetHTML(ctx, listURL)
			}
		}
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
	return urls, nil
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
	return firstSubmatch(ttJobIDPattern, u)
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
