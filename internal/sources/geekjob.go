package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// geekjob adapts geekjob.ru, a Russian IT job board. The public listing is server-rendered HTML
// paginated as /vacancies/N (1-based), each card linking to a /vacancy/<hex id> detail page whose
// schema.org JobPosting ld+json carries the posting (title, company, description, date). geekjob
// has no board API, so the description and structured fields come from a per-vacancy detail fetch,
// fanned out under a bounded pool like the other JSON-LD detail adapters (careerspage, freshteam).
//
// Boardless (geekjob.ru is one site with no per-tenant board) and an aggregator (many companies;
// each posting's employer comes from the feed's hiringOrganization, not a placeholder), so it
// stays in the source facet and inherits the reindex aggregator/ATS-duplicate suppression.
type geekjob struct {
	http HTMLGetter
}

// NewGeekjob builds the geekjob.ru listing adapter over the given HTML client.
func NewGeekjob(c HTMLGetter) Source { return geekjob{http: c} }

func (geekjob) Provider() string { return "geekjob" }

func (geekjob) boardless() {}

func (geekjob) aggregator() {}

const (
	// geekjobBaseURL is the site root the listing's relative /vacancy/<id> links resolve against.
	geekjobBaseURL = "https://geekjob.ru/"
	// geekjobMaxPages caps the /vacancies/N walk so a listing that never yields an empty page
	// cannot loop forever. The public feed is ~13 pages; this is ample headroom.
	geekjobMaxPages = 100
)

func (s geekjob) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	base, _ := url.Parse(geekjobBaseURL) // a constant literal never fails to parse

	// Page through the listing until a page yields no new links (the tail/empty page, which the
	// public feed reaches around page 14) or the safety cap is hit. A first-page failure is a
	// board-level error; a later-page failure just stops the walk with what we have.
	locs, err := crawlPagedLinks(ctx, s.http, geekjobMaxPages,
		func(page int) string { return fmt.Sprintf("%svacancies/%d", geekjobBaseURL, page) },
		func(root *html.Node) []string {
			return jobLinks(base, root, func(href string) bool { return geekjobJobID(href) != "" })
		})
	if err != nil {
		return nil, fmt.Errorf("geekjob: listing: %w", err)
	}

	// Each posting's fields come from its own detail fetch (the listing card omits the
	// description), fanned out under the shared detail pool.
	return fetchDetails(locs, defaultDetailWorkers, func(loc string) (Job, bool) {
		return s.detail(ctx, e, loc)
	}), nil
}

// detail fetches one vacancy's detail page and maps its JobPosting ld+json to a Job, returning
// ok=false when the fetch fails, the page carries no JobPosting, or the URL yields no native id,
// so the caller skips just that posting.
func (s geekjob) detail(ctx context.Context, e CompanyEntry, loc string) (Job, bool) {
	root, err := s.http.GetHTML(ctx, loc)
	if err != nil {
		return Job{}, false
	}
	var p geekjobPosting
	if !ldJobPosting(root, &p) || p.Title == "" {
		return Job{}, false
	}
	id := geekjobJobID(loc)
	if id == "" {
		return Job{}, false
	}

	location := p.location()
	// geekjob states the work arrangement only in the "jobinfo" chip block, never in the ld+json,
	// so the mode and the remote flag are read from there.
	mode, remote := geekjobArrangement(root)

	return Job{
		ExternalID:     id,
		URL:            loc,
		Title:          p.Title,
		Company:        firstNonEmpty(p.HiringOrganization.Name, e.Company),
		Location:       location,
		Description:    sanitizeHTML(html.UnescapeString(p.Description)),
		Remote:         remote || isRemote(location),
		WorkMode:       mode,
		EmploymentType: schemaEmploymentType(p.EmploymentType),
		PostedAt:       parseRFC3339OrDate(p.DatePosted),
	}, true
}

// geekjobPosting is the schema.org JobPosting decoded from a detail page's ld+json block.
// jobLocation is a single Place in practice, but decoded through schemaPlaces so an array form
// (should geekjob ever emit one) does not fail the whole posting's unmarshal.
type geekjobPosting struct {
	Title              string `json:"title"`
	Description        string `json:"description"`
	DatePosted         string `json:"datePosted"`
	EmploymentType     string `json:"employmentType"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	JobLocation schemaPlaces `json:"jobLocation"`
}

// location builds the free-text location from the first jobLocation place, or "" when the posting
// carries none (a remote-only vacancy often omits it).
func (p geekjobPosting) location() string {
	if len(p.JobLocation) == 0 {
		return ""
	}
	return p.JobLocation[0].Address.Location()
}

// geekjobJobIDPattern captures the 24-hex vacancy id from a /vacancy/<id> URL. The match must end
// at the id (end-of-string or a ?/# suffix), so a tracking suffix does not defeat it and the
// plural /vacancies listing path never matches.
var geekjobJobIDPattern = regexp.MustCompile(`/vacancy/([0-9a-f]{24})(?:$|[?#])`)

// geekjobJobID extracts the native vacancy id from a URL, or "" when the URL is not a vacancy
// detail link.
func geekjobJobID(loc string) string { return firstSubmatch(geekjobJobIDPattern, loc) }

const (
	// geekjobRemoteChip and geekjobOfficeChip are the two work-arrangement chips geekjob renders
	// in the "jobinfo" block, bullet-separated and followed by the experience line.
	geekjobRemoteChip = "удаленная работа"
	geekjobOfficeChip = "работа в офисе"
)

// geekjobArrangement reads the work arrangement from a detail page's "jobinfo" chip block. A
// posting that offers remote is treated as remote — freehire is remote-first and the remote is
// genuinely available — so the remote chip wins even when an office chip is co-listed; an
// office-only posting maps to onsite; neither leaves the mode unset for the pipeline/LLM to
// resolve. The bool reports whether the remote chip is present, for the Job.Remote flag.
func geekjobArrangement(root *html.Node) (workMode string, remote bool) {
	info := firstByClass(root, "jobinfo")
	if info == nil {
		return "", false
	}
	text := strings.ToLower(textContent(info))
	switch {
	case strings.Contains(text, geekjobRemoteChip):
		return "remote", true
	case strings.Contains(text, geekjobOfficeChip):
		return "onsite", false
	default:
		return "", false
	}
}
