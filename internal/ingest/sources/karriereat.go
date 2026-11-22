package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// karriereat adapts karriere.at, Austria's largest job board: ~13,250 postings on 2026-11-22
// across every profession, each naming its own employer. It is boardless and an aggregator.
//
// The posting set is the jobs sitemap robots.txt advertises (robots.txt allows everything for
// every agent but two SEO crawlers). It carries each posting's id and last-modified time, so every
// fact is bought with the posting page's schema.org JobPosting, read from /jobs/<id>. A plain
// client is served from the crawl host. The adapter hydrates only postings the catalogue lacks,
// newest first plus a moving backlog window (hydrationLanes), and refreshes the rest by identity.
// See providers/karriereat.md.
type karriereat struct {
	sitemap        XMLGetter
	pages          HTMLGetter
	limiter        waiter
	fresh, backlog int
	deadlineMargin time.Duration
	now            func() time.Time
}

const (
	karriereatHost       = "www.karriere.at"
	karriereatSitemapURL = "https://" + karriereatHost + "/static/sitemaps/sitemap-jobs-https.xml"

	// karriereatRequestInterval paces every posting page at one a second, a courtesy ceiling
	// toward a large site rather than a measured limit.
	karriereatRequestInterval = time.Second
	karriereatDetailWorkers   = 2
	// A run reads the newest 900 unseen postings and a moving backlog window of 300: 1,200 pages
	// at one a second is twenty minutes of the scheduler's default 2,400 s crawl budget. The
	// sitemap's last-modified times moved on 750-1,800 postings a day, so a cold start of ~13k
	// postings drains over about ten runs.
	karriereatFresh          = 900
	karriereatBacklog        = 300
	karriereatDeadlineMargin = 5 * time.Minute
)

// NewKarriereAt builds the adapter: the sitemap is read over sitemap, the posting pages over pages.
func NewKarriereAt(sitemap XMLGetter, pages HTMLGetter) Source {
	return karriereat{
		sitemap:        sitemap,
		pages:          pages,
		limiter:        rate.NewLimiter(rate.Every(karriereatRequestInterval), 1),
		fresh:          karriereatFresh,
		backlog:        karriereatBacklog,
		deadlineMargin: karriereatDeadlineMargin,
		now:            time.Now,
	}
}

func (karriereat) Provider() string { return "karriereat" }

// One national catalogue, no per-tenant board id.
func (karriereat) Boardless() {}

// Every posting names its own employer, so karriere.at stays in the source facet and in the
// cross-source duplicate suppression set.
func (karriereat) Aggregator() {}

// karriereatPostingPath matches a posting URL, /jobs/<id>.
var karriereatPostingPath = regexp.MustCompile(`^/jobs/(\d+)$`)

// karriereatPosting is one sitemap entry: the posting's id, its canonical URL and the time the
// sitemap says it last changed.
type karriereatPosting struct {
	id      string
	url     string
	lastMod string
}

func karriereatParsePosting(loc sitemapLoc) (karriereatPosting, bool) {
	raw := strings.TrimSpace(loc.Loc)
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != karriereatHost {
		return karriereatPosting{}, false
	}
	m := karriereatPostingPath.FindStringSubmatch(u.Path)
	if m == nil {
		return karriereatPosting{}, false
	}
	return karriereatPosting{id: m[1], url: "https://" + karriereatHost + "/jobs/" + m[1], lastMod: strings.TrimSpace(loc.LastMod)}, true
}

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s karriereat) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew lists the sitemap, refreshes the postings already stored, and reads a bounded slice of
// the rest. A posting whose page cannot be read is not yielded; a later run tries again.
func (s karriereat) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	var unseen []karriereatPosting
	for _, p := range postings {
		if seen(p.id) {
			jobs = append(jobs, Job{ExternalID: p.id, URL: p.url, SeenRefresh: true})
			continue
		}
		unseen = append(unseen, p)
	}
	picked := hydrationLanes(unseen, s.fresh, s.backlog, s.now())
	var answered, outOfTime atomic.Int64
	read := fetchDetails(picked, karriereatDetailWorkers, func(p karriereatPosting) (Job, bool) {
		if d, ok := ctx.Deadline(); ok && time.Until(d) < s.deadlineMargin {
			outOfTime.Add(1)
			return Job{}, false
		}
		job, ok, err := s.detail(ctx, p)
		if err != nil {
			log.Printf("karriereat: posting %s failed; deferring: %v", p.id, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, ok
	})
	if asked := int64(len(picked)) - outOfTime.Load(); asked > 0 && answered.Load() == 0 {
		return nil, fmt.Errorf("karriereat: requested %d posting pages and none answered", asked)
	}
	if n := outOfTime.Load(); n > 0 {
		log.Printf("karriereat: %d new postings left unread as the run neared its deadline, deferred to later runs", n)
	}
	if n := len(unseen) - len(picked); n > 0 {
		log.Printf("karriereat: %d new postings past this run's bound of %d, deferred to later runs", n, s.fresh+s.backlog)
	}
	return append(jobs, read...), nil
}

// list reads the jobs sitemap, newest first. It failing fails the crawl; so does a sitemap that
// lists no posting.
func (s karriereat) list(ctx context.Context) ([]karriereatPosting, error) {
	doc, err := getSitemap(ctx, s.sitemap, karriereatSitemapURL)
	if err != nil {
		return nil, fmt.Errorf("karriereat: sitemap: %w", err)
	}
	ids := map[string]bool{}
	var out []karriereatPosting
	for _, u := range doc.URLs {
		p, ok := karriereatParsePosting(u)
		if !ok || ids[p.id] {
			continue
		}
		ids[p.id] = true
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("karriereat: sitemap lists no posting")
	}
	// lastmod is an RFC 3339 timestamp, so it orders as text.
	sort.SliceStable(out, func(i, j int) bool { return out[i].lastMod > out[j].lastMod })
	return out, nil
}

// karriereatLD is the part of the posting page's schema.org JobPosting the adapter reads.
type karriereatLD struct {
	Title              string                `json:"title"`
	Description        string                `json:"description"`
	DatePosted         string                `json:"datePosted"`
	EmploymentType     json.RawMessage       `json:"employmentType"`
	JobLocation        schemaPlaces          `json:"jobLocation"`
	BaseSalary         *schemaMonetaryAmount `json:"baseSalary"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
}

// errKarriereatNoPosting marks a page that answered 200 without a JobPosting. The site answers a
// removed posting with a 404, so this is not a "gone" answer and counts as no answer.
var errKarriereatNoPosting = errors.New("page carries no JobPosting")

// detail reads one posting page. ok=false with a nil error is the site's answer that the posting
// is gone (404/410); an error is the absence of an answer.
func (s karriereat) detail(ctx context.Context, p karriereatPosting) (Job, bool, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return Job{}, false, err
	}
	root, err := s.pages.GetHTML(ctx, p.url)
	if err != nil {
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	var ld karriereatLD
	if !ldJobPosting(root, &ld) {
		return Job{}, false, errKarriereatNoPosting
	}
	job, ok := karriereatJob(ld, p)
	return job, ok, nil
}

// karriereatJob maps a posting's JobPosting to a Job. ok is false for one with no title, employer
// or body.
func karriereatJob(ld karriereatLD, p karriereatPosting) (Job, bool) {
	title := strings.Join(strings.Fields(ld.Title), " ")
	company := strings.TrimSpace(ld.HiringOrganization.Name)
	body := sanitizeHTML(ld.Description)
	if title == "" || company == "" || strings.TrimSpace(body) == "" {
		return Job{}, false
	}
	j := Job{
		ExternalID:     p.id,
		URL:            p.url,
		Title:          title,
		Company:        company,
		Description:    body,
		PostedAt:       parseRFC3339OrDate(strings.TrimSpace(ld.DatePosted)),
		EmploymentType: schemaFixedTermEmploymentType(ld.EmploymentType),
	}
	j.Location, j.Countries = schemaPlacesLocation(ld.JobLocation)
	// The page words its salary "ab 2.789,33 € monatlich": the figure is a floor (the collective
	// agreement's minimum), so only the lower end is stated.
	ld.BaseSalary.apply(&j, "EUR")
	if j.SalaryMin != nil {
		j.SalaryMax = nil
	}
	return j, true
}
