package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// pageup adapts PageUp People career sites, a multi-tenant ATS. Every tenant is served keylessly
// through careers.pageuppeople.com/<instID>/, except verified custom board 1023.
// The board remains the numeric institution id. The
// listing endpoint returns a JSON envelope whose "results" is a rendered HTML fragment; the row
// wrapper differs per tenant (<tr>, <li> or <div>), so parsing keys on the stable a.job-link
// anchor and pairs each job with the location and summary that follow it in document order. A
// job with no listing summary gets one more fetch against its detail page (detailDescription).
// Posted date is not exposed by either endpoint.
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
var pageupNorthMemorialJobRe = regexp.MustCompile(`^/nmh/en-us/job/(\d+)/`)

// pageupEnvelope is the search endpoint's JSON envelope; results is an HTML fragment.
type pageupEnvelope struct {
	Results string `json:"results"`
	Count   *int   `json:"count"`
}

func (p pageup) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	searchURL := fmt.Sprintf("%s/%s/cw/en/search/?page-items=1000", pageupHost, e.Board)
	if e.Board == "1023" {
		searchURL = "https://jobs.northmemorial.com/nmh/en-us/search/?page-items=1000"
	}
	var env pageupEnvelope
	if err := p.http.GetJSONWithHeaders(ctx, searchURL, pageupXHR, &env); err != nil {
		return nil, fmt.Errorf("pageup: search %s: %w", e.Board, err)
	}
	jobs, err := pageupParseListing(env.Results, e.Board)
	if err != nil {
		return nil, fmt.Errorf("pageup: parse %s: %w", e.Board, err)
	}
	if e.Board == "1023" && env.Count == nil {
		return nil, fmt.Errorf("pageup: search %s: missing count", e.Board)
	}
	if env.Count != nil && len(jobs) != *env.Count {
		return nil, fmt.Errorf("pageup: incomplete listing %s: recognized %d of %d", e.Board, len(jobs), *env.Count)
	}
	for i := range jobs {
		jobs[i].Company = e.Company
	}

	// Some tenants' rows carry no summary (the text lives only on the detail page); others only
	// a short teaser (pageupAlwaysDetailBoards). Fan out for both, so tenants whose summary is
	// the full text never pay the extra request.
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

// pageupDetailDescriptionID is the container the detail page's XHR envelope wraps the posting
// body in. A tenant theme with a different container yields "" here, like a fetch failure: the
// posting keeps an empty description rather than being dropped.
const pageupDetailDescriptionID = "job-details"

// pageupAlwaysDetailBoards are tenants whose listing summary is a short marketing teaser, so the
// "already has a summary" skip would leave every job stuck with it; the full posting is in the
// same #job-details container.
var pageupAlwaysDetailBoards = map[string]bool{
	"889":  true,
	"1023": true,
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

// pageupParseListing parses a search-results HTML fragment into jobs. An a.job-link starts a new
// job, and the location and summary that follow before the next job-link attach to it; the link
// is the only anchor common to the <tr> and <li> layouts.
func pageupParseListing(fragment, board string) ([]Job, error) {
	base := pageupHost
	jobRe := pageupJobRe
	if board == "1023" {
		base = "https://jobs.northmemorial.com"
		jobRe = pageupNorthMemorialJobRe
	}
	// Bare <tr>s meant for an existing <table> lose their cells to foster-parenting and drop the
	// summary row, so supply the table wrapper.
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
	seen := map[string]int{}
	flush := func() {
		if cur != nil && cur.ExternalID != "" {
			if i, ok := seen[cur.ExternalID]; ok {
				if jobs[i].Title == "" {
					jobs[i].Title = cur.Title
				}
				if jobs[i].Location == "" {
					jobs[i].Location = cur.Location
				}
				if jobs[i].Description == "" {
					jobs[i].Description = cur.Description
				}
			} else {
				seen[cur.ExternalID] = len(jobs)
				jobs = append(jobs, *cur)
			}
		}
		cur = nil
	}
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode {
			return true
		}
		switch {
		case n.Data == "a" && HasClass(n, "job-link"):
			href := Attr(n, "href")
			parsed, err := url.Parse(href)
			if err != nil || (parsed.IsAbs() && parsed.Scheme+"://"+parsed.Host != base) {
				return true
			}
			m := jobRe.FindStringSubmatch(parsed.Path)
			if m == nil {
				return true // a nav/back link, not a posting
			}
			flush()
			cur = &Job{
				ExternalID: m[1],
				Title:      textContent(n),
				URL:        base + parsed.RequestURI(),
			}
		case cur == nil:
			// nothing to attach yet
		case HasClass(n, "location") && cur.Location == "":
			cur.Location = textContent(n)
		case (HasClass(n, "jobs-summary") || HasClass(n, "summary")) && cur.Description == "":
			cur.Description = textContent(n)
		// Some tenants render the summary as a bare classless <p>.
		case n.Data == "p" && Attr(n, "class") == "" && cur.Description == "":
			cur.Description = textContent(n)
		}
		return true
	})
	flush()
	return jobs, nil
}
