package sources

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"github.com/strelov1/freehire/internal/dict/location"
	"github.com/strelov1/freehire/internal/dict/normalize"
)

// mojedelo adapts MojeDelo (www.mojedelo.com), Slovenia's largest general job board: ~4k open
// postings, every trade, a few percent technical, a few dozen abroad (mostly Austria). It is
// boardless and an aggregator — the employer is read per posting. See the "MojeDelo traps"
// section of AGENTS.md for the measurements behind each rule.
//
// The site is a single-page app over its own JSON API (api.mojedelo.com). The search lists each
// posting's title, employer, place, country and arrangement — everything but the body, which
// is bought from the posting's own resource for postings the catalogue lacks, within a per-run
// budget.
type mojedelo struct {
	http           HeaderJSONGetter
	limiter        waiter
	maxNewPerRun   int
	deadlineMargin time.Duration
}

const (
	mojedeloAPI = "https://api.mojedelo.com"
	// mojedeloSearchURL is the public search with no filter: the whole catalogue, promoted
	// postings first, then newest first. The order is stable: a full walk read 3,960 distinct
	// postings in 3,960 rows.
	mojedeloSearchURL = mojedeloAPI + "/job-ads-search?pageSize=%d&startFrom=%d"
	mojedeloJobURL    = mojedeloAPI + "/job-ads/%s"
	mojedeloPageURL   = "https://www.mojedelo.com/oglas/%s/%s"

	// mojedeloPageSize is the largest page the search serves: it validates pageSize up to 200
	// but answers 500 to anything above 100.
	mojedeloPageSize = 100
	// mojedeloMaxPages is a safety ceiling: 40 pages on 2026-10-29.
	mojedeloMaxPages = 200
	// mojedeloRequestInterval paces every request of a run, search page and posting alike, at
	// one a second — a courtesy ceiling, not a measured limit.
	mojedeloRequestInterval = time.Second
	mojedeloDetailWorkers   = 2
	// mojedeloMaxNewPerRun bounds the posting reads of one run: the 40 search pages take ~5
	// minutes (each answers in ~7 s), so 1,200 postings at one a second and the writes after
	// them fit the scheduler's 2,400 s crawl budget. A cold catalogue is taken in slices.
	mojedeloMaxNewPerRun   = 1200
	mojedeloDeadlineMargin = 5 * time.Minute
)

// mojedeloHeaders name the tenant every API request must carry ("Tenant header missing"
// otherwise). They are MojeDelo's public ids, read from the config the site itself loads
// (api.mojedelo.com/uploaded-files/config/www.mojedelo.com/jb.globals.js), not credentials.
var mojedeloHeaders = map[string]string{
	"tenantId":   "5947a585-ad25-47dc-bff3-f08620d1ce17",
	"channelId":  "8805c1b8-a0a9-4f57-ad42-329af3c92a61",
	"languageId": "db3c58e6-a083-4f72-b30b-39f2127bb18d",
}

// NewMojeDelo builds the adapter over the given client.
func NewMojeDelo(c HeaderJSONGetter) Source {
	return mojedelo{
		http:           c,
		limiter:        rate.NewLimiter(rate.Every(mojedeloRequestInterval), 1),
		maxNewPerRun:   mojedeloMaxNewPerRun,
		deadlineMargin: mojedeloDeadlineMargin,
	}
}

func (mojedelo) Provider() string { return "mojedelo" }

// One national board, no per-tenant id.
func (mojedelo) boardless() {}

// Thousands of employers and staffing agencies post here, each named per posting.
func (mojedelo) aggregator() {}

type mojedeloNamed struct {
	Name        string `json:"name"`
	Translation string `json:"translation"`
}

type mojedeloCompany struct {
	Name string `json:"name"`
}

// mojedeloItem is one search row: what the gate and the job need before the posting is bought.
type mojedeloItem struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	StartDate string `json:"startDate"`
	Town      *struct {
		Translation string `json:"translation"`
	} `json:"town"`
	Country *struct {
		CountryCode string `json:"countryCode"`
	} `json:"country"`
	JobLocationInput  string           `json:"jobLocationInput"`
	WorkModes         []mojedeloNamed  `json:"workModes"`
	Company           *mojedeloCompany `json:"company"`
	AnonymizedCompany *mojedeloCompany `json:"anonymizedCompany"`
}

type mojedeloSearch struct {
	Data struct {
		Items []mojedeloItem `json:"items"`
		Total int            `json:"total"`
	} `json:"data"`
}

// mojedeloJob is the part of a posting's own resource the adapter reads.
type mojedeloJob struct {
	Data struct {
		ID                  string          `json:"id"`
		Status              string          `json:"status"`
		JobDescription      string          `json:"jobDescription"`
		WeExpect            string          `json:"weExpect"`
		WeOffer             string          `json:"weOffer"`
		AboutTheCompany     string          `json:"aboutTheCompany"`
		AdSummary           string          `json:"adSummary"`
		Salary              *mojedeloNamed  `json:"salary"`
		WorkTime            *mojedeloNamed  `json:"workTime"`
		EmploymentTypes     []mojedeloNamed `json:"employmentTypes"`
		TotalWorkExperience *mojedeloNamed  `json:"totalWorkExperience"`
	} `json:"data"`
}

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s mojedelo) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s mojedelo) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	return s.FetchNewGated(ctx, e, seen, DetailGate{})
}

// FetchNewGated walks the whole search, refreshes the postings already stored, and reads up to
// maxNewPerRun of the rest. A posting the pipeline will discard anyway is judged on its row and
// yielded body-less without a request; one whose employer is hidden is dropped before any. A
// posting whose resource cannot be read is not yielded; a later run tries again.
func (s mojedelo) FetchNewGated(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error) {
	items, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var covered map[string]bool
	if gate.Covered != nil {
		companies := make([]string, 0, len(items))
		for _, it := range items {
			companies = append(companies, mojedeloClean(it.Company.Name))
		}
		covered = gate.Covered(companies)
	}
	var jobs []Job
	var unseen []mojedeloItem
	deferred := 0
	for _, it := range items {
		base := mojedeloBaseJob(it)
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
		unseen = append(unseen, it)
	}
	var answered, outOfTime atomic.Int64
	read := fetchDetails(unseen, mojedeloDetailWorkers, func(it mojedeloItem) (Job, bool) {
		if s.nearDeadline(ctx) {
			outOfTime.Add(1)
			return Job{}, false
		}
		job, ok, err := s.detail(ctx, it)
		if err != nil {
			log.Printf("mojedelo: posting %s failed; deferring: %v", it.ID, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, ok
	})
	// A crawl that read no posting at all is broken, not quiet. "Answered" counts a posting the
	// site reported gone; postings left for want of time are not failures.
	if int64(len(unseen)) > outOfTime.Load() && answered.Load() == 0 {
		return nil, fmt.Errorf("mojedelo: requested %d postings and none answered", int64(len(unseen))-outOfTime.Load())
	}
	if n := outOfTime.Load(); n > 0 {
		log.Printf("mojedelo: %d new postings left unread as the run neared its deadline, deferred to later runs", n)
	}
	if deferred > 0 {
		log.Printf("mojedelo: %d new postings past this run's bound of %d, deferred to later runs", deferred, s.maxNewPerRun)
	}
	return append(jobs, read...), nil
}

// nearDeadline reports whether the run's deadline is closer than the margin hydration leaves to
// the pipeline's own writes: a hydrating crawl writes nothing until FetchNewGated returns, so one
// that runs into its deadline stores nothing at all.
func (s mojedelo) nearDeadline(ctx context.Context) bool {
	d, ok := ctx.Deadline()
	return ok && time.Until(d) < s.deadlineMargin
}

// list walks the search to the total it states and returns the postings that name their
// employer, in listing order. Any page failing fails the crawl: a partial listing would let the
// company-scoped sweep close the postings of the employers it did see on the pages it did not.
func (s mojedelo) list(ctx context.Context) ([]mojedeloItem, error) {
	seen := make(map[string]bool)
	var out []mojedeloItem
	read := 0
	for page := 0; ; page++ {
		if page == mojedeloMaxPages {
			return nil, fmt.Errorf("mojedelo: search runs past %d pages", mojedeloMaxPages)
		}
		if err := s.limiter.Wait(ctx); err != nil {
			return nil, err
		}
		var res mojedeloSearch
		if err := s.http.GetJSONWithHeaders(ctx, fmt.Sprintf(mojedeloSearchURL, mojedeloPageSize, page*mojedeloPageSize), mojedeloHeaders, &res); err != nil {
			return nil, fmt.Errorf("mojedelo: search page %d: %w", page+1, err)
		}
		if len(res.Data.Items) == 0 {
			// A posting withdrawn mid-walk shrinks the total; the walk ends where the rows do.
			break
		}
		read += len(res.Data.Items)
		for _, it := range res.Data.Items {
			id := strings.TrimSpace(it.ID)
			if id == "" || seen[id] || mojedeloClean(it.Title) == "" || mojedeloHiddenEmployer(it) {
				continue
			}
			seen[id] = true
			out = append(out, it)
		}
		if read >= res.Data.Total {
			break
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("mojedelo: search listed no posting")
	}
	return out, nil
}

// mojedeloHiddenEmployer reports whether the row names no employer of its own. An anonymous
// posting names the site's operator (Styria digital marketplaces) in place of the employer and
// marks itself with anonymizedCompany; filing it under that name would invent one employer
// collecting every anonymous posting.
func mojedeloHiddenEmployer(it mojedeloItem) bool {
	return it.AnonymizedCompany != nil || it.Company == nil || mojedeloClean(it.Company.Name) == ""
}

func mojedeloClean(s string) string { return strings.Join(strings.Fields(s), " ") }

// mojedeloBaseJob is the body-less Job a search row alone supports: the identity, and the facts
// the gate judges on.
func mojedeloBaseJob(it mojedeloItem) Job {
	id := strings.TrimSpace(it.ID)
	title := mojedeloClean(it.Title)
	j := Job{
		ExternalID: id,
		URL:        fmt.Sprintf(mojedeloPageURL, normalize.Slug(title), id),
		Title:      title,
		Company:    mojedeloClean(it.Company.Name),
		WorkMode:   mojedeloWorkMode(it.WorkModes),
	}
	j.Remote = j.WorkMode == "remote"
	// The town is the place; a posting without one (most abroad) states it in free text.
	if it.Town != nil {
		j.Location = mojedeloClean(it.Town.Translation)
	}
	if j.Location == "" {
		j.Location = mojedeloClean(it.JobLocationInput)
	}
	if it.Country != nil {
		if code := location.NormalizeCountry(strings.TrimSpace(it.Country.CountryCode)); code != "" {
			j.Countries = []string{code}
		}
	}
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(it.StartDate)); err == nil {
		j.PostedAt = NotFuture(&t)
	}
	return j
}

// mojedeloWorkMode maps the row's arrangement. A posting may tick several at once ("Od doma",
// "Hibridno", "Na delovnem mestu" together), which states nothing, so only a single one is read.
// "Na terenu" (fieldwork) has no counterpart.
func mojedeloWorkMode(modes []mojedeloNamed) string {
	if len(modes) != 1 {
		return ""
	}
	switch modes[0].Translation {
	case "Od doma":
		return "remote"
	case "Hibridno (delno od doma)":
		return "hybrid"
	case "Na delovnem mestu":
		return "onsite"
	}
	return ""
}

// detail reads one posting's resource. ok=false with a nil error is the site's answer that the
// posting is gone (404/410, or no longer published); an error is the absence of an answer.
func (s mojedelo) detail(ctx context.Context, it mojedeloItem) (Job, bool, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return Job{}, false, err
	}
	var res mojedeloJob
	if err := s.http.GetJSONWithHeaders(ctx, fmt.Sprintf(mojedeloJobURL, strings.TrimSpace(it.ID)), mojedeloHeaders, &res); err != nil {
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	d := res.Data
	if d.ID == "" {
		return Job{}, false, fmt.Errorf("posting resource carries no posting")
	}
	if d.Status != "published" {
		return Job{}, false, nil
	}
	j := mojedeloBaseJob(it)
	j.Description = mojedeloBody(d.JobDescription, d.WeExpect, d.WeOffer, d.AboutTheCompany, d.AdSummary)
	if j.Description == "" {
		return Job{}, false, fmt.Errorf("posting carries no text")
	}
	j.EmploymentType = mojedeloEmploymentType(d.WorkTime, d.EmploymentTypes)
	if d.TotalWorkExperience != nil {
		if years := mojedeloYears(d.TotalWorkExperience.Name); years > 0 {
			j.ExperienceYearsMin = &years
		}
	}
	if d.Salary != nil {
		if lo, hi, ok := mojedeloSalary(d.Salary.Name); ok {
			j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod = &lo, &hi, "EUR", "month"
		}
	}
	return j, true, nil
}

// mojedeloBody renders the posting's sections, in the order its page shows them: the duties,
// what the employer expects and offers, and the posting's own paragraph about the employer
// ("O podjetju"). Each is HTML. A posting with none of them falls back to its summary.
func mojedeloBody(duties, expect, offer, about, summary string) string {
	var b strings.Builder
	section := func(heading, body string) {
		if strings.TrimSpace(sanitizeHTML(body)) == "" {
			return
		}
		if heading != "" {
			b.WriteString("<h3>" + heading + "</h3>")
		}
		b.WriteString(body)
	}
	section("", duties)
	section("Pričakujemo", expect)
	section("Nudimo", offer)
	section("O podjetju", about)
	if b.Len() == 0 && strings.TrimSpace(summary) != "" {
		b.WriteString("<p>" + strings.ReplaceAll(strings.TrimSpace(summary), "\n", "<br>") + "</p>")
	}
	return strings.TrimSpace(sanitizeHTML(b.String()))
}

// mojedeloEmploymentType reads the contract and the working time. A contract ("Pogodbeno delo")
// or a traineeship ("Praksa") stated alone is the answer; otherwise the working time decides.
// Student work ("Študentsko delo") is a student-service arrangement, not an internship, and
// sets nothing on its own.
func mojedeloEmploymentType(workTime *mojedeloNamed, types []mojedeloNamed) string {
	if len(types) == 1 {
		switch types[0].Name {
		case "Contract work":
			return "contract"
		case "Practise":
			return "internship"
		}
	}
	if workTime != nil {
		switch workTime.Name {
		case "Full time":
			return "full_time"
		case "Part time":
			return "part_time"
		}
	}
	return ""
}

// mojedeloYearsRe reads the experience band's lower bound: "1+ to 3 years", "5+ to 10 years".
// "0 to 1 year" and "Not important" state no requirement.
var mojedeloYearsRe = regexp.MustCompile(`^(\d+)\+? to \d+ years?$`)

func mojedeloYears(band string) int {
	m := mojedeloYearsRe.FindStringSubmatch(strings.TrimSpace(band))
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

// mojedeloSalaryRe reads the salary band, "1501 - 2000 EUR Gross": a gross monthly band, the
// way pay is stated in Slovenia. "negotiable" states nothing.
var mojedeloSalaryRe = regexp.MustCompile(`^(\d+) - (\d+) EUR Gross$`)

func mojedeloSalary(band string) (lo, hi int, ok bool) {
	m := mojedeloSalaryRe.FindStringSubmatch(strings.TrimSpace(band))
	if m == nil {
		return 0, 0, false
	}
	lo, _ = strconv.Atoi(m[1])
	hi, _ = strconv.Atoi(m[2])
	if lo <= 0 || hi < lo {
		return 0, 0, false
	}
	return lo, hi, true
}
