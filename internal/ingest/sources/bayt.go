package sources

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// bayt adapts Bayt.com, the dominant Gulf job board. It is a board-based multi-company
// aggregator: each configured entry is a COUNTRY scope (e.Board is the Bayt country slug,
// e.g. "saudi-arabia"), and one crawl walks that country's paginated listings, fetches each
// job-detail page, and reads its self-contained schema.org JobPosting — so the employer comes
// from the posting (hiringOrganization), not the configured entry. Bayt's Akamai/Cloudflare edge
// 403s Go's default TLS+HTTP/2 fingerprint, so in production the adapter is wired with the shared
// Chrome-fingerprint transport (fingerprintHTTP) rather than the shared client. Keyless.
//
// That fingerprint stopped passing: Bayt's Cloudflare-managed challenge 403s every plain request,
// direct or proxied, so production crawls it through the Scrapling tier's browser (scraplingtier.go).
// A browser page is slow (seconds), so the whole listing is walked (the sweep relies on it) but only
// NEW postings are hydrated, up to maxNewPerRun per country per run.

// baytHTTP is the transport bayt needs: HTML listing and detail pages.
type baytHTTP interface{ HTMLGetter }

type bayt struct {
	http baytHTTP
	// maxNewPerRun bounds how many new postings one run hydrates per country; non-positive takes
	// baytMaxNewPerRun. The rest are deferred — not yielded, so not stored — to a later run.
	maxNewPerRun int
}

// NewBayt builds the Bayt adapter over the given HTTP client (the shared Chrome-fingerprint
// fingerprintHTTP in production).
func NewBayt(c baytHTTP) Source { return bayt{http: c, maxNewPerRun: baytMaxNewPerRun} }

func (bayt) Provider() string { return "bayt" }

// aggregator documents that one bayt crawl aggregates postings from many companies (the employer
// comes from each posting, not the configured entry). bayt is board-based (board = country), so it
// already appears in the source facet without this marker; the marker records the multi-company
// nature and future-proofs facet inclusion should bayt ever become boardless.
func (bayt) aggregator() {}

// fullBoardListing: the listing walk proves completeness by paginating sequentially to the
// pager's last page (or a genuinely empty page, where no pager is rendered), and treats a page
// failure or reaching baytMaxPages as a hard failure. detail additionally never drops a
// posting on a merely-unreadable fetch (see detail's own comment) — a plain drop there
// would have defeated this marker's promise given bayt's documented throttling risk. See
// the fullBoardListing interface (source.go) for the bar.
func (bayt) fullBoardListing() {}

const (
	baytBaseURL = "https://www.bayt.com"
	// baytMaxPages caps the per-country pagination so a listing that never runs dry (or a
	// markup change that keeps yielding "new" links) cannot loop unboundedly.
	baytMaxPages = 2000
	// baytMaxNewPerRun is the default hydration budget per country per run, (browser pages are slow).
	baytMaxNewPerRun = 300
	// baytDetailWorkers bounds the detail fan-out well below the shared defaultDetailWorkers (8):
	// Bayt's Akamai edge throttles a fast burst to 403 (observed live), and the fingerprint
	// transport does not retry, so a wide fan-out would silently drop postings. A modest pool
	// trades a slightly longer crawl for far fewer throttled drops.
	baytDetailWorkers = 3
)

// baytJobIDPattern captures the numeric id at the end of a Bayt job-detail path
// (/en/<country>/jobs/<slug>-<id>/). It requires the /jobs/ segment so a /companies/<slug>-<id>/
// link is not mistaken for a posting, and anchors the id to the end so a mid-slug digit run
// never matches.
var baytJobIDPattern = regexp.MustCompile(`/jobs/[^/]+-(\d+)/?$`)

// baytJobID extracts the native Bayt posting id from a job-detail URL, "" when the URL is not a
// job-detail page or carries no trailing id. Any query string or fragment is stripped first so a
// listing href with a tracking suffix (?utm=…) still matches.
func baytJobID(loc string) string {
	return firstSubmatch(baytJobIDPattern, trimURLSuffix(loc))
}

// baytLDPosting is the slice of the page's schema.org JobPosting the adapter reads. Unlike Meta,
// Bayt renders jobLocation as a single Place object with a reliable address (ISO addressCountry),
// which the geography dictionary later resolves into country/region facets.
type baytLDPosting struct {
	Title       string    `json:"title"`
	Description string    `json:"description"`
	DatePosted  string    `json:"datePosted"`
	HiringOrg   baytOrg   `json:"hiringOrganization"`
	JobLocation baytPlace `json:"jobLocation"`
}

type baytOrg struct {
	Name string `json:"name"`
}

type baytPlace struct {
	Address baytAddress `json:"address"`
}

type baytAddress struct {
	AddressLocality string `json:"addressLocality"`
	AddressCountry  string `json:"addressCountry"`
}

// Fetch hydrates every listed posting: the path for callers that supply no seen set. FetchNew
// is the one ingest uses.
func (b bayt) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	links, err := b.listing(ctx, e)
	if err != nil {
		return nil, err
	}
	return fetchDetails(links, baytDetailWorkers, func(link string) (Job, bool) {
		return b.detail(ctx, e, link)
	}), nil
}

// FetchNew walks the whole listing and hydrates only what the catalogue lacks: a seen posting
// is a liveness refresh with no request of its own, and new ones are hydrated up to the per-run
// budget. A new posting past the budget is deferred rather than stored half-read.
func (b bayt) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	links, err := b.listing(ctx, e)
	if err != nil {
		return nil, err
	}
	budget := b.maxNewPerRun
	if budget <= 0 {
		budget = baytMaxNewPerRun
	}
	var jobs []Job
	var fresh []string
	deferred := 0
	for _, link := range links {
		id := baytJobID(link)
		switch {
		case seen(id):
			jobs = append(jobs, Job{ExternalID: id, URL: link, Company: e.Company, SeenRefresh: true})
		case len(fresh) < budget:
			fresh = append(fresh, link)
		default:
			deferred++
		}
	}
	if deferred > 0 {
		log.Printf("bayt: %s: hydration budget %d reached; %d new postings deferred to a later run", e.Board, budget, deferred)
	}
	return append(jobs, fetchDetails(fresh, baytDetailWorkers, func(link string) (Job, bool) {
		return b.detail(ctx, e, link)
	})...), nil
}

// listing walks the country's listing pages sequentially (one request at a time: the edge's burst
// throttling is concurrency-triggered) and returns every posting link, deduped.
//
// The end is the PAGER: a page that links no page beyond itself is the last one. Bayt never answers
// an empty page past its end (it redirects to the last page), so waiting for one runs into the cap.
// A genuinely empty page still ends the walk, for a page rendered without a pager. A page of only
// already-seen duplicates does not. Every page failing, or reaching baytMaxPages, is a hard failure,
// not a partial success; see the fullBoardListing interface (source.go) for the bar.
func (b bayt) listing(ctx context.Context, e CompanyEntry) ([]string, error) {
	seen := make(map[string]struct{})
	var links []string
	for page := 1; page <= baytMaxPages; page++ {
		url := fmt.Sprintf("%s/en/%s/jobs/?page=%d", baytBaseURL, e.Board, page)
		root, err := b.http.GetHTML(ctx, url)
		if err != nil {
			return nil, fmt.Errorf("bayt: listing %s page %d: %w", e.Board, page, err)
		}
		// jobLinks is every anchor on the page that IS a job-detail link (baytJobID matches),
		// before cross-page dedup — the raw count a genuinely empty page proof needs.
		// baytListingLinks itself returns every anchor including navigation chrome, which
		// is never empty even on the listing's last page, so it cannot serve as the proof.
		hrefs := baytListingLinks(root)
		var jobLinks []string
		for _, href := range hrefs {
			if baytJobID(href) != "" {
				jobLinks = append(jobLinks, href)
			}
		}
		for _, href := range jobLinks {
			id := baytJobID(href)
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			links = append(links, baytAbsURL(href))
		}
		if len(jobLinks) == 0 {
			return links, nil // a genuinely empty page: the listing is exhausted
		}
		if last, ok := baytPagerLast(hrefs); ok && last <= page {
			return links, nil // the pager links nothing beyond this page: it is the last
		}
	}
	return nil, fmt.Errorf("bayt: listing %s: reached the %d-page safety ceiling without finding the board's end", e.Board, baytMaxPages)
}

// baytPagerPattern captures the page number of a listing pager link (?page=N or &page=N).
var baytPagerPattern = regexp.MustCompile(`[?&]page=(\d+)`)

// baytPagerLast returns the highest page number a listing page's pager links, and whether it
// links any at all.
func baytPagerLast(hrefs []string) (int, bool) {
	last, found := 0, false
	for _, href := range hrefs {
		if baytJobID(href) != "" {
			continue
		}
		if m := baytPagerPattern.FindStringSubmatch(href); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil {
				found = true
				if n > last {
					last = n
				}
			}
		}
	}
	return last, found
}

// baytListingLinks collects every anchor href on a listing page; Fetch filters them to
// job-detail links via baytJobID.
func baytListingLinks(root *html.Node) []string {
	var hrefs []string
	walk(root, func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "a" {
			if href := Attr(n, "href"); href != "" {
				hrefs = append(hrefs, href)
			}
		}
		return true
	})
	return hrefs
}

// baytBase is the parsed Bayt origin, resolved against once per listing href.
var baytBase, _ = url.Parse(baytBaseURL)

// baytAbsURL resolves a listing href against the Bayt origin via ResolveReference: an absolute href
// keeps itself, a protocol-relative "//host/path" keeps its host under the origin's scheme, and a
// root-relative "/path" gets the origin.
func baytAbsURL(href string) string {
	ref, err := url.Parse(href)
	if err != nil {
		return baytBaseURL + href
	}
	return baytBase.ResolveReference(ref).String()
}

// detail fetches one job page and maps its ld+json JobPosting to a Job. A URL with no parseable id
// is a plain drop (ok=false): it could never have been stored. A 404/410 is dropped too: the
// platform's own evidence the posting is gone. Everything else (a fetch failure incl. a throttled
// 403, a 200 with no ld+json JobPosting, or no resolvable employer) comes back as an
// unreadableDetail marker: this crawl proves listing completeness (fullBoardListing), so a plain
// drop would look like a takedown, and a markup change that broke parsing would look like a mass
// removal to the board-scoped close.
// mass removal to the board-scoped close.
func (b bayt) detail(ctx context.Context, e CompanyEntry, link string) (Job, bool) {
	id := baytJobID(link)
	if id == "" {
		return Job{}, false
	}
	root, err := b.http.GetHTML(ctx, link)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(id, link, e.Company), true
		}
		return Job{}, false
	}
	var p baytLDPosting
	if !ldJobPosting(root, &p) {
		return unreadableDetail(id, link, e.Company), true
	}
	company := strings.TrimSpace(p.HiringOrg.Name)
	if company == "" {
		return unreadableDetail(id, link, e.Company), true
	}

	location := joinNonEmpty(
		strings.TrimSpace(p.JobLocation.Address.AddressLocality),
		strings.TrimSpace(p.JobLocation.Address.AddressCountry),
	)
	return Job{
		ExternalID:  id,
		URL:         link,
		Title:       strings.TrimSpace(p.Title),
		Company:     company,
		Location:    location,
		Description: sanitizeHTML(p.Description),
		Remote:      isRemote(location),
		PostedAt:    parseDate(p.DatePosted),
	}, true
}
