package sources

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync/atomic"
)

// hydrate maps the candidates to Jobs, buying a posting page only for a posting the pipeline
// will keep, within the run's budget. Candidates arrive newest first (the window, then the
// backlog), so the budget goes to the freshest postings.
//
// A new posting whose page could not be read is DEFERRED rather than stored body-less: a
// stored row is seen, and once it ages past pipeline.HydrationRetryWindow nothing would buy
// its body again, while a deferred one is simply new on a later run.
//
// Once the edge refuses one request (403/429) the rest of the run's hydration is deferred too:
// the refusal is about the IP, and every further request would be refused alike and only
// prolong the penalty.
func (s startupjobs) hydrate(ctx context.Context, c startupjobsCrawl, covered map[string]bool) ([]Job, error) {
	out := c.refreshes
	var toHydrate []Job
	deferred := 0
	for _, h := range c.candidates {
		base, ok := h.toJob()
		if !ok {
			continue
		}
		switch {
		case covered[base.Company]:
			out = append(out, base) // the pipeline discards it either way; a body would be pure loss
		case len(toHydrate) < s.maxNewPerRun:
			toHydrate = append(toHydrate, base)
		default:
			deferred++
		}
	}
	if deferred > 0 {
		log.Printf("startupjobs: hydration budget %d reached; %d new postings wait for a later run", s.maxNewPerRun, deferred)
	}

	var refused atomic.Bool
	var failed atomic.Int64
	hydrated := fetchDetails(toHydrate, startupjobsDetailWorkers, func(j Job) (Job, bool) {
		if refused.Load() {
			return Job{}, false
		}
		body, err := s.body(ctx, j.URL)
		if err != nil {
			if isRateLimited(err) && !refused.Swap(true) {
				log.Printf("startupjobs: posting pages refused (%v); deferring the rest of this run's hydration", err)
			}
			if detailUnreadable(err) { // a 404/410 is the posting gone, which is an answer
				failed.Add(1)
			}
			return Job{}, false
		}
		j.Description = sanitizeHTML(body)
		return j, true
	})
	if n := failed.Load(); n > 0 {
		log.Printf("startupjobs: %d/%d posting pages could not be read; deferred to a later run", n, len(toHydrate))
	}
	// A run that needed bodies and could read none of them is a failure, not a quiet source —
	// see AGENTS.md's "a crawl that reads nothing of what it listed must fail". The Cloudflare
	// wall is exactly the failure this would otherwise hide behind a green unit. Postings that
	// were merely gone are an answer, not a failure.
	if failed.Load() > 0 && len(hydrated) == 0 {
		return nil, fmt.Errorf("startupjobs: needed %d posting pages and read none of them", len(toHydrate))
	}
	return append(out, hydrated...), nil
}

// body reads a posting page's schema.org JobPosting description. A page that answers without
// one is an error like any other unreadable page: the posting is deferred, not stored empty.
func (s startupjobs) body(ctx context.Context, url string) (string, error) {
	root, err := s.site.GetHTML(ctx, url)
	if err != nil {
		return "", err
	}
	var p startupjobsPosting
	if !ldJobPosting(root, &p) || strings.TrimSpace(p.Description) == "" {
		return "", fmt.Errorf("startupjobs: %s carries no JobPosting description", url)
	}
	return p.Description, nil
}

// usable reports whether the index states enough for this hit to become a Job: an id to key
// it on, a path to link it to, a title, and the employer the catalogue files it under.
func (h startupjobsHit) usable() bool {
	return h.ObjectID != "" && strings.HasPrefix(h.Path, "/") && h.Title != "" && h.CompanyName != ""
}

// toJob maps an index hit to a body-less Job. PostedAt stays nil: the index's published_at
// is re-stamped in daily batches, and the page's datePosted is not a posting date either.
func (h startupjobsHit) toJob() (Job, bool) {
	if !h.usable() {
		return Job{}, false
	}
	job := Job{
		ExternalID:     h.ObjectID,
		URL:            startupjobsBaseURL + h.Path,
		Title:          h.Title,
		Company:        h.CompanyName,
		Location:       h.Location,
		WorkMode:       workplaceTypeMode(h.WorkplaceTypeID),
		EmploymentType: startupjobsEmploymentType(h.EmploymentTypeID),
		Countries:      countryFromCode(h.CountryCode),
	}
	job.Remote = job.WorkMode == "remote"
	h.applySalary(&job)
	return job, true
}

// applySalary maps the index's structured salary. The bounds are whole currency units, the
// currency is ISO 4217 and the interval names the period; an interval freehire's vocabulary has
// no value for (week, biweekly) drops the whole salary rather than filing an amount under the
// wrong period.
func (h startupjobsHit) applySalary(job *Job) {
	if h.SalaryCurrency == "" || (h.SalaryMin == nil && h.SalaryMax == nil) {
		return
	}
	period := startupjobsSalaryPeriod(h.SalaryInterval)
	if period == "" {
		return
	}
	var lo, hi *int
	if h.SalaryMin != nil {
		lo = roundSalaryPart(*h.SalaryMin)
	}
	if h.SalaryMax != nil {
		hi = roundSalaryPart(*h.SalaryMax)
	}
	if lo == nil && hi == nil {
		return
	}
	job.SalaryMin, job.SalaryMax = lo, hi
	job.SalaryCurrency, job.SalaryPeriod = strings.ToUpper(h.SalaryCurrency), period
}

// startupjobsSalaryPeriod maps the index's salary_interval onto vocab.SalaryPeriodValues; week
// and biweekly have no freehire period.
func startupjobsSalaryPeriod(interval string) string {
	switch interval {
	case "year", "month", "day", "hour":
		return interval
	default:
		return ""
	}
}

// startupjobsEmploymentType maps the index's employment_type_id — a closed four-value facet
// (full-time, contractor, part-time, internship) — onto vocab.EmploymentTypeValues.
func startupjobsEmploymentType(t string) string {
	switch t {
	case "full-time":
		return "full_time"
	case "part-time":
		return "part_time"
	case "contractor":
		return "contract"
	case "internship":
		return "internship"
	default:
		return ""
	}
}
