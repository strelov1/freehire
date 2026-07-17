package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"

	"golang.org/x/net/html"
)

// rapyd adapts the Rapyd careers site (www.rapyd.net), a self-hosted WordPress/WPBakery
// section rather than a third-party ATS: the /company/careers-search/ listing enumerates
// postings as /company/careers/positions/<slug>/ links, and each position page is
// server-rendered HTML (no schema.org JobPosting block). The board is the career-site host;
// the title, location and description come from a per-job detail fetch (bounded
// concurrency), like the other HTML-detail adapters (globalpayments, successfactors).
type rapyd struct {
	http HTMLGetter
}

// NewRapyd builds the Rapyd adapter over the given HTTP client.
func NewRapyd(c HTMLGetter) Source { return rapyd{http: c} }

func (rapyd) Provider() string { return "rapyd" }

func (r rapyd) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	base, err := url.Parse(fmt.Sprintf("https://%s/", e.Board))
	if err != nil {
		return nil, fmt.Errorf("rapyd: board %q: %w", e.Board, err)
	}

	listURL := fmt.Sprintf("https://%s/company/careers-search/", e.Board)
	root, err := r.http.GetHTML(ctx, listURL)
	if err != nil {
		return nil, fmt.Errorf("rapyd: listing %s: %w", e.Board, err)
	}

	// Each position's content comes from its own page fetch, fanned out under a bounded pool.
	return fetchDetails(rapydPositionLinks(base, root), defaultDetailWorkers, func(u string) (Job, bool) {
		return r.detail(ctx, e, u)
	}), nil
}

// detail fetches one position page and maps it to a Job, returning ok=false when the page
// fetch fails, the URL carries no slug, or the page has no title, so the caller skips just
// that posting.
func (r rapyd) detail(ctx context.Context, e CompanyEntry, jobURL string) (Job, bool) {
	id := rapydPositionID(jobURL)
	if id == "" {
		return Job{}, false // no native id → would collide on the dedup key; skip it
	}
	root, err := r.http.GetHTML(ctx, jobURL)
	if err != nil {
		return Job{}, false
	}

	h1 := firstByTag(root, "h1")
	if h1 == nil {
		return Job{}, false // a page without a heading is a layout/error page, not a posting
	}
	title := textContent(h1)
	if title == "" {
		return Job{}, false
	}

	var location string
	if loc := firstByClass(root, "country-term"); loc != nil {
		location = textContent(loc)
	}

	// The job body lives in the single-career-position__main column; scoping to it keeps the
	// surrounding nav/footer out of the description.
	var description string
	if main := firstByClass(root, "single-career-position__main"); main != nil {
		description = sanitizeHTML(innerHTML(main))
	}

	return Job{
		ExternalID:  id,
		URL:         jobURL,
		Title:       title,
		Company:     e.Company,
		Location:    location,
		Description: description,
		// No structured workplace field is exposed, so the location text is the only remote
		// signal (never the title, which false-positives on "Remote …" role names).
		Remote: isRemote(location),
	}, true
}

// rapydPositionLinks returns the absolute hrefs of all anchors linking a
// /company/careers/positions/<slug>/ page, resolved against base (the listing URL) so
// relative hrefs still yield fetchable URLs, de-duplicated in first-seen order (a card
// links the same position from its title and other controls).
func rapydPositionLinks(base *url.URL, root *html.Node) []string {
	var out []string
	seen := make(map[string]bool)
	walk(root, func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "a" {
			href := attr(n, "href")
			if rapydPositionID(href) == "" {
				return true
			}
			ref, err := url.Parse(href)
			if err != nil {
				return true // unparseable href → not a usable position link
			}
			abs := base.ResolveReference(ref).String()
			if !seen[abs] {
				seen[abs] = true
				out = append(out, abs)
			}
		}
		return true
	})
	return out
}

// rapydPositionIDPattern captures the slug from a /company/careers/positions/<slug>/ path.
// The slug is the posting's stable native id (the site exposes no numeric id).
var rapydPositionIDPattern = regexp.MustCompile(`/company/careers/positions/([^/?#]+)`)

// rapydPositionID extracts the slug id (e.g. "sales-manager-bogota-colombia") from a
// position page URL, or "" when the URL is not a position page.
func rapydPositionID(u string) string {
	return firstSubmatch(rapydPositionIDPattern, u)
}
