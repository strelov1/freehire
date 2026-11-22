package sources

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

// awwwards adapts Awwwards Jobs (awwwards.com/jobs), the web-design showcase's job board:
// UI/UX, art direction and creative development, posted by studios and agencies, each naming
// its own company. Boardless aggregator. The inventory is tiny (17 postings on 2026-11-22), so
// the listing is read in full. See providers/awwwards.md.
//
// The listing is /jobs/?page=N, 12 cards a page; a page past the end answers 404. A card
// carries no body, so this is a HydratingSource: a posting page's schema.org JobPosting is read
// only for postings the catalogue lacks. The site's own "programming" category listing names
// the postings it files as development work, and only those carry IsTechHint.
type awwwards struct {
	http HTMLGetter
}

const (
	awwwardsBase           = "https://www.awwwards.com"
	awwwardsJobsURL        = awwwardsBase + "/jobs/"
	awwwardsProgrammingURL = awwwardsJobsURL + "programming/"

	awwwardsRequestInterval = time.Second
	awwwardsDetailWorkers   = 2
	// awwwardsMaxPages is a safety ceiling on the walk.
	awwwardsMaxPages = 20
)

// awwwardsPostingPath matches a posting link, /jobs/<slug>.html. The slug is the identity.
var awwwardsPostingPath = regexp.MustCompile(`^/jobs/([a-z0-9][a-z0-9-]*)\.html$`)

// NewAwwwards builds the Awwwards Jobs adapter over the given client.
func NewAwwwards(c HTMLGetter) Source {
	return awwwards{http: pacedHTMLGetter(c, awwwardsRequestInterval, 1)}
}

func (awwwards) Provider() string { return "awwwards" }

// One public listing, no per-tenant board.
func (awwwards) Boardless() {}

// Studios and agencies post here, each named per posting.
func (awwwards) Aggregator() {}

// awwwardsLD is the part of a posting page's schema.org JobPosting the adapter reads. The page
// states no employment type and no work model.
type awwwardsLD struct {
	Title              string `json:"title"`
	Description        string `json:"description"`
	DatePosted         string `json:"datePosted"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	JobLocation schemaPlaces `json:"jobLocation"`
}

// awwwardsPosting is one listing entry: the identity and the page to read.
type awwwardsPosting struct {
	slug string
	url  string
}

func (s awwwards) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew walks the listing, refreshes the postings already stored without a request, and reads
// the page of every other one. An unreadable page retains its listing identity for retry.
func (s awwwards) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := s.list(ctx, awwwardsJobsURL, true)
	if err != nil {
		return nil, err
	}
	dev := s.developmentSlugs(ctx)
	var attempted, answered atomic.Int64
	jobs := fetchDetails(postings, awwwardsDetailWorkers, func(p awwwardsPosting) (Job, bool) {
		if seen(p.slug) {
			return Job{ExternalID: p.slug, URL: p.url, SeenRefresh: true}, true
		}
		attempted.Add(1)
		job, ok, err := s.detail(ctx, p)
		if err != nil {
			log.Printf("awwwards: posting %s failed; deferring: %v", p.slug, err)
			return unreadableDetail(p.slug, p.url, ""), true
		}
		answered.Add(1)
		job.IsTechHint = dev[p.slug]
		return job, ok
	})
	// Postings were due a page and none answered: the pages changed shape or are walled, which
	// a list-only success would report as a healthy crawl.
	if attempted.Load() > 0 && answered.Load() == 0 {
		return nil, fmt.Errorf("awwwards: requested %d posting pages and none answered", attempted.Load())
	}
	return jobs, nil
}

// developmentSlugs is the set of postings the site's "programming" category lists. The hint is
// optional, so a listing that cannot be read costs the hint, never the crawl.
func (s awwwards) developmentSlugs(ctx context.Context) map[string]bool {
	postings, err := s.list(ctx, awwwardsProgrammingURL, false)
	if err != nil {
		log.Printf("awwwards: programming category unreadable; no posting is hinted technical: %v", err)
		return nil
	}
	set := make(map[string]bool, len(postings))
	for _, p := range postings {
		set[p.slug] = true
	}
	return set
}

// list walks a listing from page 1 until a page adds no posting or answers 404 (past the end).
// mustHave makes an empty first page a failure rather than a quiet category.
func (s awwwards) list(ctx context.Context, listURL string, mustHave bool) ([]awwwardsPosting, error) {
	base, _ := url.Parse(awwwardsBase)
	var out []awwwardsPosting
	seen := map[string]bool{}
	for page := 1; page <= awwwardsMaxPages; page++ {
		u := listURL
		if page > 1 {
			u = fmt.Sprintf("%s?page=%d", listURL, page)
		}
		root, err := s.http.GetHTML(ctx, u)
		if err != nil {
			var se *StatusError
			if page > 1 && errors.As(err, &se) && se.Code == http.StatusNotFound {
				break
			}
			return nil, fmt.Errorf("awwwards: listing %s: %w", u, err)
		}
		added := 0
		for _, link := range jobLinks(base, root, func(href string) bool { return awwwardsPostingPath.MatchString(href) }) {
			ref, err := url.Parse(link)
			if err != nil {
				continue
			}
			m := awwwardsPostingPath.FindStringSubmatch(ref.Path)
			if m == nil || seen[m[1]] {
				continue
			}
			seen[m[1]] = true
			out = append(out, awwwardsPosting{slug: m[1], url: link})
			added++
		}
		if added == 0 {
			break
		}
	}
	if mustHave && len(out) == 0 {
		return nil, fmt.Errorf("awwwards: the listing carries no posting")
	}
	return out, nil
}

// detail reads one posting page. A confirmed removal returns no job.
// An unreadable page returns an error.
func (s awwwards) detail(ctx context.Context, p awwwardsPosting) (Job, bool, error) {
	root, err := s.http.GetHTML(ctx, p.url)
	if err != nil {
		if !detailUnreadable(err) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	var ld awwwardsLD
	if !ldJobPosting(root, &ld) {
		return Job{}, false, fmt.Errorf("no JobPosting on the page")
	}
	company := strings.TrimSpace(ld.HiringOrganization.Name)
	body := sanitizeHTML(ld.Description)
	if company == "" || body == "" || strings.TrimSpace(ld.Title) == "" {
		return Job{}, false, fmt.Errorf("posting lacks a title, employer or description")
	}
	return Job{
		ExternalID:  p.slug,
		URL:         p.url,
		Title:       strings.TrimSpace(ld.Title),
		Company:     company,
		Location:    designBoardLocation(ld.JobLocation),
		Description: body,
		// The page spells the date out: "November 03 2026".
		PostedAt: parseLayout("January 02 2006", strings.TrimSpace(ld.DatePosted)),
	}, true, nil
}
