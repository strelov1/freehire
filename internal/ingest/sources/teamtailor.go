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

// fullBoardListing: jobURLs proves completeness by paginating to a genuinely empty page,
// and now treats a later-page failure or reaching the ttMaxPages safety ceiling as a hard
// Fetch failure rather than a partial success. See the fullBoardListing interface for the
// bar, and ttMaxPages's own comment for the real-board truncation this closes.
func (teamtailor) fullBoardListing() {}

// ttMaxPages bounds listing pagination so a board that never returns an empty page cannot
// loop forever. It is a safety ceiling, not a real-world board size: found by code review
// (openspec/changes/teamtailor-listing-cap-fix) that the previous value, 100, was actively
// truncating real boards — a live probe of tantor.teamtailor.com found pages 100 and 101
// still full (21 links each) and the board's true end only past page 120, meaning the crawl
// was silently dropping roughly a fifth of that board's live postings, not a hypothetical.
// 1000 is a wide multiple over that measurement, and reaching it is now a hard failure
// (jobURLs), never a silent partial success — see fullBoardListing's own bar (source.go)
// for why a reachable ceiling must fail loudly rather than truncate quietly.
const (
	ttMaxPages = 1000
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
//
// Every failure to prove the walk reached the board's real end is a hard error, never a
// partial result — a page fetch failing past page 1, and running out the whole ttMaxPages
// ceiling without ever seeing an empty page, both fail the crawl. This is the
// fullBoardListing bar (source.go): the 2026 measurement that found ttMaxPages=100 quietly
// truncating a real board is exactly the shape of bug that bar exists to catch structurally
// rather than trust to a comment.
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
			// A later page failing is no longer treated as "the board must have ended
			// here" — that assumption is exactly what let ttMaxPages's truncation go
			// unnoticed. An unproven end fails the whole crawl instead.
			return nil, fmt.Errorf("teamtailor: listing %s page %d: %w", e.Board, page, err)
		}
		// Stop on the first page that carries no links at all — the raw count, read before
		// cross-page dedup, not the count of newly-added ones. A non-empty page whose links
		// are all already-seen duplicates is not itself proof the board has no more pages
		// beyond it (a sort tie spanning a page boundary, a re-served page): stopping on
		// "nothing new" there would leave an unseen posting past it unreached. Two currently
		// crawled boards (migen, tantor — see ttMaxPages' own comment) were checked live and
		// both answer a genuinely empty page past their real end, not a repeat, so this is
		// not a regression against an observed pattern; it closes the same class of gap
		// found and fixed for the hand-rolled batch (openspec/changes/fullboardlisting-hand-rolled-batch).
		links := ttJobLinks(base, root)
		for _, link := range links {
			if !seen[link] {
				seen[link] = true
				urls = append(urls, link)
			}
		}
		if len(links) == 0 {
			return urls, nil
		}
	}
	// The loop ran out ttMaxPages without ever seeing an empty page — the board is not
	// proven to have ended, so this is not "here is what we found," it is a failure.
	return nil, fmt.Errorf("teamtailor: listing %s: reached the %d-page safety ceiling without finding the board's end",
		e.Board, ttMaxPages)
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
