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

func (b bayt) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	seen := make(map[string]struct{})
	var links []string
	for page := 1; page <= baytMaxPages; page++ {
		url := fmt.Sprintf("%s/en/%s/jobs/?page=%d", baytBaseURL, e.Board, page)
		root, err := b.http.GetHTML(ctx, url)
		if err != nil {
			if page == 1 {
				return nil, fmt.Errorf("bayt: listing %s: %w", e.Board, err)
			}
			break // a later page failing just ends pagination; page 1's jobs still ingest
		}
		added := 0
		for _, href := range baytListingLinks(root) {
			id := baytJobID(href)
			if id == "" {
				continue
			}
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			links = append(links, baytAbsURL(href))
			added++
		}
		if added == 0 {
			break // no new postings on this page → the listing is exhausted
		}
	}

	return fetchDetails(links, baytDetailWorkers, func(link string) (Job, bool) {
		return b.detail(ctx, link)
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

// detail fetches one job page and maps its ld+json JobPosting to a Job, returning ok=false when
// the page fetch fails, carries no JobPosting, has no resolvable employer (company-less), or has
// no parseable id (which would collide on the dedup key) — so the caller skips just that posting.
func (b bayt) detail(ctx context.Context, link string) (Job, bool) {
	id := baytJobID(link)
	if id == "" {
		return Job{}, false
	}
	root, err := b.http.GetHTML(ctx, link)
	if err != nil {
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
