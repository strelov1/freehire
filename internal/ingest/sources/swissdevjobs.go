package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// swissdevjobs adapts SwissDevJobs (swissdevjobs.ch), a Swiss IT-only job board: a few hundred
// postings, almost every one with the employer's yearly salary in CHF. It is boardless and an
// aggregator — the employer is read per posting. See the "SwissDevJobs traps" section of
// AGENTS.md for the measurements behind each rule.
//
// The listing is the site's own public feed, /api/jobsLight: the whole catalogue in one request,
// everything but the body. The body is bought from the posting page, whose server-rendered
// window.__detailedJob holds the posting, only for postings the catalogue lacks.
type swissdevjobs struct {
	get            swissdevjobsHTTP
	limiter        waiter
	maxNewPerRun   int
	deadlineMargin time.Duration
}

type swissdevjobsHTTP interface {
	JSONGetter
	TextGetter
}

const (
	swissdevjobsBase    = "https://swissdevjobs.ch"
	swissdevjobsListURL = swissdevjobsBase + "/api/jobsLight"
	// swissdevjobsRequestInterval paces every request at one a second — a courtesy ceiling, not a
	// measured limit.
	swissdevjobsRequestInterval = time.Second
	swissdevjobsDetailWorkers   = 2
	// swissdevjobsMaxNewPerRun bounds one run's posting pages. The board held 228 postings on
	// 2026-10-29, so a first run reads it whole and later runs read the few new ones.
	swissdevjobsMaxNewPerRun   = 1000
	swissdevjobsDeadlineMargin = 5 * time.Minute
	swissdevjobsDetailMarker   = "window.__detailedJob ="
)

// NewSwissDevJobs builds the adapter over the given client.
func NewSwissDevJobs(c swissdevjobsHTTP) Source {
	return swissdevjobs{
		get:            c,
		limiter:        rate.NewLimiter(rate.Every(swissdevjobsRequestInterval), 1),
		maxNewPerRun:   swissdevjobsMaxNewPerRun,
		deadlineMargin: swissdevjobsDeadlineMargin,
	}
}

func (swissdevjobs) Provider() string { return "swissdevjobs" }

// One national board, no per-tenant id.
func (swissdevjobs) boardless() {}

// Around a hundred employers post here, each named per posting.
func (swissdevjobs) aggregator() {}

// swissdevjobsJob is one feed row; the posting page's state adds the three text fields.
type swissdevjobsJob struct {
	ID           string   `json:"_id"`
	Slug         string   `json:"jobUrl"`
	Name         string   `json:"name"`
	Company      string   `json:"company"`
	City         string   `json:"actualCity"`
	ActiveFrom   string   `json:"activeFrom"`
	Workplace    string   `json:"workplace"`
	JobType      string   `json:"jobType"`
	ExpLevel     string   `json:"expLevel"`
	MetaCategory string   `json:"metaCategory"`
	SalaryFrom   *float64 `json:"annualSalaryFrom"`
	SalaryTo     *float64 `json:"annualSalaryTo"`
	Paused       bool     `json:"isPaused"`
	// Only on the posting page.
	Description      string `json:"description"`
	Responsibilities string `json:"responsibilitiesTextArea"`
	Requirements     string `json:"requirementsMustTextArea"`
	Outdated         bool   `json:"isDisabledOrOutdated"`
}

func (j swissdevjobsJob) url() string { return swissdevjobsBase + "/jobs/" + j.Slug }

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s swissdevjobs) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s swissdevjobs) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	return s.FetchNewGated(ctx, e, seen, DetailGate{})
}

// FetchNewGated reads the feed, refreshes the postings already stored, yields body-less those the
// pipeline will discard anyway, and reads up to maxNewPerRun of the rest. A posting whose page
// cannot be read is not yielded; a later run tries again.
func (s swissdevjobs) FetchNewGated(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error) {
	rows, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var covered map[string]bool
	if gate.Covered != nil {
		companies := make([]string, 0, len(rows))
		for _, r := range rows {
			companies = append(companies, swissdevjobsClean(r.Company))
		}
		covered = gate.Covered(companies)
	}
	var jobs []Job
	var unseen []swissdevjobsJob
	deferred := 0
	for _, r := range rows {
		base := swissdevjobsBaseJob(r)
		if seen(base.ExternalID) {
			base.SeenRefresh = true
			jobs = append(jobs, base)
			continue
		}
		if covered[base.Company] || (gate.Rejected != nil && gate.Rejected(base)) {
			jobs = append(jobs, base)
			continue
		}
		if len(unseen) >= s.maxNewPerRun {
			deferred++
			continue
		}
		unseen = append(unseen, r)
	}
	var answered, outOfTime atomic.Int64
	read := fetchDetails(unseen, swissdevjobsDetailWorkers, func(r swissdevjobsJob) (Job, bool) {
		if d, ok := ctx.Deadline(); ok && time.Until(d) < s.deadlineMargin {
			outOfTime.Add(1)
			return Job{}, false
		}
		job, ok, err := s.detail(ctx, r)
		if err != nil {
			log.Printf("swissdevjobs: posting %s failed; deferring: %v", r.Slug, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, ok
	})
	if requested := int64(len(unseen)) - outOfTime.Load(); requested > 0 && answered.Load() == 0 {
		return nil, fmt.Errorf("swissdevjobs: requested %d posting pages and none answered", requested)
	}
	if n := outOfTime.Load(); n > 0 {
		log.Printf("swissdevjobs: %d new postings left unread as the run neared its deadline, deferred to later runs", n)
	}
	if deferred > 0 {
		log.Printf("swissdevjobs: %d new postings past this run's bound of %d, deferred to later runs", deferred, s.maxNewPerRun)
	}
	return append(jobs, read...), nil
}

// list reads the feed and keeps the live postings with an id, a page, a title and an employer.
func (s swissdevjobs) list(ctx context.Context) ([]swissdevjobsJob, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	var rows []swissdevjobsJob
	if err := s.get.GetJSON(ctx, swissdevjobsListURL, &rows); err != nil {
		return nil, fmt.Errorf("swissdevjobs: feed: %w", err)
	}
	var out []swissdevjobsJob
	for _, r := range rows {
		if r.Paused || r.ID == "" || r.Slug == "" || swissdevjobsClean(r.Name) == "" || swissdevjobsClean(r.Company) == "" {
			continue
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, errors.New("swissdevjobs: feed carries no posting")
	}
	return out, nil
}

// swissdevjobsBaseJob is the Job the feed row alone supports: everything but the body.
func swissdevjobsBaseJob(r swissdevjobsJob) Job {
	job := Job{
		ExternalID:        r.ID,
		URL:               r.url(),
		Title:             swissdevjobsClean(r.Name),
		Company:           swissdevjobsClean(r.Company),
		Location:          swissdevjobsClean(r.City),
		PostedAt:          NotFuture(parseRFC3339(r.ActiveFrom)),
		WorkMode:          swissdevjobsWorkMode(r.Workplace),
		EmploymentType:    swissdevjobsEmploymentType(r.JobType),
		Seniority:         swissdevjobsSeniority(r.ExpLevel),
		FallbackCountries: []string{"ch"},
		// An IT-only board; its "business" category is sales and business development.
		IsTechHint: r.MetaCategory != "business",
	}
	job.Remote = job.WorkMode == "remote"
	if r.SalaryFrom != nil && *r.SalaryFrom > 0 {
		lo := int(*r.SalaryFrom)
		hi := lo
		if r.SalaryTo != nil && int(*r.SalaryTo) >= lo {
			hi = int(*r.SalaryTo)
		}
		job.SalaryMin, job.SalaryMax, job.SalaryCurrency, job.SalaryPeriod = &lo, &hi, "CHF", "year"
	}
	return job
}

// detail fetches the posting page and reads its state. A posting the site no longer carries
// answers 404, or a page whose posting is flagged outdated, and is not yielded.
func (s swissdevjobs) detail(ctx context.Context, r swissdevjobsJob) (Job, bool, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return Job{}, false, err
	}
	page, err := s.get.GetText(ctx, r.url())
	if err != nil {
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	i := strings.Index(page, swissdevjobsDetailMarker)
	if i < 0 {
		return Job{}, false, errors.New("page carries no posting state")
	}
	var d swissdevjobsJob
	if err := json.NewDecoder(strings.NewReader(page[i+len(swissdevjobsDetailMarker):])).Decode(&d); err != nil {
		return Job{}, false, fmt.Errorf("posting state: %w", err)
	}
	if d.ID != r.ID || d.Outdated || d.Paused {
		return Job{}, false, nil
	}
	body := swissdevjobsBody(d)
	if body == "" {
		return Job{}, false, nil
	}
	job := swissdevjobsBaseJob(r)
	job.Description = body
	return job, true, nil
}

// swissdevjobsBody joins the posting's three plain-text fields, each section under the heading
// the page shows it with.
func swissdevjobsBody(d swissdevjobsJob) string {
	var b strings.Builder
	for _, sec := range [][2]string{
		{"", d.Description},
		{"Responsibilities", d.Responsibilities},
		{"Requirements", d.Requirements},
	} {
		text := strings.TrimSpace(sec[1])
		if text == "" {
			continue
		}
		if sec[0] != "" {
			b.WriteString("<h3>" + html.EscapeString(sec[0]) + "</h3>")
		}
		b.WriteString(plainTextToHTML(text))
	}
	return sanitizeHTML(b.String())
}

func swissdevjobsWorkMode(w string) string {
	switch w {
	case "remote":
		return "remote"
	case "hybrid":
		return "hybrid"
	case "office":
		return "onsite"
	}
	return ""
}

func swissdevjobsEmploymentType(t string) string {
	switch t {
	case "Full-Time":
		return "full_time"
	case "Part-Time":
		return "part_time"
	case "Internship":
		return "internship"
	case "Contract", "Freelance":
		return "contract"
	}
	return ""
}

// swissdevjobsSeniority maps the board's four levels; "Regular" is its middle one.
func swissdevjobsSeniority(l string) string {
	switch l {
	case "Junior":
		return "junior"
	case "Regular":
		return "middle"
	case "Senior":
		return "senior"
	case "Lead":
		return "lead"
	}
	return ""
}

func swissdevjobsClean(s string) string { return strings.Join(strings.Fields(s), " ") }
