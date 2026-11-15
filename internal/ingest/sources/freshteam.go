package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// freshteam adapts Freshteam (Freshworks ATS) career sites. The board is the tenant
// subdomain (e.g. "simera-talent"), so the career site is "<board>.freshteam.com". The
// /jobs listing HTML enumerates the postings (paginated via ?page=N); each job page is
// server-rendered HTML carrying a schema.org JobPosting ld+json block, so the description
// comes from a per-job detail fetch (bounded-concurrency), like the other detail adapters.
type freshteam struct {
	http HTMLGetter
}

// NewFreshteam builds the Freshteam adapter over the given HTTP client.
func NewFreshteam(c HTMLGetter) Source { return freshteam{http: c} }

func (freshteam) Provider() string { return "freshteam" }

// ftMaxPages bounds listing pagination so a board that never returns an empty page cannot
// loop forever.
const ftMaxPages = 100

func (f freshteam) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	// base carries the scheme+host; relative job hrefs resolve against it (an absolute href
	// resolves to itself), so it is parsed once rather than per listing page.
	base, err := url.Parse(fmt.Sprintf("https://%s.freshteam.com/", e.Board))
	if err != nil {
		return nil, fmt.Errorf("freshteam: board %q: %w", e.Board, err)
	}

	// Page through the listing until a page adds no new links (an empty page, or a board that
	// serves the same page for any ?page=N — de-dup turns the repeat into zero new).
	urls, err := crawlPagedLinks(ctx, f.http, ftMaxPages,
		func(page int) string { return fmt.Sprintf("https://%s.freshteam.com/jobs?page=%d", e.Board, page) },
		func(root *html.Node) []string { return ftJobLinks(base, root) })
	if err != nil {
		return nil, fmt.Errorf("freshteam: listing %s: %w", e.Board, err)
	}

	// Each job's posting comes from its own page fetch, fanned out under a bounded pool.
	return fetchDetails(urls, defaultDetailWorkers, func(u string) (Job, bool) {
		return f.detail(ctx, e, u)
	}), nil
}

// detail fetches one job page for its JobPosting ld+json block, mapping it to a Job. A page
// the platform reports gone (404/410) is dropped, as is a link carrying no parseable id; any
// other read failure — including a 200 carrying neither structured nor rendered job data —
// comes back as an unreadableDetail marker, since the page is the posting's only source and a dropped posting
// would be indistinguishable from one taken down.
func (f freshteam) detail(ctx context.Context, e CompanyEntry, jobURL string) (Job, bool) {
	id := ftJobID(jobURL)
	if id == "" {
		return Job{}, false // no native id → would collide on the dedup key; skip it
	}
	root, err := f.http.GetHTML(ctx, jobURL)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(id, jobURL, e.Company), true
		}
		return Job{}, false
	}
	var p ftPosting
	rendered := false
	if !ldJobPosting(root, &p) {
		rendered = true
		var ok bool
		p, ok = ftRenderedPosting(root)
		if !ok {
			return unreadableDetail(id, jobURL, e.Company), true
		}
	}

	description := p.Description
	if !rendered {
		description = html.UnescapeString(description)
	}
	location := p.JobLocation.Address.Location()

	// Freshteam carries an explicit remote flag as a string ("true"/"false"); isRemote
	// (location) is only a fallback (never the title, which false-positives on "Remote …"
	// role names).
	remote := strings.EqualFold(strings.TrimSpace(p.Remote), "true") || isRemote(location)

	return Job{
		ExternalID:  id,
		URL:         jobURL,
		Title:       p.Title,
		Company:     e.Company,
		Location:    location,
		Description: sanitizeHTML(description),
		Remote:      remote,
		WorkMode:    workModeFromRemote(remote),
		PostedAt:    parseSpaceTime(p.DatePosted),
	}, true
}

// ftJobIDPattern captures the native posting id from a job URL's /jobs/<id> segment. The
// live listing slugs the permalink as /jobs/<id>/<slug>, so the id is the leading 12-char
// [A-Za-z0-9_-] segment followed by a path boundary; the same id also appears bare in the
// apply page's ?jobId. Anchoring to the boundary keeps non-job paths (/jobs, /jobs/search)
// from matching.
var ftJobIDPattern = regexp.MustCompile(`/jobs/([A-Za-z0-9_-]{12})(?:[/?#]|$)`)

// ftJobID extracts the native posting id from a job page URL, or "" when the URL is not a
// job permalink.
func ftJobID(u string) string {
	return firstSubmatch(ftJobIDPattern, u)
}

// ftPosting is the schema.org JobPosting decoded from a Freshteam job page's
// application/ld+json block. Unlike Teamtailor, jobLocation is a single object.
type ftPosting struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	DatePosted  string `json:"datePosted"`
	// Freshteam serializes remote as a JSON string ("true"/"false"), not a bool.
	Remote      string      `json:"remote"`
	JobLocation schemaPlace `json:"jobLocation"`
}

// ftJobLinks returns the absolute hrefs of all anchors linking a /jobs/<id> job page,
// resolved against base (the listing URL) so a board that emits relative hrefs still yields
// fetchable URLs, de-duplicated in first-seen order (a card links the same job from its
// title and apply button).
func ftJobLinks(base *url.URL, root *html.Node) []string {
	return jobLinks(base, root, func(href string) bool { return ftJobID(href) != "" })
}

// A broken Freshteam Liquid template can omit JSON-LD while still rendering the job body.
func ftRenderedPosting(root *html.Node) (ftPosting, bool) {
	header := firstByID(root, "job-details-header")
	content := firstByClass(root, "job-details-content")
	if header == nil || content == nil {
		return ftPosting{}, false
	}
	title := firstByClass(header, "brand-color")
	var body strings.Builder
	for n := content.FirstChild; n != nil; n = n.NextSibling {
		if strings.HasPrefix(textContent(n), "Liquid error:") {
			continue
		}
		if err := html.Render(&body, n); err != nil {
			return ftPosting{}, false
		}
	}
	if title == nil || textContent(title) == "" || strings.TrimSpace(body.String()) == "" {
		return ftPosting{}, false
	}
	p := ftPosting{Title: textContent(title), Description: body.String()}

	if loc := firstByClass(header, "stick-hide-in-mobile"); loc != nil {
		var place strings.Builder
		for n := loc.FirstChild; n != nil; n = n.NextSibling {
			if n.Type == html.TextNode {
				place.WriteString(n.Data)
			}
		}
		p.JobLocation.Address.AddressLocality = collapseSpace(place.String())
	}
	return p, true
}
