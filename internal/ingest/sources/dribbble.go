package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/net/html"
)

// dribbble adapts Dribbble Jobs (dribbble.com/jobs), the design community's job board: graphic,
// product, brand and motion design, posted by employers and agencies, each naming its own
// company. Boardless aggregator. The whole inventory is small (51 postings on 2026-11-22) and
// the listing is plain server-rendered HTML, so it is read in full. See providers/dribbble.md.
//
// The listing is /jobs?page=N, 48 cards a page, ending (HTTP 200, no card) at page 3. A card
// carries no body, so this is a HydratingSource: a posting page's schema.org JobPosting is read
// only for postings the catalogue lacks. The listing files no posting under a category the
// adapter could trust, so no posting is hinted technical.
type dribbble struct {
	http HTMLGetter
	now  func() time.Time
}

const (
	dribbbleBase    = "https://dribbble.com"
	dribbbleJobsURL = dribbbleBase + "/jobs"

	// dribbbleRequestInterval paces every request of a run; the whole crawl is ~55 requests.
	dribbbleRequestInterval = time.Second
	dribbbleDetailWorkers   = 2
	// dribbbleMaxPages is a safety ceiling on the walk; the listing answers a card-less page past
	// its end.
	dribbbleMaxPages = 20
)

// dribbblePostingPath matches a posting link, /jobs/<id>-<title-slug>. The id is the identity.
// The card's "View job" link carries no query; the overlay link repeats it with ?source=index
// and is not matched.
var dribbblePostingPath = regexp.MustCompile(`^/jobs/(\d+)-([^/?#]+)$`)

// NewDribbble builds the Dribbble Jobs adapter over the given client.
func NewDribbble(c HTMLGetter) Source {
	return dribbble{http: pacedHTMLGetter(c, dribbbleRequestInterval, 1), now: time.Now}
}

func (dribbble) Provider() string { return "dribbble" }

// One public listing, no per-tenant board.
func (dribbble) Boardless() {}

// Employers and agencies post here, each named per posting.
func (dribbble) Aggregator() {}

// dribbbleLD is the part of a posting page's schema.org JobPosting the adapter reads.
type dribbbleLD struct {
	Title              string          `json:"title"`
	Description        string          `json:"description"`
	DatePosted         string          `json:"datePosted"`
	ValidThrough       string          `json:"validThrough"`
	EmploymentType     json.RawMessage `json:"employmentType"`
	JobLocationType    string          `json:"jobLocationType"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	JobLocation schemaPlaces `json:"jobLocation"`
}

// dribbblePosting is one listing entry: the identity and the page to read.
type dribbblePosting struct {
	id  string
	url string
}

func (s dribbble) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew walks the listing, refreshes the postings already stored without a request, and reads
// the page of every other one. An unreadable page retains its listing identity for retry.
func (s dribbble) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var attempted, answered atomic.Int64
	jobs := fetchDetails(postings, dribbbleDetailWorkers, func(p dribbblePosting) (Job, bool) {
		if seen(p.id) {
			return Job{ExternalID: p.id, URL: p.url, SeenRefresh: true}, true
		}
		attempted.Add(1)
		job, ok, err := s.detail(ctx, p)
		if err != nil {
			log.Printf("dribbble: posting %s failed; deferring: %v", p.id, err)
			return unreadableDetail(p.id, p.url, ""), true
		}
		answered.Add(1)
		return job, ok
	})
	// Postings were due a page and none answered: the pages changed shape or are walled, which
	// a list-only success would report as a healthy crawl.
	if attempted.Load() > 0 && answered.Load() == 0 {
		return nil, fmt.Errorf("dribbble: requested %d posting pages and none answered", attempted.Load())
	}
	return jobs, nil
}

// list walks the listing until a page adds no posting. A posting whose slug ends in -CLOSED is
// one the site keeps listed after its employer closed it.
func (s dribbble) list(ctx context.Context) ([]dribbblePosting, error) {
	links, err := crawlAllPagedLinks(ctx, s.http, dribbbleMaxPages, func(page int) string {
		if page == 1 {
			return dribbbleJobsURL
		}
		return fmt.Sprintf("%s?page=%d", dribbbleJobsURL, page)
	}, func(root *html.Node) []string {
		base, _ := url.Parse(dribbbleBase)
		return jobLinks(base, root, func(href string) bool {
			m := dribbblePostingPath.FindStringSubmatch(href)
			return m != nil && !strings.HasSuffix(strings.ToUpper(m[2]), "-CLOSED")
		})
	})
	if err != nil {
		return nil, fmt.Errorf("dribbble: listing: %w", err)
	}
	if len(links) == 0 {
		return nil, fmt.Errorf("dribbble: the listing carries no posting")
	}
	out := make([]dribbblePosting, 0, len(links))
	for _, l := range links {
		u, err := url.Parse(l)
		if err != nil {
			continue
		}
		if m := dribbblePostingPath.FindStringSubmatch(u.Path); m != nil {
			out = append(out, dribbblePosting{id: m[1], url: l})
		}
	}
	return out, nil
}

// detail reads one posting page. A confirmed removal or expiry returns no job.
// An unreadable page returns an error.
func (s dribbble) detail(ctx context.Context, p dribbblePosting) (Job, bool, error) {
	root, err := s.http.GetHTML(ctx, p.url)
	if err != nil {
		if !detailUnreadable(err) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	var ld dribbbleLD
	if !ldJobPosting(root, &ld) {
		return Job{}, false, fmt.Errorf("no JobPosting on the page")
	}
	deadline := parseRFC3339OrDate(ld.ValidThrough)
	if deadline != nil && deadline.Before(s.now()) {
		return Job{}, false, nil
	}
	company := strings.TrimSpace(ld.HiringOrganization.Name)
	body := sanitizeHTML(ld.Description)
	if company == "" || body == "" || strings.TrimSpace(ld.Title) == "" {
		return Job{}, false, fmt.Errorf("posting lacks a title, employer or description")
	}
	j := Job{
		ExternalID:          p.id,
		URL:                 p.url,
		Title:               strings.TrimSpace(ld.Title),
		Company:             company,
		Description:         body,
		PostedAt:            parseRFC3339OrDate(ld.DatePosted),
		EmploymentType:      schemaEmploymentRaw(ld.EmploymentType),
		ApplicationDeadline: deadline,
	}
	j.Location = designBoardLocation(ld.JobLocation)
	if strings.EqualFold(ld.JobLocationType, "TELECOMMUTE") {
		j.Remote = true
		j.WorkMode = "remote"
		if j.Location == "" {
			j.Location = "Remote"
		}
	}
	return j, true, nil
}

// designBoardLocation (shared with awwwards) joins the posting's address parts, leaving out the placeholders the form
// stores for a remote posting ("Anywhere", "Remote"), which say nothing about a place.
func designBoardLocation(places schemaPlaces) string {
	var parts []string
	for _, pl := range places {
		for _, v := range []string{pl.Address.AddressLocality, pl.Address.AddressRegion, pl.Address.AddressCountry} {
			v = strings.TrimSpace(v)
			if v == "" || strings.EqualFold(v, "anywhere") || strings.EqualFold(v, "remote") {
				continue
			}
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, ", ")
}
