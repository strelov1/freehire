package sources

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"

	"golang.org/x/net/html"
)

// hrmosMaxPages bounds the listing walk. The largest live sample measured (CyberAgent Group,
// 413 jobs at 100/page) needs 5; this leaves generous headroom the same way other per-tenant
// paginated adapters in this package do.
const hrmosMaxPages = 50

// hrmos adapts HRMOS career pages (hrmos.co), a Japanese multi-tenant ATS by Bizreach. The
// board is the company's HRMOS slug (a vanity slug or an opaque numeric id — both appear in
// the wild, e.g. "cyberagent-group" or "1218800560317673472"). The listing
// (hrmos.co/pages/<board>/jobs) pages via "?page=N"; each job page carries a standard
// schema.org JobPosting ld+json block, read via the same shared helper herp/breezy/teamtailor
// already use.
type hrmosHTTP interface {
	HTMLGetter
}

type hrmos struct {
	http hrmosHTTP
}

// NewHrmos builds the HRMOS adapter over the given HTTP client.
func NewHrmos(c hrmosHTTP) Source { return hrmos{http: c} }

func (hrmos) Provider() string { return "hrmos" }

// fullBoardListing: crawlAllPagedLinks fails the whole Fetch if ANY listing page fails, not
// just the first — never a silently truncated listing. See the fullBoardListing interface
// for the bar.
func (hrmos) fullBoardListing() {}

func (h hrmos) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	base, err := url.Parse(fmt.Sprintf("https://hrmos.co/pages/%s/jobs", e.Board))
	if err != nil {
		return nil, fmt.Errorf("hrmos: bad board %s: %w", e.Board, err)
	}

	links, err := crawlAllPagedLinks(ctx, h.http, hrmosMaxPages,
		func(page int) string { return fmt.Sprintf("%s?page=%d", base, page) },
		func(root *html.Node) []string {
			return jobLinks(base, root, func(href string) bool { return isHrmosJobLink(e.Board, href) })
		})
	if err != nil {
		return nil, fmt.Errorf("hrmos: list board %s: %w", e.Board, err)
	}

	return fetchDetails(links, defaultDetailWorkers, func(link string) (Job, bool) {
		return h.detail(ctx, e, link)
	}), nil
}

// hrmosEmploymentTypes maps schema.org's employmentType enum onto vocab.EmploymentTypeValues.
// Only a value with a genuine freehire equivalent is mapped; TEMPORARY, VOLUNTEER, PER_DIEM,
// and OTHER have none and are deliberately absent, so the pipeline's own dictionary decides
// rather than this adapter guessing a best-effort match.
var hrmosEmploymentTypes = map[string]string{
	"FULL_TIME":  "full_time",
	"PART_TIME":  "part_time",
	"CONTRACTOR": "contract",
	"INTERN":     "internship",
}

// detail fetches one job page for its JobPosting ld+json block, mapping it to a Job. A page
// the platform reports gone (404/410) is dropped; any other read failure — including a 200
// carrying no JobPosting block — comes back as an unreadableDetail marker, since the page is
// the posting's only source and a dropped posting would be indistinguishable from one taken
// down.
func (h hrmos) detail(ctx context.Context, e CompanyEntry, link string) (Job, bool) {
	id := path.Base(link)
	root, err := h.http.GetHTML(ctx, link)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(id, link, e.Company), true
		}
		return Job{}, false
	}

	var p struct {
		Title          string `json:"title"`
		Description    string `json:"description"`
		DatePosted     string `json:"datePosted"`
		EmploymentType string `json:"employmentType"`
		JobLocation    []struct {
			Address struct {
				AddressLocality string `json:"addressLocality"`
				AddressRegion   string `json:"addressRegion"`
			} `json:"address"`
		} `json:"jobLocation"`
	}
	if !ldJobPosting(root, &p) || p.Title == "" {
		return unreadableDetail(id, link, e.Company), true
	}

	var location string
	if len(p.JobLocation) > 0 {
		addr := p.JobLocation[0].Address
		location = joinNonEmpty(addr.AddressLocality, addr.AddressRegion)
	}

	return Job{
		ExternalID:     id,
		URL:            link,
		Title:          p.Title,
		Company:        e.Company,
		Location:       location,
		Description:    sanitizeHTML(p.Description),
		PostedAt:       parseRFC3339(p.DatePosted),
		EmploymentType: hrmosEmploymentTypes[p.EmploymentType],
	}, true
}

// isHrmosJobLink reports whether href is a job posting link for board: exactly
// "/pages/<board>/jobs/<jobID>" (host empty or hrmos.co). Requiring the literal "jobs"
// segment — rather than accepting any single segment after the board — deliberately avoids
// the class of bug herp-source's review found: a looser one-segment match also matches the
// platform's own single-segment navigation links.
func isHrmosJobLink(board, href string) bool {
	u, err := url.Parse(href)
	if err != nil {
		return false
	}
	if u.Host != "" && u.Host != "hrmos.co" {
		return false
	}
	prefix := "/pages/" + board + "/jobs/"
	if !strings.HasPrefix(u.Path, prefix) {
		return false
	}
	rest := strings.TrimPrefix(u.Path, prefix)
	return rest != "" && !strings.Contains(rest, "/")
}
