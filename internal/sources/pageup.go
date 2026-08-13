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
// HTML fragment; the row wrapper differs per tenant (a <tr> table, a <li> list, or a <div>
// card), so parsing keys on the cross-tenant-stable a.job-link anchor and pairs each job
// with the location and summary that follow it in document order. Most tenants' listings
// carry a usable summary; a minority render nothing of the kind, so a job that comes out of
// pageupParseListing with no description gets one more fetch against its own detail page
// (see detailDescription) — bounded so tenants whose listing already has a summary never
// pay the extra request. Posted date is not exposed by either endpoint.
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

	// Some tenants' listing rows carry no summary of any shape (e.g. board 798) — the
	// full text lives only on the job's own detail page. Others carry a summary that's
	// just a short teaser rather than the posting body (pageupAlwaysDetailBoards). Fan
	// out for both, so tenants whose listing summary already IS the full text never pay
	// the extra request.
	var need []int
	for i, j := range jobs {
		if j.Description == "" || pageupAlwaysDetailBoards[e.Board] {
			need = append(need, i)
		}
	}
	if len(need) > 0 {
		filled := fetchDetails(need, defaultDetailWorkers, func(i int) (Job, bool) {
			j := jobs[i]
			j.Description = p.detailDescription(ctx, j.URL)
			return j, true
		})
		for k, i := range need {
			jobs[i] = filled[k]
		}
	}
	return jobs, nil
}

// pageupDetailDescriptionID is the container the detail page's XHR envelope wraps the full
// posting body in, on tenants whose listing omits a summary (confirmed on board 798). It
// is a platform template id, not something a tenant's own theme reliably reuses — a tenant
// whose detail markup uses a different container (board 709, which needs no detail fetch
// since its listing already carries a summary) simply yields "" here, same as a fetch
// failure: the posting keeps its empty description rather than being dropped.
const pageupDetailDescriptionID = "job-details"

// pageupAlwaysDetailBoards are tenants whose listing summary is a short teaser (one or two
// sentences) rather than the actual posting body, so the "already has a summary" skip would
// otherwise leave every job on the board stuck with that teaser. Confirmed on board 889
// (Flight Centre): its listing row's "summary" carries a marketing blurb, while the full
// posting lives in the same #job-details container the no-summary tenants use.
var pageupAlwaysDetailBoards = map[string]bool{
	"889": true,
}

// detailDescription fetches one job's detail page and returns its sanitized description
// HTML, or "" when the fetch fails or the known container is absent.
func (p pageup) detailDescription(ctx context.Context, jobURL string) string {
	var env pageupEnvelope
	if err := p.http.GetJSONWithHeaders(ctx, jobURL, pageupXHR, &env); err != nil {
		return ""
	}
	root, err := html.Parse(strings.NewReader(env.Results))
	if err != nil {
		return ""
	}
	node := firstByID(root, pageupDetailDescriptionID)
	if node == nil {
		return ""
	}
	return sanitizeHTML(innerHTML(node))
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
		// Some tenants render the summary as a bare <p> with no class at all (e.g. board
		// 709); catch it too, so long as nothing has claimed the description yet.
		case n.Data == "p" && attr(n, "class") == "" && cur.Description == "":
			cur.Description = textContent(n)
		}
		return true
	})
	flush()
	return jobs, nil
}
