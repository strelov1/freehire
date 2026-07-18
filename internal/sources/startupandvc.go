package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"time"

	"golang.org/x/net/html"
)

// startupandvc adapts startupandvc.com's venture-capital jobs board, a curated Webflow site
// listing VC roles that link out to the original posting (LinkedIn, an ATS, or a company page).
// The single listing at /venture-capital-jobs server-renders every card (≤100, Webflow's CMS
// collection cap; there is no pagination), each linking to a /venture-capital-jobs/<slug> detail
// page whose schema.org JobPosting ld+json carries the structured fields. The rich description
// lives in the page's w-richtext body (the ld+json description is only a one-line stub), and an
// "Apply now" button links to the real posting.
//
// Boardless (one site, no per-tenant board) and an aggregator (each posting's employer is its
// hiringOrganization, and it re-lists jobs owned by first-party ATS boards), so it stays in the
// source facet and inherits the reindex aggregator/ATS-duplicate suppression.
type startupandvc struct {
	http HTMLGetter
}

// NewStartupAndVC builds the startupandvc.com VC-jobs adapter over the given HTML client.
func NewStartupAndVC(c HTMLGetter) Source { return startupandvc{http: c} }

func (startupandvc) Provider() string { return "startupandvc" }

func (startupandvc) boardless() {}

func (startupandvc) aggregator() {}

const (
	// startupandvcBaseURL is the site root the listing's relative /venture-capital-jobs/<slug>
	// links resolve against.
	startupandvcBaseURL = "https://www.startupandvc.com/"
	// startupandvcListURL is the single listing page. Webflow renders every card server-side
	// (capped at 100), so one GET yields the whole board — there is no pagination to walk.
	startupandvcListURL = "https://www.startupandvc.com/venture-capital-jobs"
)

func (s startupandvc) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	base, _ := url.Parse(startupandvcBaseURL) // a constant literal never fails to parse

	root, err := s.http.GetHTML(ctx, startupandvcListURL)
	if err != nil {
		return nil, fmt.Errorf("startupandvc: listing: %w", err)
	}
	locs := jobLinks(base, root, func(href string) bool { return startupandvcSlug(href) != "" })

	// Each posting's fields come from its own detail fetch (the listing card omits the
	// description), fanned out under the shared detail pool.
	return fetchDetails(locs, defaultDetailWorkers, func(loc string) (Job, bool) {
		return s.detail(ctx, e, loc)
	}), nil
}

// detail fetches one vacancy's detail page and maps its JobPosting ld+json (plus the w-richtext
// body and the apply button) to a Job, returning ok=false when the fetch fails, the page carries
// no JobPosting, or the URL yields no slug, so the caller skips just that posting.
func (s startupandvc) detail(ctx context.Context, e CompanyEntry, loc string) (Job, bool) {
	root, err := s.http.GetHTML(ctx, loc)
	if err != nil {
		return Job{}, false
	}
	var p startupandvcPosting
	if !ldJobPosting(root, &p) || p.Title == "" {
		return Job{}, false
	}
	slug := startupandvcSlug(loc)
	if slug == "" {
		return Job{}, false
	}

	location := p.location()
	// URL points at the real posting the "Apply now" button links to; when the page has no
	// external button we fall back to the stable startupandvc landing page.
	jobURL := firstNonEmpty(startupandvcApplyURL(root), loc)

	return Job{
		ExternalID:  slug,
		URL:         jobURL,
		Title:       p.Title,
		Company:     firstNonEmpty(p.HiringOrganization.Name, e.Company),
		Location:    location,
		Description: startupandvcDescription(root, p.Description),
		Remote:      isRemote(location),
		PostedAt:    startupandvcDate(p.DatePosted),
	}, true
}

// startupandvcPosting is the schema.org JobPosting decoded from a detail page's ld+json block.
// jobLocation is a single Place in practice, but decoded through schemaPlaces so an array form
// does not fail the whole posting's unmarshal.
type startupandvcPosting struct {
	Title              string `json:"title"`
	Description        string `json:"description"`
	DatePosted         string `json:"datePosted"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	JobLocation schemaPlaces `json:"jobLocation"`
}

// location builds the free-text location from the first jobLocation place, or "" when the posting
// carries none (a remote-only vacancy often omits it).
func (p startupandvcPosting) location() string {
	if len(p.JobLocation) == 0 {
		return ""
	}
	return p.JobLocation[0].Address.Location()
}

// startupandvcDescription returns the sanitized rich description from the page's w-richtext body,
// which carries the full posting text; it falls back to the ld+json stub only when the page has no
// richtext block (the ld+json description is a one-line "<Company> is looking for a <Role>" stub).
func startupandvcDescription(root *html.Node, ldStub string) string {
	if body := firstByClass(root, "w-richtext"); body != nil {
		if rich := innerHTML(body); rich != "" {
			return sanitizeHTML(rich)
		}
	}
	return sanitizeHTML(html.UnescapeString(ldStub))
}

// startupandvcSlugPattern captures the <slug> from a /venture-capital-jobs/<slug> detail URL. The
// match must end at the slug (end-of-string or a ?/# suffix), so the bare /venture-capital-jobs
// listing path and deeper /venture-capital-jobs/x/y paths never match, and a tracking suffix does
// not defeat it.
var startupandvcSlugPattern = regexp.MustCompile(`/venture-capital-jobs/([a-z0-9][a-z0-9-]*)(?:$|[?#])`)

// startupandvcSlug extracts the native posting slug from a URL, or "" when the URL is not a
// vacancy detail link.
func startupandvcSlug(loc string) string { return firstSubmatch(startupandvcSlugPattern, loc) }

// startupandvcDate parses startupandvc's "Jul 07, 2026" datePosted (Webflow's "%b %d, %Y" format),
// which the shared RFC3339/ISO parsers do not recognize. The lenient day layout ("2") accepts both
// the zero-padded "07" the site emits and a bare "7", should a posting ever drop the pad.
func startupandvcDate(s string) *time.Time { return parseLayout("Jan 2, 2006", s) }

// startupandvcApplyURL returns the outbound "Apply now" link — the real posting on LinkedIn, an
// ATS, or the company site — or "" when the page has no external apply button (the caller then
// falls back to the startupandvc landing page).
//
// The button is the page's only <a class="button-large …">, and its href is the destination with
// startupandvc's tracking params appended (e.g. ".../jobs/view/4417182287/?eBP=…&trk=…?utm_source=
// startupandvc&…"). We drop the whole query/fragment: these postings carry their identity in the
// PATH (LinkedIn /jobs/view/<id>/, /posts/<slug>), so trimming yields the clean, stable canonical
// link and strips both startupandvc's UTM suffix and LinkedIn's session-scoped eBP/trk/refId
// tracking — which go stale between crawl and click anyway. The trade-off: an ATS destination that
// kept its id in the query (?gh_jid=…) would lose it, but the board's outbound links are
// LinkedIn-dominated and path-identified, so a clean link is the better default.
func startupandvcApplyURL(root *html.Node) string {
	href := elementAttr(root, "a", "button-large", "href")
	if href == "" {
		return ""
	}
	return trimURLSuffix(href)
}
