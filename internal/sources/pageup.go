package sources

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// pageup adapts PageUp People career sites — a multi-tenant ATS common in AU/UK/NZ higher
// education. Every tenant is served keylessly through the canonical host
// careers.pageuppeople.com/<instID>/, so the board is the numeric institution id (e.g.
// "513"). The listing endpoint returns a JSON envelope whose "results" field is a rendered
// HTML fragment; the row wrapper differs per tenant (a <tr> table or a <li> list), so parsing
// keys on the cross-tenant-stable a.job-link anchor and pairs each job with the location and
// summary that follow it in document order. Posted date and the full description live only on
// the per-tenant detail page (whose markup varies too), so this list-only adapter carries the
// listing's summary snippet; a detail fan-out is a possible later enrichment.
type pageup struct {
	http pageupHTTP
}

// pageupHTTP is the transport PageUp needs: a JSON GET that can set the XHR header the
// endpoint requires (without it the search URL 302s to the full HTML page).
type pageupHTTP interface {
	GetJSONWithHeaders(ctx context.Context, url string, headers map[string]string, v any) error
}

// NewPageUp builds the PageUp adapter over the given HTTP client.
func NewPageUp(c pageupHTTP) Source { return pageup{http: c} }

func (pageup) Provider() string { return "pageup" }

// pageupHost is the canonical multi-tenant host; vanity hosts often sit behind a WAF, this
// one is served cleanly and keylessly.
const pageupHost = "https://careers.pageuppeople.com"

// pageupXHR is the header the search/detail endpoints require to return their JSON envelope.
var pageupXHR = map[string]string{"X-Requested-With": "XMLHttpRequest"}

// pageupJobRe extracts the stable numeric job id from a job-link href of the form
// /<instID>/cw/en/job/<jobID>/<slug>.
var pageupJobRe = regexp.MustCompile(`/cw/en/job/(\d+)/`)

// pageupEnvelope is the search endpoint's JSON envelope; results is an HTML fragment.
type pageupEnvelope struct {
	Results string `json:"results"`
	Count   int    `json:"count"`
}

func (p pageup) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	url := fmt.Sprintf("%s/%s/cw/en/search/?page-items=1000", pageupHost, e.Board)
	var env pageupEnvelope
	if err := p.http.GetJSONWithHeaders(ctx, url, pageupXHR, &env); err != nil {
		return nil, fmt.Errorf("pageup: search %s: %w", e.Board, err)
	}
	jobs, err := pageupParseListing(env.Results, e.Board)
	if err != nil {
		return nil, fmt.Errorf("pageup: parse %s: %w", e.Board, err)
	}
	for i := range jobs {
		jobs[i].Company = e.Company
	}
	return jobs, nil
}

// pageupParseListing parses a search-results HTML fragment into jobs. It walks the DOM in
// document order: an a.job-link starts a new job (id/title/url), and the location and
// summary that follow before the next job-link are attached to it — a pairing that holds
// across the <tr> and <li> tenant layouts, whose only reliable common anchor is the link.
func pageupParseListing(fragment, board string) ([]Job, error) {
	// A tenant that renders rows returns bare <tr>s meant to be injected into an existing
	// <table>; parsing them without that context foster-parents the cells out and drops the
	// summary row, so supply the table wrapper. The <li> layout carries no <tr> and parses
	// as-is.
	if strings.Contains(fragment, "<tr") {
		fragment = "<table>" + fragment + "</table>"
	}
	root, err := html.Parse(strings.NewReader(fragment))
	if err != nil {
		return nil, err
	}
	var (
		jobs []Job
		cur  *Job
	)
	flush := func() {
		if cur != nil && cur.ExternalID != "" {
			jobs = append(jobs, *cur)
		}
		cur = nil
	}
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode {
			return true
		}
		switch {
		case n.Data == "a" && hasClass(n, "job-link"):
			href := attr(n, "href")
			m := pageupJobRe.FindStringSubmatch(href)
			if m == nil {
				return true // a nav/back link, not a posting
			}
			flush()
			cur = &Job{
				ExternalID: m[1],
				Title:      textContent(n),
				URL:        pageupHost + href,
			}
		case cur == nil:
			// nothing to attach yet
		case hasClass(n, "location") && cur.Location == "":
			cur.Location = textContent(n)
		case (hasClass(n, "jobs-summary") || hasClass(n, "summary")) && cur.Description == "":
			cur.Description = textContent(n)
		}
		return true
	})
	flush()
	return jobs, nil
}
