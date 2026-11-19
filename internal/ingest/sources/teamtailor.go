package sources

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"

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

// fullBoardListing: jobURLs paginates to a genuinely empty page; a later-page failure or hitting
// ttMaxPages fails the crawl instead of returning a partial list.
func (teamtailor) FullBoardListing() {}

// ttMaxPages bounds listing pagination so a board that never returns an empty page cannot loop
// forever. Real boards run past 100 pages, so it is a safety ceiling; reaching it fails the crawl
// (jobURLs).
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
// page only for an id the catalogue does not already have. A seen posting is emitted as a liveness
// refresh (identity only, no detail request, no content rewrite). This costs a request per new
// posting instead of per posting, which keeps the fleet under Teamtailor's edge limits (it answers
// heavy crawls with 403).
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

// jobURLs enumerates every posting URL on a board, shared by Fetch and FetchNew. Failing to prove
// the walk reached the board's real end (a page fails past page 1, or ttMaxPages runs out) is a
// hard error, never a partial result: the fullBoardListing bar (source.go).
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
	pageVisits := make(map[string]int)
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
				if err != nil && errors.As(err, &se) && se.Code == http.StatusNotFound {
					return nil, fmt.Errorf("teamtailor: career site %q: %w", e.Board, ErrBoardGone)
				}
			}
		}
		if err != nil {
			// A later page failing is an unproven end: fail the whole crawl.
			return nil, fmt.Errorf("teamtailor: listing %s page %d: %w", e.Board, page, err)
		}
		// Stop on the first page with no links (raw count, before cross-page dedup): a page of only
		// already-seen links does not prove the end (sort ties, re-served pages).
		//
		// A standard site renders postings in ul#jobs_list_container and drops it past the board's
		// end, while its page chrome may link a posting on EVERY page (an FAQ pointing at an
		// open-application role). So when page 1 carries the list, every page is read through it
		// alone; a custom site without it keeps the whole-page read. Deciding per page would
		// reintroduce the chrome link.
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

		// Tolerate one overlapping page, but repeated non-empty pages prove no end.
		if len(links) > 0 {
			signatureLinks := append([]string(nil), links...)
			sort.Strings(signatureLinks)
			signature := strings.Join(signatureLinks, "\n")
			pageVisits[signature]++
			if pageVisits[signature] >= 3 {
				return nil, fmt.Errorf("teamtailor: listing %s page %d: repeated non-empty listing page", e.Board, page)
			}
		}
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
	// ttMaxPages ran out without an empty page: the end is unproven, so fail.
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

// ttFeedPageSize is how many items one jobs.rss request asks for. The feed honours per_page and
// offset; 500 keeps one response near 4 MB with descriptions inline.
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

// Values seen are none, onsite, hybrid and fully (Teamtailor also documents temporary). "none" is
// the default nobody changed and "temporary" does not state the hiring arrangement, so both stay
// "".
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
