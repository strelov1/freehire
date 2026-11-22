package sources

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// unstop adapts Unstop (unstop.com), the Indian student and early-career platform. Besides
// competitions and quizzes, which are not postings and are never read, it lists jobs and
// internships across every profession. Boardless and an aggregator: the employer is read per
// posting. See providers/unstop.md for the measurements behind each rule.
//
// robots.txt allows /api/public/*, and its search endpoint is what the site's own listing pages
// call. It carries every posting's body and structured facets inline, so there is no detail
// request. The two feeds ("jobs" and "internships") are read through oppstatus=open, which is
// the only slice that is complete: without it the endpoint pages through a 10,000-row window
// (667 pages, then a 500) of open and expired postings alike.
type unstop struct {
	http JSONGetter
}

const (
	unstopListURL = "https://unstop.com/api/public/opportunity/search-result"
	// The two feeds each fit in two 500-row pages. Smaller pages repeated IDs at their
	// boundaries, and 500 rows keeps each payload below the size of a single 1,000-row page.
	unstopPerPage = 500
	// unstopMaxPages bounds one feed so a response that never reaches its last page cannot loop.
	unstopMaxPages = 200
	// unstopRequestInterval honours the robots.txt Crawl-delay of 1.
	unstopRequestInterval = time.Second
	// The reported total can exceed the rows returned even when the feed fits on one page
	// (662 reported internships, 640 rows). Leave room for that gap and minor offset drift.
	unstopCountTolerance = 5
)

// unstopFeeds are the two opportunity kinds that are postings. The value is the endpoint's
// `opportunity` parameter, and an internship is a job whose subtype says so.
var unstopFeeds = []string{"jobs", "internships"}

// NewUnstop builds the adapter over the given client.
func NewUnstop(c JSONGetter) Source { return unstop{http: c} }

func (unstop) Provider() string { return "unstop" }

// Boardless marks unstop as having no per-tenant board id: one crawl covers the whole site.
func (unstop) Boardless() {}

// Aggregator keeps unstop in the source facet: it lists many employers.
func (unstop) Aggregator() {}

// FullCatalog: Fetch pages each feed to its declared last page, fails on any page error, on a
// feed that lists nothing and on a crawl that lost more of the reported total than
// unstopCountTolerance allows, so a finished crawl has seen every open posting. See the
// fullCatalog interface (markers.go).
func (unstop) FullCatalog() {}

// unstopPage is one page of the search endpoint.
type unstopPage struct {
	Data struct {
		LastPage int             `json:"last_page"`
		Total    int             `json:"total"`
		Data     []unstopPosting `json:"data"`
	} `json:"data"`
}

type unstopPosting struct {
	ID           int    `json:"id"`
	Title        string `json:"title"`
	Details      string `json:"details"`
	SEOURL       string `json:"seo_url"`
	Status       string `json:"status"`
	Visibility   string `json:"visibility"`
	RegnOpen     int    `json:"regn_open"`
	Subtype      string `json:"subtype"`
	ApprovedDate string `json:"approved_date"`
	EndDate      string `json:"end_date"`
	Organisation struct {
		Name string `json:"name"`
	} `json:"organisation"`
	Locations []struct {
		City    string `json:"city"`
		State   string `json:"state"`
		Country string `json:"country"`
	} `json:"locations"`
	JobDetail *unstopJobDetail `json:"jobDetail"`
}

type unstopJobDetail struct {
	Type         string `json:"type"`
	Timing       string `json:"timing"`
	Currency     string `json:"currency"`
	PayIn        string `json:"pay_in"`
	MinSalary    *int   `json:"min_salary"`
	MaxSalary    *int   `json:"max_salary"`
	ShowSalary   int    `json:"show_salary"`
	NotDisclosed bool   `json:"not_disclosed"`
}

// Fetch reads both feeds whole. Any failure fails the run: unstop is swept by source, so a
// listing missing a feed would close every posting in it.
func (u unstop) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	var jobs []Job
	for _, feed := range unstopFeeds {
		got, err := u.feed(ctx, feed)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, got...)
	}
	return jobs, nil
}

func (u unstop) feed(ctx context.Context, feed string) ([]Job, error) {
	var jobs []Job
	seen := map[int]bool{}
	var total int
	for page := 1; ; page++ {
		if page > unstopMaxPages {
			return nil, fmt.Errorf("unstop: %s: no last page after %d pages", feed, unstopMaxPages)
		}
		var resp unstopPage
		url := fmt.Sprintf("%s?opportunity=%s&oppstatus=open&per_page=%d&page=%d", unstopListURL, feed, unstopPerPage, page)
		if err := u.http.GetJSON(ctx, url, &resp); err != nil {
			return nil, fmt.Errorf("unstop: %s page %d: %w", feed, page, err)
		}
		total = resp.Data.Total
		for _, p := range resp.Data.Data {
			if p.ID == 0 || seen[p.ID] {
				continue
			}
			seen[p.ID] = true
			if job, ok := p.toJob(); ok {
				jobs = append(jobs, job)
			}
		}
		if page >= resp.Data.LastPage {
			break
		}
	}
	if total == 0 {
		return nil, fmt.Errorf("unstop: %s lists no open posting", feed)
	}
	if missing := total - len(seen); missing*100 > total*unstopCountTolerance {
		return nil, fmt.Errorf("unstop: %s: read %d of %d reported postings", feed, len(seen), total)
	}
	return jobs, nil
}

// unstopWorkModes maps the posting's own work-arrangement enum. on_field (sales visits and the
// like) is neither an office nor remote, so it states no mode.
var unstopWorkModes = map[string]string{"wfh": "remote", "in_office": "onsite", "hybrid": "hybrid"}

// unstopEmploymentTypes maps jobDetail.timing; an internship overrides it.
var unstopEmploymentTypes = map[string]string{"full_time": "full_time", "part_time": "part_time", "contractual": "contract"}

// unstopSalaryPeriods maps jobDetail.pay_in to the period vocabulary.
var unstopSalaryPeriods = map[string]string{"annually": "year", "monthly": "month"}

// toJob maps a posting, returning ok=false for one the adapter cannot stand behind: no id, title,
// employer or body, or one the site no longer shows as open.
func (p unstopPosting) toJob() (Job, bool) {
	title := strings.Join(strings.Fields(p.Title), " ")
	company := strings.Join(strings.Fields(p.Organisation.Name), " ")
	description := sanitizeHTML(p.Details)
	if title == "" || company == "" || p.SEOURL == "" || p.Status != "LIVE" || p.RegnOpen != 1 ||
		p.Visibility != "public" || strings.TrimSpace(textFromHTML(description)) == "" {
		return Job{}, false
	}
	job := Job{
		ExternalID:  strconv.Itoa(p.ID),
		URL:         p.SEOURL,
		Title:       title,
		Company:     company,
		Location:    p.locationLine(),
		Description: description,
		PostedAt:    parseLayout("2006-01-02 15:04:05 GMT-0700", p.ApprovedDate),
		// Unstop is an Indian platform and every posting measured named India, but a posting
		// with no place at all (a work-from-home role) states no country, hence the weak signal.
		FallbackCountries: []string{"in"},
	}
	// The deadline is the date part of end_date as the site states it. It does not go through
	// ParseDate, whose NotFuture is for a posted_at and would drop a deadline days away.
	if len(p.EndDate) >= len("2006-01-02") {
		if t, err := time.Parse("2006-01-02", p.EndDate[:len("2006-01-02")]); err == nil {
			job.ApplicationDeadline = &t
		}
	}
	if d := p.JobDetail; d != nil {
		job.WorkMode = unstopWorkModes[d.Type]
		job.Remote = job.WorkMode == "remote"
		job.EmploymentType = unstopEmploymentTypes[d.Timing]
		d.salary(&job)
	}
	if p.Subtype == "internships" {
		job.EmploymentType = "internship"
	}
	return job, true
}

// locationLine joins the posting's places as "City, State, Country", one per place. A posting
// can name several cities.
func (p unstopPosting) locationLine() string {
	var places []string
	for _, l := range p.Locations {
		var parts []string
		for _, s := range []string{l.City, l.State, l.Country} {
			if s = strings.TrimSpace(s); s != "" {
				parts = append(parts, s)
			}
		}
		if len(parts) > 0 {
			places = append(places, strings.Join(parts, ", "))
		}
	}
	return distinctJoin(places, "; ", func(s string) string { return s })
}

// salary takes the pay a posting shows. A hidden or undisclosed salary, a currency other than the
// rupee and a period outside the table are left without rather than guessed.
func (d *unstopJobDetail) salary(job *Job) {
	period := unstopSalaryPeriods[d.PayIn]
	if d.ShowSalary != 1 || d.NotDisclosed || d.Currency != "fa-rupee" || period == "" {
		return
	}
	lo, hi := d.MinSalary, d.MaxSalary
	if (lo == nil || *lo <= 0) && (hi == nil || *hi <= 0) {
		return
	}
	if lo != nil && *lo > 0 {
		job.SalaryMin = lo
	}
	if hi != nil && *hi > 0 {
		job.SalaryMax = hi
	}
	job.SalaryCurrency, job.SalaryPeriod = "INR", period
}
