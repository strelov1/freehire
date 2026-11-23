package sources

import (
	"context"
	"errors"
	"fmt"
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
	"golang.org/x/time/rate"
)

// naukrigulf adapts Naukrigulf (naukrigulf.com), Info Edge's job board for the Gulf: the UAE,
// Saudi Arabia, Qatar, Oman, Bahrain, Kuwait and the wider region, every trade. It is boardless
// and an aggregator: one crawl covers the whole catalogue (~41.5k postings on 2026-11-22), the
// employer read per posting.
//
// The site publishes no posting sitemap (its sitemaps list search landing pages), and its pages
// are a client-rendered shell. The data comes from the JSON API the web app itself calls,
// /spapi/jobapi/search for the listing and /spapi/jobs/<id> for a posting; robots.txt disallows
// neither. The listing is walked by Offset, newest first, and carries the title, employer,
// place, date and experience but only a one-line summary, so the full body is bought from the
// detail API for postings the catalogue lacks. Akamai refuses a plain client, so production
// reads through the Scrapling tier over the proxy (scraplingtier.go). See providers/naukrigulf.md.
type naukrigulf struct {
	http         HeaderJSONGetter
	limiter      waiter
	maxNewPerRun int
	// retryBackoff is the first pause before a refused request is asked again; it doubles.
	retryBackoff time.Duration
}

const (
	naukrigulfBase      = "https://www.naukrigulf.com"
	naukrigulfSearchURL = naukrigulfBase + "/spapi/jobapi/search?seo=1&showBellyFilters=false&Limit=%d&Offset=%d"
	// naukrigulfDetailURL carries the web app's own query; xz is the search-session tag it echoes.
	naukrigulfDetailURL = naukrigulfBase + "/spapi/jobs/%s?seo=1&xz=1_0_5&source=srp"

	// naukrigulfPageSize is the largest page the API accepts: it answers 400 "limit must be
	// lesser than 50", and the web app itself asks for 30.
	naukrigulfPageSize = 49
	// naukrigulfMaxOffset is the API's own ceiling ("Offset must be lesser than 45000"). The
	// catalogue held 41,499 postings, so the walk normally ends on an empty page first; past the
	// ceiling the tail is simply not listed, and a catalogue that grows beyond it needs the
	// listing sliced by country.
	naukrigulfMaxOffset = 45000

	// naukrigulfConfidential is the employer name the site files an anonymous posting under for
	// every such posting at once, a quarter of the catalogue. It is no employer, so filing the
	// postings under it would build one bogus company out of all of them.
	naukrigulfConfidential = "confidential company"

	// naukrigulfSweepGrace absorbs a walk that races the catalogue: the listing is paged by
	// offset while postings are added at the front, so a posting can slip between two pages.
	naukrigulfSweepGrace = 72 * time.Hour

	// A refused request (Akamai's 403, a rate limit or a server failure) is asked again up to
	// naukrigulfRetries times: the proxy rotates exits and a refusal is one bad exit, not the
	// site's verdict on the request.
	naukrigulfRetries      = 4
	naukrigulfRetryBackoff = time.Second

	// naukrigulfRequestInterval paces every request of a run, listing page and detail alike, at
	// two a second: a courtesy ceiling toward a large site, not a measured limit. A request takes
	// ~1 s through the proxy, so the walk of ~850 pages is bounded by latency, not by this.
	naukrigulfRequestInterval = 500 * time.Millisecond
	naukrigulfDetailWorkers   = 4
	// naukrigulfMaxNewPerRun bounds how many unseen postings one run reads. The listing walk
	// takes ~15 minutes of the scheduler's 2,400 s crawl budget and a run that meets its deadline
	// stores nothing, so the details get ~12 minutes: 1,500 at two a second. The site adds ~1,000
	// postings a day, so the cold backlog drains over about a day of runs, newest first.
	naukrigulfMaxNewPerRun = 1500
)

// NewNaukrigulf builds the Naukrigulf adapter over the given client.
func NewNaukrigulf(c HeaderJSONGetter) Source {
	return naukrigulf{
		http:         c,
		limiter:      rate.NewLimiter(rate.Every(naukrigulfRequestInterval), 2),
		maxNewPerRun: naukrigulfMaxNewPerRun,
		retryBackoff: naukrigulfRetryBackoff,
	}
}

func (naukrigulf) Provider() string { return "naukrigulf" }

// boardless: one crawl covers the whole regional catalogue.
func (naukrigulf) Boardless() {}

// Every posting names its own employer, so naukrigulf stays in the source facet and in the
// cross-source duplicate suppression set.
func (naukrigulf) Aggregator() {}

func (naukrigulf) SweepGrace() time.Duration { return naukrigulfSweepGrace }

// The public web app identifies its API requests with these headers. The version also enables pagination.
var naukrigulfHeaders = map[string]string{
	"version":  "v1",
	"appid":    "205",
	"systemid": "2323",
}

// naukrigulfRow is one listed posting, reduced to what the adapter reads. The listing's numbers
// are strings ("LatestPostedDate": "1795366168").
type naukrigulfRow struct {
	JobID       string `json:"JobId"`
	Designation string `json:"Designation"`
	Location    string `json:"Location"`
	JdURL       string `json:"JdURL"`
	Company     struct {
		Name string `json:"Name"`
	} `json:"Company"`
	Experience struct {
		Min flexjson.Int `json:"Min"`
	} `json:"Experience"`
	LatestPostedDate flexjson.Int64 `json:"LatestPostedDate"`
}

type naukrigulfListing struct {
	Jobs []struct {
		Job naukrigulfRow `json:"Job"`
	} `json:"Jobs"`
}

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s naukrigulf) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew walks the whole listing, refreshes the postings already stored by identity and reads
// the body of up to maxNewPerRun of the rest, newest first. A posting whose detail cannot be
// read is not yielded; a later run tries again.
func (s naukrigulf) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	rows, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var jobs, unseen []Job
	deferred := 0
	for _, r := range rows {
		base, ok := naukrigulfBaseJob(r)
		if !ok {
			continue
		}
		if seen(base.ExternalID) {
			base.SeenRefresh = true
			jobs = append(jobs, base)
			continue
		}
		if len(unseen) >= s.maxNewPerRun {
			deferred++
			continue
		}
		unseen = append(unseen, base)
	}
	var answered atomic.Int64
	jobs = append(jobs, fetchDetails(unseen, naukrigulfDetailWorkers, func(base Job) (Job, bool) {
		job, ok, err := s.withDetail(ctx, base)
		if err != nil {
			log.Printf("naukrigulf: posting %s failed; deferring: %v", base.ExternalID, err)
			return unreadableDetail(base.ExternalID, base.URL, base.Company), true
		}
		answered.Add(1)
		return job, ok
	})...)
	// A crawl that asked for details and got no answer to any is broken, not quiet. "Answered"
	// counts a posting the site reported gone.
	if len(unseen) > 0 && answered.Load() == 0 {
		return nil, fmt.Errorf("naukrigulf: requested %d posting details and none answered", len(unseen))
	}
	if deferred > 0 {
		log.Printf("naukrigulf: %d new postings past this run's bound of %d, deferred to later runs", deferred, s.maxNewPerRun)
	}
	return jobs, nil
}

// list walks the search by Offset to the first empty page and returns every distinct posting,
// newest first. Any page failing fails the crawl: a short list would leave the postings it
// missed un-refreshed and read as a shrunken catalogue.
func (s naukrigulf) list(ctx context.Context) ([]naukrigulfRow, error) {
	var out []naukrigulfRow
	ids := make(map[string]bool)
	for offset := 0; offset < naukrigulfMaxOffset; offset += naukrigulfPageSize {
		var page naukrigulfListing
		u := fmt.Sprintf(naukrigulfSearchURL, naukrigulfPageSize, offset)
		if err := s.get(ctx, u, &page); err != nil {
			return nil, fmt.Errorf("naukrigulf: listing offset %d: %w", offset, err)
		}
		if len(page.Jobs) == 0 {
			break
		}
		for _, j := range page.Jobs {
			// An offset walk over a catalogue that grows at its front repeats a posting at a page
			// boundary now and then.
			if j.Job.JobID == "" || ids[j.Job.JobID] {
				continue
			}
			ids[j.Job.JobID] = true
			out = append(out, j.Job)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("naukrigulf: the listing holds no posting")
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].LatestPostedDate > out[j].LatestPostedDate })
	return out, nil
}

// get reads one API answer through the pacer, asking again while the request is refused.
func (s naukrigulf) get(ctx context.Context, u string, v any) error {
	return retryWhile(ctx, naukrigulfRetries, s.retryBackoff, naukrigulfRefused, func() error {
		if err := s.limiter.Wait(ctx); err != nil {
			return err
		}
		return s.http.GetJSONWithHeaders(ctx, u, naukrigulfHeaders, v)
	})
}

// naukrigulfRefused reports an answer worth asking again: Akamai's 403, a rate limit or a server
// failure, none of which the Scrapling client retries itself.
func naukrigulfRefused(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && (se.Code == http.StatusForbidden || se.Code == http.StatusTooManyRequests || se.Code >= 500)
}

// naukrigulfBaseJob maps a listed posting to a body-less Job. ok is false for one with no id,
// title or employer, and for an anonymous one, which has no employer to be filed under.
func naukrigulfBaseJob(r naukrigulfRow) (Job, bool) {
	title := collapseSpace(r.Designation)
	company := collapseSpace(r.Company.Name)
	if r.JobID == "" || title == "" || company == "" || strings.EqualFold(company, naukrigulfConfidential) {
		return Job{}, false
	}
	j := Job{
		ExternalID: r.JobID,
		URL:        strings.TrimSpace(r.JdURL),
		Title:      title,
		Company:    company,
		Location:   collapseSpace(r.Location),
	}
	if r.LatestPostedDate > 0 {
		j.PostedAt = NotFuture(timePtr(time.Unix(int64(r.LatestPostedDate), 0).UTC()))
	}
	if r.Experience.Min > 0 {
		years := int(r.Experience.Min)
		j.ExperienceYearsMin = &years
	}
	return j, true
}

func timePtr(t time.Time) *time.Time { return &t }

// naukrigulfDetail is the part of the posting API's answer the adapter reads.
type naukrigulfDetail struct {
	Job struct {
		JobID          string `json:"JobId"`
		FunctionalArea string `json:"FunctionalArea"`
		Description    string `json:"Description"`
		EmploymentType string `json:"employmentType"`
		LocationType   string `json:"locationType"`
		IsExpired      bool   `json:"isExpired"`
		Compensation   struct {
			IsCtcHidden string `json:"IsCtcHidden"`
			MinCurrency string `json:"jobMinCurrency"`
			MaxCurrency string `json:"jobMaxCurrency"`
		} `json:"Compensation"`
		Other struct {
			Currency     string `json:"currLabel"`
			Confidential bool   `json:"isConfidentialCompany"`
		} `json:"Other"`
		DesiredCandidate struct {
			Profile string `json:"Profile"`
		} `json:"DesiredCandidate"`
	} `json:"Job"`
}

// withDetail adds the posting's body and structured facets. ok=false with a nil error is the
// site's answer that the posting is gone (404/410, or flagged expired); an error is the absence
// of an answer.
func (s naukrigulf) withDetail(ctx context.Context, base Job) (Job, bool, error) {
	var d naukrigulfDetail
	if err := s.get(ctx, fmt.Sprintf(naukrigulfDetailURL, url.PathEscape(base.ExternalID)), &d); err != nil {
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	if d.Job.JobID != base.ExternalID {
		return Job{}, false, fmt.Errorf("posting answered id %q", d.Job.JobID)
	}
	if d.Job.IsExpired || d.Job.Other.Confidential {
		return Job{}, false, nil
	}
	return naukrigulfApplyDetail(base, d)
}

// naukrigulfApplyDetail merges a posting's detail into its listing Job.
func naukrigulfApplyDetail(base Job, d naukrigulfDetail) (Job, bool, error) {
	body := sanitizeHTML(d.Job.Description)
	if profile := sanitizeHTML(d.Job.DesiredCandidate.Profile); strings.TrimSpace(profile) != "" {
		body += "\n<h3>Desired candidate</h3>\n" + profile
	}
	if strings.TrimSpace(body) == "" {
		return Job{}, false, errors.New("posting carries no body")
	}
	base.Description = body
	base.IsTechHint = naukrigulfIsIT(d.Job.FunctionalArea)
	switch strings.ToLower(strings.TrimSpace(d.Job.LocationType)) {
	case "remote":
		base.WorkMode, base.Remote = "remote", true
	case "hybrid":
		base.WorkMode = "hybrid"
	case "on-site", "onsite":
		base.WorkMode = "onsite"
	}
	switch strings.ToLower(strings.TrimSpace(d.Job.EmploymentType)) {
	case "full-time", "full time":
		base.EmploymentType = "full_time"
	case "part-time", "part time":
		base.EmploymentType = "part_time"
	case "contract", "temporary":
		base.EmploymentType = "contract"
	case "internship", "intern":
		base.EmploymentType = "internship"
	}
	if d.Job.Compensation.IsCtcHidden != "true" {
		naukrigulfSalary(&base, d)
	}
	return base, true, nil
}

// naukrigulfIsIT reports a posting the site itself files under its IT functional area: "IT
// Software", or a subcategory suffixed "(IT Software)" ("Software Development / Application
// Development (IT Software)"). The company's industry says nothing about the posting.
func naukrigulfIsIT(functionalArea string) bool {
	fa := strings.TrimSpace(functionalArea)
	return fa == "IT Software" || strings.HasSuffix(fa, "(IT Software)")
}

var naukrigulfDigits = regexp.MustCompile(`\d[\d,]*`)

// naukrigulfAmount reads the figure out of a salary label ("AED 12,000", "15,000").
func naukrigulfAmount(label string) int {
	m := naukrigulfDigits.FindString(label)
	n, err := strconv.Atoi(strings.ReplaceAll(m, ",", ""))
	if err != nil {
		return 0
	}
	return n
}

// naukrigulfSalary sets the posting's salary from the posting's own currency-denominated range.
// The public job pages and the employer posting form both label this field Monthly Salary.
func naukrigulfSalary(j *Job, d naukrigulfDetail) {
	lo := naukrigulfAmount(d.Job.Compensation.MinCurrency)
	hi := naukrigulfAmount(d.Job.Compensation.MaxCurrency)
	cur := strings.ToUpper(strings.TrimSpace(d.Job.Other.Currency))
	if len(cur) != 3 || lo <= 0 || hi < lo {
		return
	}
	j.SalaryMin, j.SalaryMax = &lo, &hi
	j.SalaryCurrency = cur
	j.SalaryPeriod = "month"
}
