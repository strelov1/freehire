package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	stdhtml "html"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/strelov1/freehire/internal/platform/flexjson"
	"golang.org/x/net/html"
	"golang.org/x/time/rate"
)

// clearancejobs adapts ClearanceJobs (clearancejobs.com), the US board for security-cleared
// defense and intelligence work, every discipline, whose postings each name their own employer.
// It is boardless and an aggregator: one crawl reads the whole live catalogue (~55k postings on
// 2026-11-22). Not to be confused with clearedjobs, a different site.
//
// The wildcard robots.txt rules disallow /jobs? (search queries), /*/apply$ and /ux-test/.
// The policy names the sitemap index. The
// posting pages, /jobs/<id>/<slug>, are open to a plain client and each carries a schema.org
// JobPosting with the whole body, so the crawl is the two job_postings sitemaps plus one page per
// posting the catalogue lacks. See providers/clearancejobs.md.
//
// The sitemaps carry no lastmod and keep every posting the site ever published (80k entries back
// to 2016), while a page of an expired posting still answers 200 with a validThrough in the past.
// Ids rise with datePosted and validThrough is a fixed 60 days after it, so the live postings are
// exactly the ids above a boundary: it is found by binary search over the sorted ids (about 17
// page reads), and nothing below it is read or refreshed, which is also what lets the unseen sweep
// close a posting once it expires.
type clearancejobs struct {
	http         clearancejobsHTTP
	limiter      waiter
	maxNewPerRun int
	retryBackoff time.Duration
	now          func() time.Time
}

// clearancejobsHTTP is the transport the adapter needs: sitemaps as a raw stream, pages as HTML.
type clearancejobsHTTP interface {
	StreamGetter
	HTMLGetter
}

const (
	clearancejobsBase         = "https://www.clearancejobs.com"
	clearancejobsSitemapIndex = clearancejobsBase + "/data/sitemaps/index.xml"
	// clearancejobsPostingNeedle selects the posting files (job_postings.xml, job_postings_2.xml)
	// among the ~19 landing-page files the index names; a later job_postings_3 joins by name.
	clearancejobsPostingNeedle = "/data/sitemaps/job_postings"
	// clearancejobsMaxSitemapBytes bounds one inflated sitemap file (the largest is ~10 MB).
	clearancejobsMaxSitemapBytes = 64 << 20

	clearancejobsRetries      = 2
	clearancejobsRetryBackoff = 2 * time.Second
	// clearancejobsRequestInterval paces every request of a run, three a second: a courtesy
	// ceiling toward a site that served six back-to-back reads without a refusal.
	clearancejobsRequestInterval     = 330 * time.Millisecond
	clearancejobsDetailWorkers       = 3
	clearancejobsBoundaryProbeWindow = 8
	// clearancejobsMaxNewPerRun bounds the pages one run reads: 4,000 at three a second is ~22
	// minutes of the scheduler's crawl budget, beside the sitemaps and the ~17 boundary probes. A
	// cold start reads the ~55k live backlog over ~14 runs, newest first; a day adds ~1.5k.
	clearancejobsMaxNewPerRun = 4000
)

// NewClearanceJobs builds the ClearanceJobs adapter over the given client.
func NewClearanceJobs(c clearancejobsHTTP) Source {
	return clearancejobs{
		http:         c,
		limiter:      rate.NewLimiter(rate.Every(clearancejobsRequestInterval), 1),
		maxNewPerRun: clearancejobsMaxNewPerRun,
		retryBackoff: clearancejobsRetryBackoff,
		now:          time.Now,
	}
}

func (clearancejobs) Provider() string { return "clearancejobs" }

// boardless: one crawl covers the whole catalogue.
func (clearancejobs) Boardless() {}

// Every posting names its own employer, so ClearanceJobs stays in the source facet and in the
// cross-source duplicate suppression set.
func (clearancejobs) Aggregator() {}

// clearancejobsPostingPath matches a posting URL, /jobs/<id>/<slug>. The sitemap also lists
// category, company and city landing pages under /jobs/, which carry no numeric id.
var clearancejobsPostingPath = regexp.MustCompile(`^/jobs/(\d+)/[^/]+/?$`)

type clearancejobsPosting struct {
	id  int64
	url string
}

func (p clearancejobsPosting) externalID() string { return strconv.FormatInt(p.id, 10) }

func clearancejobsParsePosting(raw string) (clearancejobsPosting, bool) {
	if !strings.HasPrefix(raw, clearancejobsBase+"/jobs/") {
		return clearancejobsPosting{}, false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return clearancejobsPosting{}, false
	}
	m := clearancejobsPostingPath.FindStringSubmatch(u.Path)
	if m == nil {
		return clearancejobsPosting{}, false
	}
	id, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil || id <= 0 {
		return clearancejobsPosting{}, false
	}
	return clearancejobsPosting{id: id, url: raw}, true
}

func (s clearancejobs) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew lists the sitemaps, finds the live boundary, refreshes the stored live postings and
// reads up to maxNewPerRun of the rest, newest first. An unreadable page retains its listing identity.
func (s clearancejobs) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	live, err := s.livePostings(ctx, postings)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	var unseen []clearancejobsPosting
	deferred := 0
	for _, p := range live {
		if seen(p.externalID()) {
			jobs = append(jobs, Job{ExternalID: p.externalID(), URL: p.url, SeenRefresh: true})
			continue
		}
		if len(unseen) >= s.maxNewPerRun {
			deferred++
			continue
		}
		unseen = append(unseen, p)
	}
	var answered atomic.Int64
	read := fetchDetails(unseen, clearancejobsDetailWorkers, func(p clearancejobsPosting) (Job, bool) {
		job, ok, err := s.detail(ctx, p)
		if err != nil {
			log.Printf("clearancejobs: posting %d failed; deferring: %v", p.id, err)
			return unreadableDetail(p.externalID(), p.url, ""), true
		}
		answered.Add(1)
		return job, ok
	})
	if len(unseen) > 0 && answered.Load() == 0 {
		return nil, fmt.Errorf("clearancejobs: requested %d posting pages and none answered", len(unseen))
	}
	if deferred > 0 {
		log.Printf("clearancejobs: %d new postings past this run's bound of %d, deferred to later runs", deferred, s.maxNewPerRun)
	}
	return append(jobs, read...), nil
}

// list reads every posting sitemap the index names and returns their postings, oldest id first.
// Any file failing fails the crawl: a short list would leave the postings it missed unrefreshed.
func (s clearancejobs) list(ctx context.Context) ([]clearancejobsPosting, error) {
	index, err := s.sitemap(ctx, clearancejobsSitemapIndex)
	if err != nil {
		return nil, fmt.Errorf("clearancejobs: sitemap index: %w", err)
	}
	ids := make(map[int64]bool)
	var out []clearancejobsPosting
	files := 0
	for _, sm := range index.Sitemaps {
		loc := strings.TrimSpace(sm.Loc)
		if !strings.HasPrefix(loc, clearancejobsBase+clearancejobsPostingNeedle) {
			continue
		}
		files++
		doc, err := s.sitemap(ctx, loc)
		if err != nil {
			return nil, fmt.Errorf("clearancejobs: sitemap %s: %w", sm.Loc, err)
		}
		for _, u := range doc.URLs {
			p, ok := clearancejobsParsePosting(strings.TrimSpace(u.Loc))
			if !ok || ids[p.id] {
				continue
			}
			ids[p.id] = true
			out = append(out, p)
		}
	}
	if files == 0 {
		return nil, fmt.Errorf("clearancejobs: sitemap index names no %s file", clearancejobsPostingNeedle)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("clearancejobs: the sitemaps list no posting")
	}
	sort.Slice(out, func(i, j int) bool { return out[i].id < out[j].id })
	return out, nil
}

// livePostings returns the postings above the live boundary, newest first. A catalogue with no
// live posting at all is a failed measurement, not an empty site.
func (s clearancejobs) livePostings(ctx context.Context, asc []clearancejobsPosting) ([]clearancejobsPosting, error) {
	var probeErr error
	first := sort.Search(len(asc), func(i int) bool {
		if probeErr != nil {
			return true
		}
		live, err := s.probeBoundary(ctx, asc[i:])
		if err != nil {
			probeErr = err
			return true
		}
		return live
	})
	if probeErr != nil {
		return nil, fmt.Errorf("clearancejobs: live boundary: %w", probeErr)
	}
	if first == len(asc) {
		return nil, fmt.Errorf("clearancejobs: none of the %d listed postings is live", len(asc))
	}
	out := make([]clearancejobsPosting, 0, len(asc)-first)
	for i := len(asc) - 1; i >= first; i-- {
		out = append(out, asc[i])
	}
	return out, nil
}

// sitemap fetches and decodes one sitemap file, plain or gzip.
// A removed URL has no date, so probe past it before choosing the live boundary.
func (s clearancejobs) probeBoundary(ctx context.Context, postings []clearancejobsPosting) (bool, error) {
	for _, p := range postings[:min(len(postings), clearancejobsBoundaryProbeWindow)] {
		ld, live, err := s.read(ctx, p)
		if err != nil {
			return false, err
		}
		if _, ok := clearancejobsTime(ld.ValidThrough); ok {
			return live, nil
		}
	}
	return false, fmt.Errorf("no dated posting near ID %d", postings[0].id)
}

func (s clearancejobs) sitemap(ctx context.Context, loc string) (sitemapDoc, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return sitemapDoc{}, err
	}
	var doc sitemapDoc
	err := s.http.GetStream(ctx, loc, "application/xml, application/x-gzip", func(r io.Reader) error {
		body, err := io.ReadAll(io.LimitReader(r, clearancejobsMaxSitemapBytes+1))
		if err != nil {
			return err
		}
		if len(body) > clearancejobsMaxSitemapBytes {
			return fmt.Errorf("sitemap exceeds %d bytes", clearancejobsMaxSitemapBytes)
		}
		doc, err = decodeSitemapBody(body)
		return err
	})
	return doc, err
}

// clearancejobsLD is the part of the posting page's schema.org JobPosting the adapter reads.
// validThrough is "2026-12-28T16:07:03+00:00Z", an offset followed by a stray Z.
type clearancejobsLD struct {
	Title                        string          `json:"title"`
	Description                  string          `json:"description"`
	SecurityClearanceRequirement string          `json:"securityClearanceRequirement"`
	DatePosted                   string          `json:"datePosted"`
	ValidThrough                 string          `json:"validThrough"`
	EmploymentType               string          `json:"employmentType"`
	Industry                     string          `json:"industry"`
	JobLocation                  json.RawMessage `json:"jobLocation"`
	HiringOrganization           struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	BaseSalary struct {
		Currency string `json:"currency"`
		Value    struct {
			UnitText string         `json:"unitText"`
			MinValue flexjson.Float `json:"minValue"`
			MaxValue flexjson.Float `json:"maxValue"`
		} `json:"value"`
	} `json:"baseSalary"`
}

// read fetches a posting page and reports whether the posting is still live. A 404 is an expired
// posting; an error is the absence of an answer.
func (s clearancejobs) read(ctx context.Context, p clearancejobsPosting) (clearancejobsLD, bool, error) {
	var root *html.Node
	err := retryWhile(ctx, clearancejobsRetries, s.retryBackoff, clearancejobsRefused, func() error {
		if err := s.limiter.Wait(ctx); err != nil {
			return err
		}
		var err error
		root, err = s.http.GetHTML(ctx, p.url)
		return err
	})
	if err != nil {
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return clearancejobsLD{}, false, nil
		}
		return clearancejobsLD{}, false, err
	}
	var ld clearancejobsLD
	if !ldJobPosting(root, &ld) {
		return clearancejobsLD{}, false, fmt.Errorf("page carries no JobPosting")
	}
	return ld, !s.expired(ld), nil
}

// expired reports a validThrough in the past. A missing or unreadable one reads as live.
func (s clearancejobs) expired(ld clearancejobsLD) bool {
	t, ok := clearancejobsTime(ld.ValidThrough)
	return ok && t.Before(s.now())
}

// clearancejobsTime reads the site's timestamps, dropping the stray Z after an explicit offset.
func clearancejobsTime(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if strings.HasSuffix(raw, "00:00Z") {
		raw = strings.TrimSuffix(raw, "Z")
	}
	t, err := time.Parse(time.RFC3339, raw)
	return t, err == nil
}

// detail reads one unseen posting. A removal or expiry returns no job.
// An unreadable page returns an error.
func (s clearancejobs) detail(ctx context.Context, p clearancejobsPosting) (Job, bool, error) {
	ld, live, err := s.read(ctx, p)
	if err != nil || !live {
		return Job{}, false, err
	}
	job, ok := clearancejobsJob(ld, p)
	if !ok {
		return Job{}, false, errors.New("posting lacks a title, employer or description")
	}
	return job, true, nil
}

func clearancejobsRefused(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && (se.Code == http.StatusForbidden || se.Code == http.StatusTooManyRequests || se.Code >= 500)
}

// clearancejobsJob maps a posting's JobPosting to a Job. ok is false for one with no title,
// employer or body.
func clearancejobsJob(ld clearancejobsLD, p clearancejobsPosting) (Job, bool) {
	// The page title carries the site's "Jobs" suffix ("Mission Manager Jobs").
	title := strings.TrimSuffix(collapseSpace(ld.Title), " Jobs")
	company := collapseSpace(ld.HiringOrganization.Name)
	body := sanitizeHTML(ld.Description)
	if title == "" || company == "" || strings.TrimSpace(body) == "" {
		return Job{}, false
	}
	// The level is a structured field and is rarely in the text; a labelled line is the form the
	// clearance dictionary reads as an assertion.
	if level := collapseSpace(ld.SecurityClearanceRequirement); level != "" && !strings.EqualFold(level, "unspecified") {
		body += "\n<p>Security Clearance: " + stdhtml.EscapeString(level) + "</p>"
	}
	industry := collapseSpace(ld.Industry)
	j := Job{
		ExternalID:        p.externalID(),
		URL:               p.url,
		Title:             title,
		Company:           company,
		Description:       body,
		Location:          clearancejobsPlaces(ld.JobLocation),
		FallbackCountries: []string{"us"},
		PostedAt:          NotFuture(parseRFC3339OrDate(ld.DatePosted)),
		// The category is the employer's own pick from the site's list, whose IT ones are all
		// named "IT - <discipline>".
		IsTechHint: strings.HasPrefix(industry, "IT - "),
	}
	if deadline, ok := clearancejobsTime(ld.ValidThrough); ok {
		j.ApplicationDeadline = &deadline
	}
	switch strings.ToLower(strings.TrimSpace(ld.EmploymentType)) {
	case "full-time", "full time":
		j.EmploymentType = "full_time"
	case "part-time", "part time":
		j.EmploymentType = "part_time"
	case "contract", "contractor", "temporary":
		j.EmploymentType = "contract"
	case "intern", "internship":
		j.EmploymentType = "internship"
	}
	sal := ld.BaseSalary
	period := map[string]string{"YEARLY": "year", "MONTHLY": "month", "HOURLY": "hour"}[strings.ToUpper(sal.Value.UnitText)]
	if cur := strings.TrimSpace(sal.Currency); cur != "" && period != "" && sal.Value.MinValue > 0 && sal.Value.MaxValue >= sal.Value.MinValue {
		lo, hi := int(sal.Value.MinValue), int(sal.Value.MaxValue)
		j.SalaryMin, j.SalaryMax = &lo, &hi
		j.SalaryCurrency, j.SalaryPeriod = strings.ToUpper(cur), period
	}
	return j, true
}

// clearancejobsPlaces joins a posting's distinct places as "City, ST"; a country other than the
// United States is appended so the dictionary can place an overseas posting.
func clearancejobsPlaces(raw json.RawMessage) string {
	type place struct {
		Address struct {
			Locality string `json:"addressLocality"`
			Region   string `json:"addressRegion"`
			Country  string `json:"addressCountry"`
		} `json:"address"`
	}
	var many []place
	if err := json.Unmarshal(raw, &many); err != nil {
		var one place
		if json.Unmarshal(raw, &one) != nil {
			return ""
		}
		many = []place{one}
	}
	return distinctJoin(many, "; ", func(pl place) string {
		country := collapseSpace(pl.Address.Country)
		if strings.EqualFold(country, "United States") {
			country = ""
		}
		return joinNonEmpty(collapseSpace(pl.Address.Locality), collapseSpace(pl.Address.Region), country)
	})
}
