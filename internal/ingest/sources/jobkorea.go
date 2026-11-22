package sources

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
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

// jobkorea adapts JobKorea (jobkorea.co.kr), a large South Korean general job board, every
// trade, whose postings each name their own employer. It is boardless and an aggregator: one
// crawl covers the whole catalogue (102,455 postings in the sitemaps on 2026-11-22).
//
// The posting set is the site's own sitemaps (robots.txt names them and disallows none of the
// routes read here): the index names 21 files under /content/sitemapindex/agi/ of up to 5,000
// /Recruit/GI_Read/<id> URLs each. They carry nothing else (every lastmod is the generation
// time), so every fact is bought per posting, in two requests: the posting page, whose
// schema.org JobPosting states title, employer, dates, employment type, address and, when the
// poster gave one, a salary, but only a one-sentence summary as its description; and the frame
// the page embeds the poster's own body in (/Recruit/GI_Read_Comt_Ifrm?Gno=<id>). The adapter
// hydrates only postings the catalogue lacks, newest id first, and refreshes the rest by
// identity. See providers/jobkorea.md.
type jobkorea struct {
	http         jobkoreaHTTP
	limiter      waiter
	maxNewPerRun int
	// retryBackoff is the first pause before a refused page is asked again; it doubles.
	retryBackoff time.Duration
	// now is the clock the expiry check reads.
	now func() time.Time
}

// jobkoreaHTTP is the transport the adapter needs: the sitemaps as a raw stream and the pages.
type jobkoreaHTTP interface {
	StreamGetter
	HTMLGetter
}

const (
	jobkoreaBase         = "https://www.jobkorea.co.kr"
	jobkoreaSitemapIndex = jobkoreaBase + "/content/sitemapindex.xml"
	// The index names ~270 files of company, salary, Q&A and search landing pages beside the
	// posting ones; the posting files all live under this directory.
	jobkoreaPostingNeedle = "/sitemapindex/agi/"
	jobkoreaPostingURL    = jobkoreaBase + "/Recruit/GI_Read/%d"
	// jobkoreaBodyURL is the frame the poster's body is rendered in. It is not disallowed
	// (robots.txt names /Recruit/JobList/SpaceClick and nothing else under /Recruit/).
	jobkoreaBodyURL = jobkoreaBase + "/Recruit/GI_Read_Comt_Ifrm?Gno=%d"
	// jobkoreaMaxSitemapBytes bounds one sitemap file (5,000 URLs are ~700 KB).
	jobkoreaMaxSitemapBytes = 16 << 20

	// jobkoreaSweepGrace absorbs a sitemap generated before a posting reached it; a posting
	// lives about a month.
	jobkoreaSweepGrace = 3 * 24 * time.Hour

	// A refused or failed page is asked again up to jobkoreaRetries times, doubling the wait.
	jobkoreaRetries      = 2
	jobkoreaRetryBackoff = 2 * time.Second

	// jobkoreaRequestInterval paces every request of a run, sitemap, page and frame alike, at two
	// a second — a courtesy ceiling toward a large site, not a measured limit.
	jobkoreaRequestInterval = 500 * time.Millisecond
	jobkoreaDetailWorkers   = 4
	// jobkoreaMaxNewPerRun bounds how many unseen postings one run reads: 1,200 postings are 2,400
	// requests, 20 minutes at the pace above, inside the scheduler's default 2,400 s crawl budget
	// beside the 21 sitemap files and the writes after the fetch.
	jobkoreaMaxNewPerRun = 1200
)

// jobkoreaZone is the time zone the site states its deadlines in, which carry no offset.
var jobkoreaZone = time.FixedZone("KST", 9*60*60)

// NewJobKorea builds the JobKorea adapter over the given client.
func NewJobKorea(c jobkoreaHTTP) Source {
	return jobkorea{
		http:         c,
		limiter:      rate.NewLimiter(rate.Every(jobkoreaRequestInterval), 1),
		maxNewPerRun: jobkoreaMaxNewPerRun,
		retryBackoff: jobkoreaRetryBackoff,
		now:          time.Now,
	}
}

func (jobkorea) Provider() string { return "jobkorea" }

// boardless: one crawl covers the whole national catalogue.
func (jobkorea) Boardless() {}

// Every posting names its own employer, so JobKorea stays in the source facet and in the
// cross-source duplicate suppression set.
func (jobkorea) Aggregator() {}

func (jobkorea) SweepGrace() time.Duration { return jobkoreaSweepGrace }

// jobkoreaPosting is one sitemap entry.
type jobkoreaPosting struct {
	id  int64
	url string
}

func (p jobkoreaPosting) externalID() string { return strconv.FormatInt(p.id, 10) }

var jobkoreaPostingPath = regexp.MustCompile(`^https://www\.jobkorea\.co\.kr/Recruit/GI_Read/(\d+)/?$`)

// jobkoreaParsePosting reads a sitemap URL, or ok=false when it is not a posting URL.
func jobkoreaParsePosting(raw string) (jobkoreaPosting, bool) {
	m := jobkoreaPostingPath.FindStringSubmatch(strings.TrimSpace(raw))
	if m == nil {
		return jobkoreaPosting{}, false
	}
	id, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil || id <= 0 {
		return jobkoreaPosting{}, false
	}
	return jobkoreaPosting{id: id, url: fmt.Sprintf(jobkoreaPostingURL, id)}, true
}

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s jobkorea) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew lists every posting the sitemaps name, refreshes the ones already stored, and reads
// up to maxNewPerRun of the rest, newest id first. A posting whose page cannot be read is not
// yielded; a later run tries again.
func (s jobkorea) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	var unseen []jobkoreaPosting
	deferred := 0
	for _, p := range postings {
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
	read := fetchDetails(unseen, jobkoreaDetailWorkers, func(p jobkoreaPosting) (Job, bool) {
		job, ok, err := s.detail(ctx, p)
		if err != nil {
			log.Printf("jobkorea: posting %d failed; deferring: %v", p.id, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, ok
	})
	// A crawl that read no page at all is broken, not quiet. "Answered" counts a posting the
	// site reported gone or expired.
	if len(unseen) > 0 && answered.Load() == 0 {
		return nil, fmt.Errorf("jobkorea: requested %d posting pages and none answered", len(unseen))
	}
	if deferred > 0 {
		log.Printf("jobkorea: %d new postings past this run's bound of %d, deferred to later runs", deferred, s.maxNewPerRun)
	}
	return append(jobs, read...), nil
}

// list reads every posting sitemap the index names and returns their postings, newest id first.
// Any file failing fails the crawl: a short list would leave the postings it missed
// un-refreshed, and the index only names files that exist.
func (s jobkorea) list(ctx context.Context) ([]jobkoreaPosting, error) {
	index, err := s.sitemap(ctx, jobkoreaSitemapIndex)
	if err != nil {
		return nil, fmt.Errorf("jobkorea: sitemap index: %w", err)
	}
	ids := make(map[int64]bool)
	var out []jobkoreaPosting
	files := 0
	for _, sm := range index.Sitemaps {
		if !strings.Contains(sm.Loc, jobkoreaPostingNeedle) {
			continue
		}
		files++
		doc, err := s.sitemap(ctx, strings.TrimSpace(sm.Loc))
		if err != nil {
			return nil, fmt.Errorf("jobkorea: sitemap %s: %w", sm.Loc, err)
		}
		for _, u := range doc.URLs {
			p, ok := jobkoreaParsePosting(u.Loc)
			if !ok || ids[p.id] {
				continue
			}
			ids[p.id] = true
			out = append(out, p)
		}
	}
	if files == 0 {
		return nil, fmt.Errorf("jobkorea: sitemap index names no %s file", jobkoreaPostingNeedle)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("jobkorea: the sitemaps list no posting")
	}
	sort.Slice(out, func(i, j int) bool { return out[i].id > out[j].id })
	return out, nil
}

// sitemap fetches and decodes one sitemap file. The body is read to EOF, so a truncated file
// fails instead of reading as a short list.
func (s jobkorea) sitemap(ctx context.Context, loc string) (sitemapDoc, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return sitemapDoc{}, err
	}
	var doc sitemapDoc
	err := s.http.GetStream(ctx, loc, "application/xml", func(r io.Reader) error {
		body, err := io.ReadAll(io.LimitReader(r, jobkoreaMaxSitemapBytes+1))
		if err != nil {
			return err
		}
		if len(body) > jobkoreaMaxSitemapBytes {
			return fmt.Errorf("sitemap exceeds %d bytes", jobkoreaMaxSitemapBytes)
		}
		doc, err = decodeSitemapBody(body)
		return err
	})
	return doc, err
}

// jobkoreaLD is the part of the posting page's schema.org JobPosting the adapter reads.
// validThrough is "2026-12-10T23:59" (no offset) or a bare date.
type jobkoreaLD struct {
	Title              string `json:"title"`
	Description        string `json:"description"`
	DatePosted         string `json:"datePosted"`
	ValidThrough       string `json:"validThrough"`
	EmploymentType     string `json:"employmentType"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	JobLocation struct {
		Address struct {
			Street string `json:"streetAddress"`
		} `json:"address"`
	} `json:"jobLocation"`
	BaseSalary struct {
		Currency string `json:"currency"`
		Value    struct {
			UnitText string         `json:"unitText"`
			MinValue flexjson.Float `json:"minValue"`
			MaxValue flexjson.Float `json:"maxValue"`
		} `json:"value"`
	} `json:"baseSalary"`
}

// detail reads one posting: its page and then its body frame. ok=false with a nil error is the
// site's answer that the posting is gone (404/410), expired, or carries no usable posting; an
// error is the absence of an answer.
func (s jobkorea) detail(ctx context.Context, p jobkoreaPosting) (Job, bool, error) {
	page, err := s.get(ctx, p.url)
	if err != nil {
		return Job{}, false, jobkoreaGone(err)
	}
	var ld jobkoreaLD
	if !ldJobPosting(page, &ld) {
		return Job{}, false, fmt.Errorf("page carries no JobPosting")
	}
	job, ok := jobkoreaJob(ld, p, s.now())
	if !ok {
		return Job{}, false, nil
	}
	frame, err := s.get(ctx, fmt.Sprintf(jobkoreaBodyURL, p.id))
	if err != nil {
		// Without its frame the posting would be stored with a one-sentence body for good, so
		// a frame that could not be read defers the posting; one the site says is gone does not.
		if jobkoreaGone(err) == nil {
			return job, true, nil
		}
		return Job{}, false, err
	}
	content := firstByID(frame, "detail-content")
	if content == nil {
		return Job{}, false, errors.New("body frame lacks #detail-content")
	}
	// An image-only posting keeps the JobPosting summary.
	if body := sanitizeHTML(innerHTML(content)); strings.TrimSpace(textFromHTML(body)) != "" {
		job.Description = body
	}
	return job, true, nil
}

// get reads one page, asking again after a refusal or a server failure.
func (s jobkorea) get(ctx context.Context, u string) (*html.Node, error) {
	var root *html.Node
	err := retryWhile(ctx, jobkoreaRetries, s.retryBackoff, jobkoreaRefused, func() error {
		if err := s.limiter.Wait(ctx); err != nil {
			return err
		}
		var err error
		root, err = s.http.GetHTML(ctx, u)
		return err
	})
	return root, err
}

func jobkoreaRefused(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && (se.Code == http.StatusForbidden || se.Code == http.StatusTooManyRequests || se.Code >= 500)
}

// jobkoreaGone returns nil when err is the site saying the page does not exist, else err.
func jobkoreaGone(err error) error {
	var se *StatusError
	if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
		return nil
	}
	return err
}

// jobkoreaDeadline reads validThrough, a bare date or a date and minute in KST.
func jobkoreaDeadline(s string) *time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range []string{"2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, jobkoreaZone); err == nil {
			return &t
		}
	}
	return nil
}

// jobkoreaJob maps a posting's JobPosting to a Job. ok is false for one with no title or
// employer, or whose deadline has passed (the site keeps serving a closed posting's page).
// The description is the JobPosting's one-sentence summary until the caller replaces it with
// the poster's body.
func jobkoreaJob(ld jobkoreaLD, p jobkoreaPosting, now time.Time) (Job, bool) {
	title := collapseSpace(ld.Title)
	company := collapseSpace(ld.HiringOrganization.Name)
	if title == "" || company == "" {
		return Job{}, false
	}
	deadline := jobkoreaDeadline(ld.ValidThrough)
	if deadline != nil && deadline.Before(now) {
		return Job{}, false
	}
	j := Job{
		ExternalID:        p.externalID(),
		URL:               p.url,
		Title:             title,
		Company:           company,
		Description:       sanitizeHTML(ld.Description),
		Location:          collapseSpace(ld.JobLocation.Address.Street),
		FallbackCountries: []string{"kr"},
		PostedAt:          parseRFC3339OrDate(strings.TrimSpace(ld.DatePosted)),
	}
	if deadline != nil {
		d := deadline.UTC()
		j.ApplicationDeadline = &d
	}
	switch strings.ToUpper(strings.TrimSpace(ld.EmploymentType)) {
	case "FULL_TIME":
		j.EmploymentType = "full_time"
	case "PART_TIME":
		j.EmploymentType = "part_time"
	case "CONTRACTOR", "TEMPORARY":
		j.EmploymentType = "contract"
	case "INTERN":
		j.EmploymentType = "internship"
	}
	sal := ld.BaseSalary
	period := map[string]string{"YEAR": "year", "MONTH": "month", "DAY": "day", "HOUR": "hour"}[strings.ToUpper(sal.Value.UnitText)]
	if cur := strings.TrimSpace(sal.Currency); cur != "" && period != "" &&
		sal.Value.MinValue > 0 && sal.Value.MaxValue >= sal.Value.MinValue {
		lo, hi := int(sal.Value.MinValue), int(sal.Value.MaxValue)
		j.SalaryMin, j.SalaryMax = &lo, &hi
		j.SalaryCurrency = strings.ToUpper(cur)
		j.SalaryPeriod = period
	}
	return j, true
}
