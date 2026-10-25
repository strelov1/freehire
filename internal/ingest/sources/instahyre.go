package sources

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// instahyre adapts Instahyre (instahyre.com), an Indian hiring platform for technical roles whose
// postings each name their own employer. Board-based aggregator: a board is one of the search's
// COMPANY-SIZE buckets ("1" small, "3" medium, "2" large), the one filter that partitions the
// catalogue into slices the search can page through whole (it stops at 10,000 rows; the whole
// catalogue held 12,724 on 2026-10-25).
//
// The listing is the JSON search the site's own app calls (/api/v1/job_search), which carries no
// body, so this is a HydratingSource that reads a posting's schema.org JobPosting from its public
// page only when the catalogue lacks it, up to instahyreDetailBudget a run. The pages sit behind
// Cloudflare and are read through the Scrapling tier (scraplingtier.go); the search is not
// challenged. See the Instahyre traps in AGENTS.md.
type instahyre struct {
	api   JSONGetter
	pages HTMLGetter
	// spent counts the detail requests claimed across every board of the run (boards may be
	// crawled concurrently, hence atomic).
	spent *atomic.Int64
}

const (
	instahyreBase      = "https://www.instahyre.com"
	instahyreSearchURL = instahyreBase + "/api/v1/job_search?"

	// instahyrePageSize is what the search serves whatever limit is asked.
	instahyrePageSize = 35
	// instahyreWindow is the search's deepest row: offset+limit above it answers 400 "Result
	// window is too large".
	instahyreWindow = 10_000
	// instahyreRequestInterval paces every request of a run, search and page alike, at one a
	// second — a courtesy ceiling, not a measured limit.
	instahyreRequestInterval = time.Second
	instahyreRequestBurst    = 1
	instahyreDetailWorkers   = 2
	// instahyreDetailBudget bounds one run's page reads (~10 minutes at the pace above). The
	// ~365 search pages of a full walk answer in ~3 s each (61 pages took 183 s), ~18 minutes, and
	// both must fit the scheduler's default 2,400 s crawl budget. A cold start is ~12.7k pages,
	// ~21 runs.
	instahyreDetailBudget = 600
)

// instahyreBoards are the company-size buckets the search accepts; anything else answers 400
// "Invalid company size".
var instahyreBoards = map[string]bool{"1": true, "2": true, "3": true}

// NewInstahyre builds the Instahyre adapter: the search over api, the posting pages over pages.
func NewInstahyre(api JSONGetter, pages HTMLGetter) Source {
	return instahyre{api: api, pages: pages, spent: new(atomic.Int64)}
}

// pacedInstahyre holds the search and the pages of a run to one limiter.
func pacedInstahyre(api JSONGetter, pages HTMLGetter) Source {
	limiter := rate.NewLimiter(rate.Every(instahyreRequestInterval), instahyreRequestBurst)
	return NewInstahyre(rateLimitedJSONGetter{inner: api, limiter: limiter}, rateLimitedHTMLGetter{inner: pages, limiter: limiter})
}

func (instahyre) Provider() string { return "instahyre" }

// Every posting names its own employer, so Instahyre stays in the source facet and in the
// cross-source duplicate suppression set.
func (instahyre) aggregator() {}

// instahyreListing is one search row, reduced to what the adapter reads.
type instahyreListing struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Locations string `json:"locations"`
	PublicURL string `json:"public_url"`
	Employer  struct {
		CompanyName string `json:"company_name"`
	} `json:"employer"`
}

type instahyreSearchPage struct {
	Meta struct {
		TotalCount int `json:"total_count"`
	} `json:"meta"`
	Objects []instahyreListing `json:"objects"`
}

// instahyrePosting is the schema.org JobPosting on a posting's page.
type instahyrePosting struct {
	Title          string `json:"title"`
	Description    string `json:"description"`
	DatePosted     string `json:"datePosted"`
	EmploymentType string `json:"employmentType"`
	JobLocation    []struct {
		Address struct {
			Locality string `json:"addressLocality"`
			Country  string `json:"addressCountry"`
		} `json:"address"`
	} `json:"jobLocation"`
}

func (s instahyre) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s instahyre) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	return s.FetchNewGated(ctx, e, seen, DetailGate{})
}

// FetchNewGated is FetchNew that reads no page for a posting the pipeline will discard anyway: a
// discarded posting is never stored, so without the gate every run would read it again.
func (s instahyre) FetchNewGated(ctx context.Context, e CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error) {
	if !instahyreBoards[e.Board] {
		return nil, fmt.Errorf("instahyre: board %q is not a company-size bucket (1, 2 or 3)", e.Board)
	}
	rows, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	return s.hydrate(ctx, rows, seen, gate)
}

// list walks the bucket to its end. The first page failing fails the board; a later page failing
// ends the walk with what was read. A bucket past the search's window is read to the window and
// logged: it means the buckets no longer partition the catalogue finely enough.
func (s instahyre) list(ctx context.Context, size string) ([]instahyreListing, error) {
	var out []instahyreListing
	ids := map[int64]bool{}
	for offset := 0; offset+instahyrePageSize <= instahyreWindow; offset += instahyrePageSize {
		q := url.Values{}
		q.Set("company_size", size)
		q.Set("limit", strconv.Itoa(instahyrePageSize))
		q.Set("offset", strconv.Itoa(offset))
		var p instahyreSearchPage
		if err := s.api.GetJSON(ctx, instahyreSearchURL+q.Encode(), &p); err != nil {
			if offset == 0 {
				return nil, fmt.Errorf("instahyre: company size %s: %w", size, err)
			}
			log.Printf("instahyre: company size %s: offset %d failed; keeping %d postings read: %v", size, offset, len(out), err)
			break
		}
		for _, r := range p.Objects {
			if r.ID <= 0 || ids[r.ID] {
				continue
			}
			ids[r.ID] = true
			out = append(out, r)
		}
		if len(p.Objects) == 0 || offset+instahyrePageSize >= p.Meta.TotalCount {
			break
		}
		if offset+2*instahyrePageSize > instahyreWindow {
			log.Printf("instahyre: company size %s: %d postings, the search serves the first %d",
				size, p.Meta.TotalCount, instahyreWindow)
		}
	}
	return out, nil
}

// hydrate maps the listed rows to Jobs. A seen posting, and one the gate condemns, costs no
// request; every other posting costs one page read, up to the run's budget, claimed in a
// sequential pass before any request goes out. A page that could not be read, or that the budget
// did not reach, defers its posting to a later run rather than storing it body-less.
func (s instahyre) hydrate(ctx context.Context, rows []instahyreListing, seen func(string) bool, gate DetailGate) ([]Job, error) {
	var covered map[string]bool
	if gate.Covered != nil {
		companies := make([]string, 0, len(rows))
		for _, r := range rows {
			companies = append(companies, strings.TrimSpace(r.Employer.CompanyName))
		}
		covered = gate.Covered(companies)
	}
	var jobs, claimed []Job
	deferred := 0
	for _, r := range rows {
		base, ok := r.toJob()
		if !ok {
			continue
		}
		if seen(base.ExternalID) {
			base.SeenRefresh = true
			jobs = append(jobs, base)
			continue
		}
		if covered[base.Company] || (gate.Rejected != nil && gate.Rejected(base)) {
			jobs = append(jobs, base)
			continue
		}
		if s.spent.Add(1) > instahyreDetailBudget {
			deferred++
			continue
		}
		claimed = append(claimed, base)
	}
	var answered atomic.Int64
	jobs = append(jobs, fetchDetails(claimed, instahyreDetailWorkers, func(base Job) (Job, bool) {
		job, err := s.withPage(ctx, base)
		if err != nil {
			log.Printf("instahyre: posting %s failed; deferring: %v", base.ExternalID, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, true
	})...)
	// A crawl that read no page at all is broken, not quiet.
	if len(claimed) > 0 && answered.Load() == 0 {
		return nil, fmt.Errorf("instahyre: requested %d posting pages and none answered", len(claimed))
	}
	if deferred > 0 {
		log.Printf("instahyre: detail budget reached (%d a run); %d new postings deferred to later runs", instahyreDetailBudget, deferred)
	}
	return jobs, nil
}

// withPage adds the body, places, posting date and employment type from the page's JobPosting.
// A page without one is not read as gone: a closed posting's page drops its JobPosting, but so
// does anything that is not the posting (a challenge page), and a listed posting is open.
func (s instahyre) withPage(ctx context.Context, base Job) (Job, error) {
	root, err := s.pages.GetHTML(ctx, base.URL)
	if err != nil {
		return Job{}, err
	}
	var p instahyrePosting
	if !LDJobPosting(root, &p) {
		return Job{}, fmt.Errorf("page carries no JobPosting")
	}
	body := sanitizeHTML(p.Description)
	if strings.TrimSpace(body) == "" {
		return Job{}, fmt.Errorf("posting carries no body")
	}
	base.Description = body
	var places []string
	for _, l := range p.JobLocation {
		if c := strings.TrimSpace(l.Address.Locality); c != "" {
			places = append(places, c)
		}
	}
	// The page's places are the search's own; they stand in only when the row named none.
	if base.Location == "" {
		base.Location = strings.Join(places, "; ")
	}
	if t, err := time.Parse("2006-01-02", strings.TrimSpace(p.DatePosted)); err == nil {
		base.PostedAt = NotFuture(&t)
	}
	switch strings.ToUpper(strings.TrimSpace(p.EmploymentType)) {
	case "FULL_TIME":
		base.EmploymentType = "full_time"
	case "PART_TIME":
		base.EmploymentType = "part_time"
	case "INTERN", "INTERNSHIP":
		base.EmploymentType = "internship"
	case "CONTRACTOR":
		base.EmploymentType = "contract"
	}
	return base, nil
}

// toJob maps a search row to a body-less Job. "Work From Home" is one of the search's own places,
// and a posting whose only place it is states remote work. Most postings are technical (the
// search's largest functions are backend, full-stack and data), but support and sales roles are
// listed too, so the title dictionary decides; no tech hint is set.
func (r instahyreListing) toJob() (Job, bool) {
	title := strings.Join(strings.Fields(r.Title), " ")
	company := strings.TrimSpace(r.Employer.CompanyName)
	link := strings.TrimSpace(r.PublicURL)
	if r.ID <= 0 || title == "" || company == "" || !strings.HasPrefix(link, instahyreBase+"/job-") {
		return Job{}, false
	}
	j := Job{
		ExternalID:        strconv.FormatInt(r.ID, 10),
		URL:               link,
		Title:             title,
		Company:           company,
		FallbackCountries: []string{"in"},
	}
	var places []string
	remote := false
	for _, p := range strings.Split(r.Locations, ",") {
		p = strings.TrimSpace(p)
		switch {
		case p == "":
		case strings.EqualFold(p, "Work From Home"):
			remote = true
		default:
			places = append(places, p)
		}
	}
	if remote {
		// Beside an office, "Work From Home" is left to the location parse as "Remote".
		places = append(places, "Remote")
		if len(places) == 1 {
			j.WorkMode, j.Remote = "remote", true
		}
	}
	j.Location = strings.Join(places, "; ")
	return j, true
}
