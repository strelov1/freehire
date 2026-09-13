package sources

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"
)

// herp adapts HERP career pages (herp.careers). The board is the company's HERP slug (e.g.
// "a244" for herp.careers/v1/a244). The listing page is server-rendered HTML linking either
// directly to job postings (/v1/<board>/<jobID>) or to "requisition groups"
// (/v1/<board>/requisition-groups/<uuid>) — themselves one more listing page of the same
// direct-job-link shape, one level deep. Each job page carries a standard schema.org
// JobPosting ld+json block, read via the same shared helper breezy/teamtailor use.
type herpHTTP interface {
	HTMLGetter
}

type herp struct {
	http herpHTTP
}

// NewHerp builds the HERP adapter over the given HTTP client.
func NewHerp(c herpHTTP) Source { return herp{http: c} }

func (herp) Provider() string { return "herp" }

// fullBoardListing: a failure fetching the company page or any requisition-group page fails
// the whole Fetch — never a silently truncated listing. See the fullBoardListing interface
// for the bar.
func (herp) fullBoardListing() {}

func (h herp) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	base, err := url.Parse(fmt.Sprintf("https://herp.careers/v1/%s", e.Board))
	if err != nil {
		return nil, fmt.Errorf("herp: bad board %s: %w", e.Board, err)
	}

	root, err := h.http.GetHTML(ctx, base.String())
	if err != nil {
		return nil, fmt.Errorf("herp: list board %s: %w", e.Board, err)
	}

	links := jobLinks(base, root, func(href string) bool { return isHerpJobLink(e.Board, href) })
	groups := jobLinks(base, root, func(href string) bool { return isHerpGroupLink(e.Board, href) })
	seen := make(map[string]bool, len(links))
	for _, l := range links {
		seen[l] = true
	}
	for _, g := range groups {
		groot, err := h.http.GetHTML(ctx, g)
		if err != nil {
			return nil, fmt.Errorf("herp: list requisition group %s: %w", g, err)
		}
		for _, l := range jobLinks(base, groot, func(href string) bool { return isHerpJobLink(e.Board, href) }) {
			if !seen[l] {
				seen[l] = true
				links = append(links, l)
			}
		}
	}

	return fetchDetails(links, defaultDetailWorkers, func(link string) (Job, bool) {
		return h.detail(ctx, e, link)
	}), nil
}

// herpJobID returns the job link's final path segment, HERP's own opaque per-posting id.
func herpJobID(link string) string {
	return path.Base(link)
}

// detail fetches one job page for its JobPosting ld+json block, mapping it to a Job. A page
// the platform reports gone (404/410) is dropped; any other read failure — including a 200
// carrying no JobPosting block — comes back as an unreadableDetail marker, since the page is
// the posting's only source and a dropped posting would be indistinguishable from one taken
// down.
func (h herp) detail(ctx context.Context, e CompanyEntry, link string) (Job, bool) {
	id := herpJobID(link)
	root, err := h.http.GetHTML(ctx, link)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(id, link, e.Company), true
		}
		return Job{}, false
	}

	var p struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		DatePosted  string `json:"datePosted"`
		JobLocation struct {
			Address string `json:"address"`
		} `json:"jobLocation"`
	}
	if !ldJobPosting(root, &p) || p.Title == "" {
		return unreadableDetail(id, link, e.Company), true
	}

	return Job{
		ExternalID:  id,
		URL:         link,
		Title:       p.Title,
		Company:     e.Company,
		Location:    p.JobLocation.Address,
		Description: sanitizeHTML(p.Description),
		PostedAt:    parseRFC3339(p.DatePosted),
	}, true
}

// herpPath returns the path segment(s) after "/v1/<board>/" when href (relative or absolute)
// is a herp.careers URL rooted at exactly that board, or "" otherwise. A share-widget link
// (e.g. Twitter's) has its OWN host, so it never matches here even when its query string
// embeds a real herp.careers URL — href is resolved to a URL and matched by host and path,
// never by substring.
func herpPath(board, href string) string {
	u, err := url.Parse(href)
	if err != nil {
		return ""
	}
	if u.Host != "" && u.Host != "herp.careers" {
		return ""
	}
	prefix := "/v1/" + board + "/"
	if !strings.HasPrefix(u.Path, prefix) {
		return ""
	}
	return strings.TrimPrefix(u.Path, prefix)
}

// herpNonJobSegments are single-segment paths under "/v1/<board>/" that are the platform's
// own machinery, never a job id — "top" is the board's optional distinct landing page, linked
// from the career-page-header__link on every listing and job page of a board that has one
// (confirmed live, e.g. herp.careers/v1/clueitinc/top). Left unexcluded it is indistinguishable
// in shape from a real opaque HERP job id, and — because a listing page carries no JobPosting
// block — its detail fetch would be marked Unreadable on every single crawl, permanently
// withholding that board's stale-job close (see internal/ingest/pipeline's Unreadable-ratio
// gate).
var herpNonJobSegments = map[string]bool{"top": true}

// isHerpJobLink reports whether href is a direct job posting link for board: exactly one path
// segment after "/v1/<board>/", excluding its own "/apply" sublink and known platform words.
func isHerpJobLink(board, href string) bool {
	p := herpPath(board, href)
	if p == "" || strings.HasPrefix(p, "requisition-groups/") || herpNonJobSegments[p] {
		return false
	}
	return !strings.Contains(p, "/")
}

// isHerpGroupLink reports whether href is a requisition-group link for board.
func isHerpGroupLink(board, href string) bool {
	return strings.HasPrefix(herpPath(board, href), "requisition-groups/")
}
