package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// vietnamworks adapts VietnamWorks (vietnamworks.com), Vietnam's largest job board. It is
// boardless and an aggregator — the employer is read per posting. See providers/vietnamworks.md
// for the measurements behind each rule.
//
// The listing is the search API the site's own frontend calls (ms.vietnamworks.com), read page by
// page with an empty query so it returns the whole open catalogue, every profession. A row
// carries every fact except the body, which the search truncates to a few hundred characters; the
// full body is read from the posting's page, where Next.js inlines it in the RSC flight stream.
// The adapter hydrates postings due for a body read and refreshes the rest by identity.
//
// Plain HTTP from the crawl host is served: no proxy and no Scrapling tier.
type vietnamworks struct {
	http         vietnamworksHTTP
	limiter      waiter
	maxNewPerRun int
	// now dates the backlog window; nil is time.Now.
	now func() time.Time
}

// vietnamworksHTTP is the transport the adapter needs: the search API and each posting page.
type vietnamworksHTTP interface {
	HTMLGetter
	PostJSONWithHeaders(ctx context.Context, url string, headers map[string]string, body, v any) error
}

const (
	vietnamworksSearchURL = "https://ms.vietnamworks.com/job-search/v1.0/search"
	// vietnamworksPageSize is the most rows a search page returns: a larger hitsPerPage is
	// silently cut back to 100.
	vietnamworksPageSize = 100
	// vietnamworksMaxPages bounds the listing against a response that claims an absurd page
	// count; the catalogue was ~90 pages (8.9k postings) when measured.
	vietnamworksMaxPages = 400
	// vietnamworksPageAttempts is how often one search page is asked for before the crawl fails.
	vietnamworksPageAttempts = 3
	// vietnamworksRequestInterval paces every request of a run, search page and posting page
	// alike, through one limiter. A posting page takes ~1.4 s and 30 in a row drew no refusal.
	vietnamworksRequestInterval = 500 * time.Millisecond
	vietnamworksWorkers         = 4
	// vietnamworksMaxNewPerRun bounds how many unseen postings one run hydrates: 3000 pages are
	// ~25 minutes at the pace above. The first runs face the whole ~9k catalogue, taken in
	// slices; what is hydrated leaves the unseen set. The backlog lane (see hydrationLanes) is
	// only for pages that failed to read.
	vietnamworksMaxNewPerRun = 3000
	vietnamworksBacklogLane  = 500
	// vietnamworksITGroup is the id of the site's own top-level function group "Information
	// Technology/Telecommunications", the one structured statement of IT-ness a posting carries.
	vietnamworksITGroup = 5
	// vietnamworksFullTime is the typeWorkingId of a full-time posting (383 of 400 sampled). The
	// other ids (5, 6) are unnamed by the API and are left unmapped.
	vietnamworksFullTime = 1
	// vietnamworksMonthly is the salaryPeriodId of a per-month salary, the only period sampled
	// on a posting that shows its salary.
	vietnamworksMonthly = 1
)

// vietnamworksFields is the retrieveFields the listing asks for. The API answers every field it
// is not asked for with an empty value, so a missing name here reads as a missing fact.
var vietnamworksFields = []string{
	"jobId", "jobTitle", "jobUrl", "companyName", "isAnonymous",
	"approvedOn", "expiredOn", "workingLocations", "typeWorkingId",
	"groupJobFunctionsV3", "isSalaryVisible", "salaryMin", "salaryMax",
	"salaryCurrency", "salaryPeriodId",
}

// NewVietnamworks builds the adapter over the given client.
func NewVietnamworks(c vietnamworksHTTP) Source {
	return vietnamworks{
		http:         c,
		limiter:      rate.NewLimiter(rate.Every(vietnamworksRequestInterval), 1),
		maxNewPerRun: vietnamworksMaxNewPerRun,
	}
}

func (vietnamworks) Provider() string { return "vietnamworks" }

// Boardless marks vietnamworks as having no per-tenant board id: one crawl covers the whole site.
func (vietnamworks) Boardless() {}

// Aggregator keeps vietnamworks in the source facet: one crawl aggregates postings from many
// companies, so filtering by source=vietnamworks is meaningful.
func (vietnamworks) Aggregator() {}

// vietnamworksRequest is the search body. The three empty lists must go out as [], not null.
type vietnamworksRequest struct {
	UserID         int      `json:"userId"`
	Query          string   `json:"query"`
	Filter         []any    `json:"filter"`
	Ranges         []any    `json:"ranges"`
	Order          []any    `json:"order"`
	HitsPerPage    int      `json:"hitsPerPage"`
	Page           int      `json:"page"`
	RetrieveFields []string `json:"retrieveFields"`
}

type vietnamworksResponse struct {
	Meta struct {
		Code     int `json:"code"`
		NbHits   int `json:"nbHits"`
		NbPages  int `json:"nbPages"`
		PageSize int `json:"hitsPerPage"`
	} `json:"meta"`
	Data []vietnamworksRow `json:"data"`
}

type vietnamworksRow struct {
	JobID         int64  `json:"jobId"`
	Title         string `json:"jobTitle"`
	URL           string `json:"jobUrl"`
	Company       string `json:"companyName"`
	IsAnonymous   bool   `json:"isAnonymous"`
	ApprovedOn    string `json:"approvedOn"`
	ExpiredOn     string `json:"expiredOn"`
	TypeWorkingID int    `json:"typeWorkingId"`
	Group         struct {
		ID int `json:"groupJobFunctionV3Id"`
	} `json:"groupJobFunctionsV3"`
	Locations []struct {
		City string `json:"cityName"`
	} `json:"workingLocations"`
	SalaryVisible bool   `json:"isSalaryVisible"`
	SalaryMin     int    `json:"salaryMin"`
	SalaryMax     int    `json:"salaryMax"`
	SalaryCurr    string `json:"salaryCurrency"`
	SalaryPeriod  int    `json:"salaryPeriodId"`
}

// Fetch hydrates every listed posting: the path for callers that supply no seen set. FetchNew is
// the one ingest uses.
func (v vietnamworks) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return v.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew reads the whole listing and refreshes seen postings by identity. Postings that need
// a body are hydrated up to the per-run budget. Failed reads preserve the listing's liveness
// evidence while the body remains due for retry.
func (v vietnamworks) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	rows, err := v.listing(ctx)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	var fresh []vietnamworksRow
	for _, r := range rows {
		id := strconv.FormatInt(r.JobID, 10)
		if seen(id) {
			jobs = append(jobs, Job{ExternalID: id, URL: r.URL, Title: r.Title, Company: r.Company, SeenRefresh: true})
		} else {
			fresh = append(fresh, r)
		}
	}
	now := time.Now
	if v.now != nil {
		now = v.now
	}
	budget := v.maxNewPerRun
	if budget <= 0 {
		budget = vietnamworksMaxNewPerRun
	}
	backlog := min(vietnamworksBacklogLane, budget)
	queue := hydrationLanes(fresh, budget-backlog, backlog, now())
	if deferred := len(fresh) - len(queue); deferred > 0 {
		log.Printf("vietnamworks: hydrating %d of %d new postings; %d deferred to a later run", len(queue), len(fresh), deferred)
	}
	var answered atomic.Int64
	hydrated := fetchDetails(queue, vietnamworksWorkers, func(r vietnamworksRow) (Job, bool) {
		job, ok, err := v.detail(ctx, r)
		if err != nil {
			log.Printf("vietnamworks: posting %d failed; deferring: %v", r.JobID, err)
			return unreadableDetail(strconv.FormatInt(r.JobID, 10), r.URL, r.Company), true
		}
		answered.Add(1)
		return job, ok
	})
	// A successful detail or a confirmed closure counts as an answer.
	if len(queue) > 0 && answered.Load() == 0 {
		return nil, fmt.Errorf("vietnamworks: attempted to read %d postings and none answered", len(queue))
	}
	return append(jobs, hydrated...), nil
}

// listing reads every search page and returns the postings worth hydrating, one per id. The
// ranking shuffles between requests, so a page can repeat a row another page already gave (~3%
// of a 500-row sample) and an occasional posting is missed for one run; the unseen sweep's
// grace window absorbs that. A page that cannot be read fails the crawl.
func (v vietnamworks) listing(ctx context.Context) ([]vietnamworksRow, error) {
	var rows []vietnamworksRow
	ids := map[int64]bool{}
	for page := 0; ; page++ {
		resp, err := v.searchPage(ctx, page)
		if err != nil {
			return nil, err
		}
		for _, r := range resp.Data {
			// A posting posted under the site's placeholder employer would pool unrelated
			// companies under one name, and one without a URL cannot be fetched.
			if r.JobID == 0 || ids[r.JobID] || r.IsAnonymous || r.URL == "" ||
				strings.TrimSpace(r.Title) == "" || strings.TrimSpace(r.Company) == "" {
				continue
			}
			ids[r.JobID] = true
			rows = append(rows, r)
		}
		if page+1 >= min(resp.Meta.NbPages, vietnamworksMaxPages) {
			break
		}
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("vietnamworks: the search listed no usable posting")
	}
	log.Printf("vietnamworks: %d postings listed", len(rows))
	return rows, nil
}

// searchPage asks for one search page through the limiter, up to vietnamworksPageAttempts times.
func (v vietnamworks) searchPage(ctx context.Context, page int) (vietnamworksResponse, error) {
	body := vietnamworksRequest{
		Filter:         []any{},
		Ranges:         []any{},
		Order:          []any{},
		HitsPerPage:    vietnamworksPageSize,
		Page:           page,
		RetrieveFields: vietnamworksFields,
	}
	headers := map[string]string{"Origin": "https://www.vietnamworks.com"}
	var err error
	for range vietnamworksPageAttempts {
		if err = v.limiter.Wait(ctx); err != nil {
			return vietnamworksResponse{}, err
		}
		var resp vietnamworksResponse
		if err = v.http.PostJSONWithHeaders(ctx, vietnamworksSearchURL, headers, body, &resp); err == nil {
			if resp.Meta.Code != 200 {
				err = fmt.Errorf("answered code %d", resp.Meta.Code)
				continue
			}
			return resp, nil
		}
	}
	return vietnamworksResponse{}, fmt.Errorf("vietnamworks: search page %d: %w", page, err)
}

// detail reads one posting's body and builds its Job. ok=false with a nil error is the site's
// answer that the posting is gone (404/410). An unreadable posting yields an error.
func (v vietnamworks) detail(ctx context.Context, r vietnamworksRow) (Job, bool, error) {
	if err := v.limiter.Wait(ctx); err != nil {
		return Job{}, false, err
	}
	flight, err := fetchFlight(ctx, v.http, r.URL)
	if err != nil {
		if !detailUnreadable(err) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	description := vietnamworksBody(flight, r.JobID)
	if strings.TrimSpace(textFromHTML(description)) == "" {
		return Job{}, false, errors.New("page carries no posting body")
	}
	return r.job(description), true, nil
}

// vietnamworksBody assembles the posting's description and requirements from the flight stream.
// Each is either inline or a "$<hex>" reference to a text row; both are read from the posting's
// own record, found by its id, since the page also carries other postings' records.
func vietnamworksBody(flight string, id int64) string {
	record, ok := vietnamworksFlightRecord(flight, id)
	if !ok {
		return ""
	}
	rows := nextFlightTextRows(flight)
	var parts []string
	for _, f := range []struct{ value, heading string }{
		{record.Description, "Mô tả công việc"},
		{record.Requirement, "Yêu cầu công việc"},
	} {
		text := f.value
		if vietnamworksTextRef.MatchString(text) {
			text, ok = rows[text[1:]]
			if !ok {
				return ""
			}
		}
		if strings.TrimSpace(textFromHTML(text)) == "" {
			continue
		}
		parts = append(parts, "<h2>"+f.heading+"</h2>"+text)
	}
	return sanitizeHTML(strings.Join(parts, ""))
}

var vietnamworksTextRef = regexp.MustCompile(`^\$[0-9a-f]+$`)

type vietnamworksDetailRecord struct {
	ID          int64  `json:"jobId"`
	Description string `json:"jobDescription"`
	Requirement string `json:"jobRequirement"`
}

// vietnamworksFlightRecord finds the model row for id. Text rows have byte lengths and may
// contain newlines or run directly into the next row. Other rows end at a newline.
func vietnamworksFlightRecord(flight string, id int64) (vietnamworksDetailRecord, bool) {
	for flight != "" {
		_, payload, ok := strings.Cut(flight, ":")
		if !ok {
			break
		}
		if row, isText := strings.CutPrefix(payload, "T"); isText {
			length, text, found := strings.Cut(row, ",")
			n, err := strconv.ParseUint(length, 16, 64)
			if !found || err != nil || n > uint64(len(text)) {
				break
			}
			flight = text[n:]
			continue
		}
		var line string
		line, flight, _ = strings.Cut(payload, "\n")
		if strings.HasPrefix(line, "{") && strings.Contains(line, `"jobId"`) {
			var record vietnamworksDetailRecord
			if json.Unmarshal([]byte(line), &record) == nil && record.ID == id {
				return record, true
			}
		}
	}
	return vietnamworksDetailRecord{}, false
}

func (r vietnamworksRow) job(description string) Job {
	var cities []string
	for _, l := range r.Locations {
		cities = append(cities, strings.TrimSpace(l.City))
	}
	job := Job{
		ExternalID:          strconv.FormatInt(r.JobID, 10),
		URL:                 r.URL,
		Title:               strings.Join(strings.Fields(r.Title), " "),
		Company:             strings.Join(strings.Fields(r.Company), " "),
		Location:            distinctJoin(append(cities, "Vietnam"), ", ", func(s string) string { return s }),
		Description:         description,
		PostedAt:            NotFuture(parseRFC3339(r.ApprovedOn)),
		ApplicationDeadline: vietnamworksDeadline(r.ExpiredOn),
		Countries:           countryFromCode("VN"),
		IsTechHint:          r.Group.ID == vietnamworksITGroup,
	}
	if r.TypeWorkingID == vietnamworksFullTime {
		job.EmploymentType = "full_time"
	}
	if r.SalaryVisible && r.SalaryPeriod == vietnamworksMonthly && r.SalaryCurr != "" &&
		(r.SalaryMin > 0 || r.SalaryMax > 0) {
		if r.SalaryMin > 0 {
			job.SalaryMin = &r.SalaryMin
		}
		if r.SalaryMax > 0 {
			job.SalaryMax = &r.SalaryMax
		}
		job.SalaryCurrency, job.SalaryPeriod = r.SalaryCurr, "month"
	}
	return job
}

// vietnamworksDeadline reads the posting's expiry as the calendar day the site states it in
// (Vietnam time, 23:59:59), as a UTC day. It is not passed through NotFuture, which is for a
// posted_at and would drop every deadline more than two days away.
func vietnamworksDeadline(s string) *time.Time {
	if len(s) < len("2006-01-02") {
		return nil
	}
	t, err := time.Parse("2006-01-02", s[:len("2006-01-02")])
	if err != nil {
		return nil
	}
	return &t
}
