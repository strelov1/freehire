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
	http teamtailorHTTP
}

// teamtailorHTTP is the client capability teamtailor needs: the listing and job pages as
// parsed HTML, and the board's RSS feed (remoteStatuses) as XML.
type teamtailorHTTP interface {
	HTMLGetter
	XMLGetter
}

// NewTeamtailor builds the Teamtailor adapter over the given HTTP client.
func NewTeamtailor(c teamtailorHTTP) Source { return teamtailor{http: c} }

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

// ttJobListID is the id of the element a standard Teamtailor listing renders its postings in.
const ttJobListID = "jobs_list_container"

func (t teamtailor) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	urls, err := t.jobURLs(ctx, e)
	if err != nil {
		return nil, err
	}

	statuses := t.remoteStatuses(ctx, e, urls)
	// Each job's posting comes from its own page fetch, fanned out under a bounded pool.
	return fetchDetails(urls, defaultDetailWorkers, func(u string) (Job, bool) {
		return t.detail(ctx, e, u, statuses[ttJobID(u)])
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

	// The feed is read only for what will be hydrated: a run where every posting is already
	// known — the steady state — costs no feed request either.
	var unseen []string
	for _, u := range urls {
		if id := ttJobID(u); id != "" && !seen(id) {
			unseen = append(unseen, u)
		}
	}
	statuses := t.remoteStatuses(ctx, e, unseen)

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
		return t.detail(ctx, e, u, statuses[id])
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
	listed := false // page 1 carried the job-list element; see the scope below
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
		//
		// A standard site renders its postings in ul#jobs_list_container and drops that
		// element past the board's end, while its page chrome may link a posting on EVERY
		// page — an FAQ answer pointing at the open-application role. Read whole, such a
		// page is never empty and the walk runs into ttMaxPages on every crawl (74 boards
		// on prod, measured 2026-11-01). So when page 1 carries the list, every page is read
		// through it alone. A custom site without it (www.agidens.com) keeps the whole-page
		// read; deciding per page instead would read a later page that lacks the list whole
		// and reintroduce the chrome link.
		list := firstByID(root, ttJobListID)
		if page == 1 {
			listed = list != nil
		}
		var links []string
		switch {
		case !listed:
			links = ttJobLinks(base, root)
		case list != nil:
			links = ttJobLinks(base, list)
		} // listed, and this page has no list: past the end, zero links

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
// skips just that posting. remoteStatus is the posting's feed <remoteStatus>, "" when the
// feed did not carry it.
func (t teamtailor) detail(ctx context.Context, e CompanyEntry, jobURL, remoteStatus string) (Job, bool) {
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

	// The feed's remoteStatus is the posting's stated arrangement; jobLocationType
	// TELECOMMUTE is how the page itself marks a "fully" remote one, and is all there is
	// when the feed is unavailable. Teamtailor emits no jobLocationType for hybrid or
	// on-site, so without the feed those stay to the dictionaries.
	workMode := ttRemoteStatusMode(remoteStatus)
	if workMode == "" && p.JobLocationType == "TELECOMMUTE" {
		workMode = "remote"
	}

	return Job{
		ExternalID:  id,
		URL:         jobURL,
		Title:       p.Title,
		Company:     e.Company,
		Location:    location,
		Description: sanitizeHTML(html.UnescapeString(p.Description)),
		// The stated arrangement is the authoritative remote signal; isRemote(location) is
		// only a fallback (never the title, which false-positives on "Remote …" role names).
		Remote:   workMode == "remote" || (workMode == "" && isRemote(location)),
		WorkMode: workMode,
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

// ttFeedPageSize is how many items one jobs.rss request asks for. The feed honours
// per_page and offset (verified 2026-10-18: 100 items by default, 5000 on request, offset
// pages contiguous with a single larger page); 500 keeps one response near 4 MB with
// descriptions inline.
const ttFeedPageSize = 500

// ttFeedMaxPages bounds the feed walk. Unlike the listing it proves nothing about the
// board, so running out of pages is not a failure: postings past it fall back to what
// their page states.
const ttFeedMaxPages = 20

// remoteStatuses reads the board's jobs.rss for the <remoteStatus> of each posting in urls,
// keyed by posting id. The job page states the same field only in a <dl> whose label and
// value are in the career site's own language ("Status Homeoffice/Vor Ort" on a German
// site), omitted for on-site, and its ld+json says nothing for hybrid; the feed carries it
// as a closed, unlocalized enum.
//
// It is best-effort: the feed is a facet source, never the listing, so a failed request
// ends the walk with what it has and a missing status leaves the posting to its page and
// the dictionaries. Newest postings come first, so a run hydrating only new ones usually
// stops after a single request.
func (t teamtailor) remoteStatuses(ctx context.Context, e CompanyEntry, urls []string) map[string]string {
	want := make(map[string]bool, len(urls))
	for _, u := range urls {
		if id := ttJobID(u); id != "" {
			want[id] = true
		}
	}
	out := make(map[string]string, len(want))
	for page := 0; page < ttFeedMaxPages && len(want) > 0; page++ {
		feedURL := fmt.Sprintf("https://%s/jobs.rss?offset=%d&per_page=%d", e.Board, page*ttFeedPageSize, ttFeedPageSize)
		var feed ttFeed
		if err := t.http.GetXML(ctx, feedURL, &feed); err != nil {
			return out
		}
		for _, it := range feed.Channel.Items {
			id := ttJobID(it.Link)
			if want[id] {
				out[id] = it.RemoteStatus
				delete(want, id)
			}
		}
		if len(feed.Channel.Items) < ttFeedPageSize {
			break
		}
	}
	return out
}

// ttFeed is the part of a Teamtailor jobs.rss the adapter reads.
type ttFeed struct {
	Channel struct {
		Items []struct {
			Link         string `xml:"link"`
			RemoteStatus string `xml:"remoteStatus"`
		} `xml:"item"`
	} `xml:"channel"`
}

// ttRemoteStatusMode maps the feed's remoteStatus enum to a work mode. Sampled over 25 live
// boards (2026-10-18) the values are none, onsite, hybrid and fully; Teamtailor also
// documents temporary. "none" is the default nobody changed, and "temporary" (remote for
// now) does not state the arrangement the posting hires into, so both stay "".
func ttRemoteStatusMode(s string) string {
	switch s {
	case "fully":
		return "remote"
	case "hybrid":
		return "hybrid"
	case "onsite":
		return "onsite"
	default:
		return ""
	}
}
