package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"

	"golang.org/x/net/html"
)

// odoo adapts self-hosted Odoo recruitment sites (the website_hr_recruitment module). The
// board is the careers host (e.g. jobs.<company>.com or <company>.com). GET /jobs is a
// server-rendered listing whose cards link to /jobs/detail/<slug>-<id> pages carrying a
// schema.org JobPosting expressed as microdata (itemprop): the description
// (itemprop="description") and datePosted, with the title in the page <h1>. There is no
// pagination — Odoo lists all published positions on the one page.
type odoo struct {
	http HTMLGetter
}

// NewOdoo builds the Odoo adapter over the given HTML client.
func NewOdoo(c HTMLGetter) Source { return odoo{http: c} }

func (odoo) Provider() string { return "odoo" }

func (s odoo) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	base, err := url.Parse(fmt.Sprintf("https://%s/jobs", e.Board))
	if err != nil {
		return nil, fmt.Errorf("odoo: board %q: %w", e.Board, err)
	}
	root, err := s.http.GetHTML(ctx, base.String())
	if err != nil {
		return nil, fmt.Errorf("odoo: listing %s: %w", e.Board, err)
	}
	locs := jobLinks(base, root, func(href string) bool { return odooJobID(href) != "" })
	return fetchDetails(locs, defaultDetailWorkers, func(loc string) (Job, bool) {
		return s.detail(ctx, e, loc)
	}), nil
}

// detail fetches one job's detail page and maps its JobPosting microdata to a Job, returning
// ok=false when the fetch fails or the page carries no id/description so the caller skips it.
func (s odoo) detail(ctx context.Context, e CompanyEntry, loc string) (Job, bool) {
	id := odooJobID(loc)
	if id == "" {
		return Job{}, false
	}
	root, err := s.http.GetHTML(ctx, loc)
	if err != nil {
		return Job{}, false
	}
	description := sanitizeHTML(itempropHTML(root, "description"))
	if description == "" {
		return Job{}, false // not a JobPosting page (or an empty stub) — skip it
	}
	title := firstNonEmpty(firstElementText(root, "h1"), itempropText(root, "title"))
	location := joinNonEmpty(itempropText(root, "addressLocality"), itempropText(root, "addressCountry"))

	return Job{
		ExternalID:  id,
		URL:         loc,
		Title:       title,
		Company:     e.Company,
		Location:    location,
		Description: description,
		Remote:      isRemote(location),
		PostedAt:    parseDate(itempropAttr(root, "datePosted")),
	}, true
}

// odooJobIDPattern captures the trailing numeric id from a /jobs/<slug>-<id> or
// /jobs/detail/<slug>-<id> URL (any leading language prefix is ignored).
var odooJobIDPattern = regexp.MustCompile(`/jobs/(?:detail/)?[^"?#]*?-(\d+)(?:[/?#]|$)`)

// odooJobID extracts the native numeric job id from a detail URL, or "" when the URL is not
// a job posting (e.g. the /jobs listing itself).
func odooJobID(loc string) string {
	if m := odooJobIDPattern.FindStringSubmatch(loc); m != nil {
		return m[1]
	}
	return ""
}

// itempropAttr returns the datetime/content attribute of the first element carrying the given
// schema.org itemprop (Odoo dates a posting with a datePosted whose value is an attribute, not
// text), or "" when none is present.
func itempropAttr(root *html.Node, prop string) string {
	ns := findItemprops(root, prop)
	if len(ns) == 0 {
		return ""
	}
	return firstNonEmpty(attr(ns[0], "datetime"), attr(ns[0], "content"))
}
