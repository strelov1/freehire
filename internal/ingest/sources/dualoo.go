package sources

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// dualoo adapts Dualoo (jobs.dualoo.com), a Swiss ATS for SMEs, municipalities and care homes,
// most of them in German-speaking Switzerland. Every customer's postings live on the one host,
// either under its careers portal (/portal/<code>/<uuid>/detail) or as a bare link
// (/link/<uuid>/detail), and robots.txt advertises a sitemap index with one sitemap per customer
// (472 on 2026-10-29, ~8.9k postings). The adapter is boardless: the posting set is that index,
// and the employer is read per posting.
//
// robots.txt disallows every apply form and a list of ~550 portals; the sitemap index names
// none of those portals, so walking it reads only what robots allows.
//
// The sitemaps carry each posting's URL and lastmod, so every fact is bought with the posting
// page's schema.org JobPosting. The adapter hydrates only postings the catalogue lacks, newest
// first plus a moving backlog window (hydrationLanes), and refreshes the rest by identity.
//
// It is marked an aggregator because it serves many employers under one provider, which keeps it
// in the source facet. It does NOT claim fullCatalog: nothing states how many postings the
// sitemaps should hold.
type dualoo struct {
	sitemap        XMLGetter
	pages          HTMLGetter
	limiter        waiter
	fresh, backlog int
	deadlineMargin time.Duration
	now            func() time.Time
}

const (
	dualooHost         = "jobs.dualoo.com"
	dualooSitemapIndex = "https://" + dualooHost + "/sitemap_index.xml"

	// dualooRequestInterval paces every request — the ~470 customer sitemaps and the posting
	// pages alike — at one a second, so a run's listing costs about eight minutes.
	dualooRequestInterval = time.Second
	dualooDetailWorkers   = 2
	// dualooFreshPerRun and dualooBacklogPerRun bound one run's page reads to twenty minutes,
	// which with the listing fits the default 50-minute run. The ~8.9k postings of 2026-10-29
	// take the first runs in slices.
	dualooFreshPerRun    = 900
	dualooBacklogPerRun  = 300
	dualooDeadlineMargin = 5 * time.Minute
)

// NewDualoo builds the adapter: the sitemaps are read over sitemap, the posting pages over pages.
func NewDualoo(sitemap XMLGetter, pages HTMLGetter) Source {
	return dualoo{
		sitemap:        sitemap,
		pages:          pages,
		limiter:        rate.NewLimiter(rate.Every(dualooRequestInterval), 1),
		fresh:          dualooFreshPerRun,
		backlog:        dualooBacklogPerRun,
		deadlineMargin: dualooDeadlineMargin,
		now:            time.Now,
	}
}

func (dualoo) Provider() string { return "dualoo" }

// The sitemap index is the whole crawl; no per-customer board is needed to reach a posting.
func (dualoo) boardless() {}

// Many employers publish through it, each named per posting.
func (dualoo) aggregator() {}

// dualooPostingPath matches a posting's path in either of its two forms and captures its UUID.
var dualooPostingPath = regexp.MustCompile(`^/(?:portal/[a-z0-9-]+|link)/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})/detail$`)

// dualooTestSitemap matches the customer sitemaps of the vendor's own test tenants, which publish
// to the live index: "michele-testing-ag" (39 duplicated postings on 2026-10-29),
// "test-language-prod", and smoke tests named by date ("smoketest-9-3-226-dualoo-ag",
// "25-1-26-dualoo-ag"). Dualoo AG's own sitemap ("dualoo-ag") is a real employer and is kept, and
// "test" must be a whole slug word, so an employer such as Testo is not mistaken for one.
var dualooTestSitemap = regexp.MustCompile(`^/sitemap_(?:(?:.*-)?(?:smoketest|testing|test)(?:-.*)?|[0-9-]+-dualoo-ag)\.xml$`)

// dualooPosting is one sitemap entry.
type dualooPosting struct {
	id      string
	url     string
	lastmod string
	portal  bool
}

func dualooParsePosting(raw, lastmod string) (dualooPosting, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host != dualooHost {
		return dualooPosting{}, false
	}
	m := dualooPostingPath.FindStringSubmatch(u.Path)
	if m == nil {
		return dualooPosting{}, false
	}
	return dualooPosting{
		id:      m[1],
		url:     u.String(),
		lastmod: strings.TrimSpace(lastmod),
		portal:  strings.HasPrefix(u.Path, "/portal/"),
	}, true
}

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s dualoo) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew lists the sitemaps, refreshes the postings already stored, and reads a bounded slice
// of the rest. A posting whose page cannot be read is not yielded; a later run tries again.
func (s dualoo) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	var unseen []dualooPosting
	for _, p := range postings {
		if seen(p.id) {
			jobs = append(jobs, Job{ExternalID: p.id, URL: p.url, SeenRefresh: true})
			continue
		}
		unseen = append(unseen, p)
	}
	picked := hydrationLanes(unseen, s.fresh, s.backlog, s.now())
	var answered, outOfTime atomic.Int64
	read := fetchDetails(picked, dualooDetailWorkers, func(p dualooPosting) (Job, bool) {
		if d, ok := ctx.Deadline(); ok && time.Until(d) < s.deadlineMargin {
			outOfTime.Add(1)
			return Job{}, false
		}
		job, ok, err := s.detail(ctx, p)
		if err != nil {
			log.Printf("dualoo: posting %s failed; deferring: %v", p.id, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, ok
	})
	if asked := int64(len(picked)) - outOfTime.Load(); asked > 0 && answered.Load() == 0 {
		return nil, fmt.Errorf("dualoo: requested %d posting pages and none answered", asked)
	}
	if n := outOfTime.Load(); n > 0 {
		log.Printf("dualoo: %d new postings left unread as the run neared its deadline, deferred to later runs", n)
	}
	if n := len(unseen) - len(picked); n > 0 {
		log.Printf("dualoo: %d new postings past this run's bound of %d, deferred to later runs", n, s.fresh+s.backlog)
	}
	return append(jobs, read...), nil
}

// list walks the sitemap index and every customer sitemap it names, newest lastmod first. The
// index failing fails the crawl. A customer sitemap failing is skipped — that customer's
// postings simply go unlisted this run, which the company-scoped sweep does not read as a
// closure — unless every one of them failed, which is no listing at all.
func (s dualoo) list(ctx context.Context) ([]dualooPosting, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	index, err := getSitemap(ctx, s.sitemap, dualooSitemapIndex)
	if err != nil {
		return nil, fmt.Errorf("dualoo: sitemap index: %w", err)
	}
	byID := map[string]int{}
	var out []dualooPosting
	var read, failed int
	for _, sm := range index.Sitemaps {
		loc := resolveSitemapLoc(dualooSitemapIndex, sm.Loc)
		if u, err := url.Parse(loc); err != nil || u.Host != dualooHost || dualooTestSitemap.MatchString(u.Path) {
			continue
		}
		if err := s.limiter.Wait(ctx); err != nil {
			return nil, err
		}
		doc, err := getSitemap(ctx, s.sitemap, loc)
		if err != nil {
			failed++
			log.Printf("dualoo: sitemap %s: %v", loc, err)
			continue
		}
		read++
		for _, u := range doc.URLs {
			p, ok := dualooParsePosting(u.Loc, u.LastMod)
			if !ok {
				continue
			}
			// One posting can be listed both under its portal and as a bare link; the
			// portal URL is the one its employer's careers page links to.
			if i, dup := byID[p.id]; dup {
				if p.portal && !out[i].portal {
					out[i] = p
				}
				continue
			}
			byID[p.id] = len(out)
			out = append(out, p)
		}
	}
	if read == 0 {
		return nil, fmt.Errorf("dualoo: none of %d customer sitemaps could be read", failed)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("dualoo: sitemaps list no posting")
	}
	// lastmod is an ISO date, so the string order is the date order.
	slices.SortStableFunc(out, func(a, b dualooPosting) int { return cmp.Compare(b.lastmod, a.lastmod) })
	return out, nil
}

// dualooLD is the part of the posting page's schema.org JobPosting the adapter reads. Each body
// block carries its own heading ("Ihre Aufgaben", "Ihr Profil", "Wir bieten").
type dualooLD struct {
	Title              string          `json:"title"`
	Description        string          `json:"description"`
	Responsibilities   string          `json:"responsibilities"`
	Skills             string          `json:"skills"`
	Qualifications     string          `json:"qualifications"`
	JobBenefits        string          `json:"jobBenefits"`
	DatePosted         string          `json:"datePosted"`
	EmploymentType     json.RawMessage `json:"employmentType"`
	JobLocationType    string          `json:"jobLocationType"`
	JobLocation        schemaPlaces    `json:"jobLocation"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
}

// errDualooNoPosting marks a page that answered 200 without a JobPosting. The site answers a
// removed posting with a 404, so this is not a "gone" answer and counts as no answer.
var errDualooNoPosting = errors.New("page carries no JobPosting")

// detail reads one posting page. ok=false with a nil error is the site's answer that the posting
// is gone (404/410); an error is the absence of an answer.
func (s dualoo) detail(ctx context.Context, p dualooPosting) (Job, bool, error) {
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
	var ld dualooLD
	if !ldJobPosting(root, &ld) {
		return Job{}, false, errDualooNoPosting
	}
	job, ok := dualooJob(ld, p)
	return job, ok, nil
}

// dualooJob maps a posting's JobPosting to a Job. ok is false for one with no title, employer or
// body.
func dualooJob(ld dualooLD, p dualooPosting) (Job, bool) {
	title := strings.Join(strings.Fields(html.UnescapeString(ld.Title)), " ")
	company := strings.TrimSpace(html.UnescapeString(ld.HiringOrganization.Name))
	body := sanitizeHTML(ld.Description + ld.Responsibilities + ld.Skills + ld.Qualifications + ld.JobBenefits)
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
	if strings.EqualFold(strings.TrimSpace(ld.JobLocationType), "TELECOMMUTE") {
		j.WorkMode, j.Remote = "remote", true
	}
	return j, true
}
