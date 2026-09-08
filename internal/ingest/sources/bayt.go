package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
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

// baytHTTP is the transport bayt needs: HTML listing and detail pages.
type baytHTTP interface{ HTMLGetter }

type bayt struct {
	http baytHTTP
}

// NewBayt builds the Bayt adapter over the given HTTP client (the shared Chrome-fingerprint
// fingerprintHTTP in production).
func NewBayt(c baytHTTP) Source { return bayt{http: c} }

func (bayt) Provider() string { return "bayt" }

// aggregator documents that one bayt crawl aggregates postings from many companies (the employer
// comes from each posting, not the configured entry). bayt is board-based (board = country), so it
// already appears in the source facet without this marker; the marker records the multi-company
// nature and future-proofs facet inclusion should bayt ever become boardless.
func (bayt) aggregator() {}

// fullBoardListing: Fetch proves completeness by paginating sequentially to a genuinely
// empty page (the raw job-link count, before cross-page dedup), and treats a page failure
// or reaching baytMaxPages as a hard Fetch failure. detail additionally never drops a
// posting on a merely-unreadable fetch (see detail's own comment) — a plain drop there
// would have defeated this marker's promise given bayt's documented throttling risk. See
// the fullBoardListing interface (source.go) for the bar.
func (bayt) fullBoardListing() {}

const (
	baytBaseURL = "https://www.bayt.com"
	// baytMaxPages caps the per-country pagination so a listing that never runs dry (or a
	// markup change that keeps yielding "new" links) cannot loop unboundedly.
	baytMaxPages = 50
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

// Fetch walks the country's listing pages sequentially (one request at a time — the burst
// throttling bayt's Akamai edge is documented to apply is a concurrency-triggered risk,
// which the detail fan-out below already paces around with baytDetailWorkers) until a
// genuinely empty page is reached — the raw per-page link count, not the count of links
// newly kept after cross-page dedup, since a non-empty page whose links are all
// already-seen duplicates is not itself proof the listing has no more pages beyond it.
// Every page failing, and reaching baytMaxPages without a genuinely empty page, are hard
// Fetch failures rather than a partial success — see the fullBoardListing interface
// (source.go) for the bar.
func (b bayt) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	seen := make(map[string]struct{})
	var links []string
	done := false
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
		var jobLinks []string
		for _, href := range baytListingLinks(root) {
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
			done = true
			break // a genuinely empty page: the listing is exhausted
		}
	}
	if !done {
		return nil, fmt.Errorf("bayt: listing %s: reached the %d-page safety ceiling without finding the board's end", e.Board, baytMaxPages)
	}

	return fetchDetails(links, baytDetailWorkers, func(link string) (Job, bool) {
		return b.detail(ctx, e, link)
	}), nil
}

// baytListingLinks collects every anchor href on a listing page; Fetch filters them to
// job-detail links via baytJobID.
func baytListingLinks(root *html.Node) []string {
	var hrefs []string
	walk(root, func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "a" {
			if href := attr(n, "href"); href != "" {
				hrefs = append(hrefs, href)
			}
		}
		return true
	})
	return hrefs
}

// baytBase is the parsed Bayt origin, resolved against once per listing href.
var baytBase, _ = url.Parse(baytBaseURL)

// baytAbsURL resolves a listing href against the Bayt origin, handling all three forms via
// ResolveReference: an already-absolute href keeps itself, a protocol-relative "//host/path"
// keeps its own host under the origin's scheme, and a root-relative "/path" gets the origin. The
// old strings.HasPrefix(href, "http") guess mis-resolved a protocol-relative href into
// "https://www.bayt.com//host/path", silently dropping those postings at the detail fetch.
func baytAbsURL(href string) string {
	ref, err := url.Parse(href)
	if err != nil {
		return baytBaseURL + href
	}
	return baytBase.ResolveReference(ref).String()
}

// detail fetches one job page and maps its ld+json JobPosting to a Job. A URL carrying no
// parseable id is a plain drop (ok=false) — it could never have been stored, so no close can
// reach it. A page the platform answers 404/410 for is dropped too: the platform's own
// evidence the posting is gone. Everything else the fetch could fail with — including a
// throttled 403, the documented risk of a fast burst against Bayt's Akamai edge — comes back
// as an unreadableDetail marker instead: this crawl now proves listing completeness
// (fullBoardListing) and re-fetches every posting's detail on every run (no HydratingSource),
// so a plain drop on a merely-unreadable page would be indistinguishable from the posting
// having been taken down. A page with no usable JobPosting (no ld+json, or no resolvable
// employer) is a content-shape drop, not a fetch failure, and stays ok=false.
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
		return Job{}, false
	}
	company := strings.TrimSpace(p.HiringOrg.Name)
	if company == "" {
		return Job{}, false
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
