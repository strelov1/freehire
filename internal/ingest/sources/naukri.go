package sources

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// naukri adapts Naukri (naukri.com), India's largest general job board, whose postings each
// name their own employer. Board-based aggregator: a board is one of the platform's FUNCTIONAL
// AREAS (functionAreaIdGid — "5" Engineering - Software & QA, "8" IT & Information Security,
// "3" Data Science & Analytics), which is what keeps a general board on its technical slice.
//
// The listing is the search API Naukri's own Android app calls; the website's identical
// endpoint answers "recaptcha required". A search row carries only a ~300-character snippet,
// so this is a HydratingSource that buys a posting's full body from the posting API only when
// the catalogue lacks it, newest first, up to naukriDetailBudget a run.
//
// The walk covers the last naukriJobAge days and stops at the platform's deepest page, so a
// busy area is truncated to its newest naukriMaxPages*naukriPageSize postings and a posting
// that merely aged out of the window reads as unseen — hence the wide sweep grace and no
// fullBoardListing. See the Naukri traps in AGENTS.md.
type naukri struct {
	http HeaderJSONGetter
	// spent counts the detail requests claimed across every board of the run, shared by the
	// adapter's copies (boards may be crawled concurrently, hence atomic).
	spent *atomic.Int64
	// refusals counts the run's posting-API refusals (naukriRefusal). At naukriRefusalLimit the
	// egress is taken to be refused rather than one exit unlucky, and hydration ends for every
	// board of the run.
	refusals *atomic.Int64
}

const (
	naukriBase      = "https://www.naukri.com"
	naukriSearchURL = naukriBase + "/jobapi/v3/search?"
	naukriDetailURL = naukriBase + "/jobapi/v4/job/%s?microsite=y"

	// naukriUserAgent is the Android app's HTTP stack. The edge (Akamai) answers 403 Access
	// Denied to our own string, "recaptcha required" to a browser string, and serves the app's
	// API to this one alone (measured 2026-10-18). robots.txt disallows nothing under /jobapi/.
	naukriUserAgent = "okhttp/4.9.0"

	// naukriPageSize is the most rows a search page serves (noOfResults=100 answered 100).
	naukriPageSize = 100
	// naukriMaxPages is the platform's own deepest page: page 150 answers, page 151 answers
	// 400 "Invalid value" at any page size, so no query reaches past 15,000 rows.
	naukriMaxPages = 150
	// naukriJobAge is the freshness window in days (jobAge). Area 5 held 18,920 postings in
	// 7 days and 56,950 in 30 on 2026-10-18; 7 keeps every area but 5 inside the page window
	// and area 5's truncation to its newest 15,000.
	naukriJobAge = 7
	// naukriSweepGrace absorbs the window: a posting not refreshed by its employer leaves the
	// 7-day search while it is still live, and the posting API is not a liveness probe the
	// sweep can ask. Two weeks past last sight keeps a live posting open; an expired one
	// lingers that long at most.
	naukriSweepGrace = 14 * 24 * time.Hour
)

// The posting API served 150 requests at four concurrent (~4 req/s) through the shared proxy
// without a refusal (2026-10-18). Half that rate leaves room for the proxy's other providers.
// A cold start is ~23k postings across the three technical areas, so a RUN — every board
// cmd/ingest crawls in one process — hydrates at most naukriDetailBudget (~12.5 minutes at the
// pace above) and the backlog accretes over the next runs; steady state is a few hundred new
// postings a run.
//
// One board may spend at most naukriBoardDetailShare of it, a third for the three technical
// areas, so area 5's 15k cold backlog cannot take the whole run while 8 and 3 wait. A fourth
// board gets only what the others leave, which during a cold start is nothing; the shares drain
// each saturated backlog at 500 a run (area 5's in ~30 runs), after which every board's new
// postings fit and nothing is starved for good.
const (
	naukriRequestInterval  = 500 * time.Millisecond
	naukriRequestBurst     = 2
	naukriDetailWorkers    = 4
	naukriDetailBudget     = 1500
	naukriBoardDetailShare = naukriDetailBudget / 3
)

// naukriAppHeaders identify the Android app. Without them the API answers 400 "Please
// provide the valid App Id and System Id".
var naukriAppHeaders = map[string]string{"appid": "105", "systemid": "105"}

// naukriTechAreas are the functional areas that are technical by the platform's own filing,
// so their postings set IsTechHint. classify.IsTech accepts only 80 of 300 such titles
// ("Tcs is hiring For Swift, Objective C_Bangalore") while confirming none non-technical.
// Area 4 (Engineering - Hardware & Networks) mixes in hardware roles and is not listed.
var naukriTechAreas = map[string]bool{"3": true, "5": true, "8": true}

var naukriBoardRE = regexp.MustCompile(`^[0-9]+$`)

// NewNaukri builds the Naukri adapter over the given HTTP client. The registry builds one
// adapter per process and cmd/ingest crawls one provider per process, so the adapter's detail
// counter is the run's.
func NewNaukri(c HeaderJSONGetter) Source {
	return naukri{http: c, spent: new(atomic.Int64), refusals: new(atomic.Int64)}
}

// pacedNaukriGetter holds every request of a run — search and posting — to one limiter.
func pacedNaukriGetter(c HeaderJSONGetter) HeaderJSONGetter {
	return rateLimitedHeaderJSONGetter{
		inner:   c,
		limiter: rate.NewLimiter(rate.Every(naukriRequestInterval), naukriRequestBurst),
	}
}

func (naukri) Provider() string { return "naukri" }

// Every posting names its own employer, so naukri stays in the source facet and in the
// cross-source duplicate suppression set.
func (naukri) aggregator() {}

func (naukri) sweepGrace() time.Duration { return naukriSweepGrace }

// naukriListing is one search row, reduced to what the adapter reads.
type naukriListing struct {
	JobID         string `json:"jobId"`
	Title         string `json:"title"`
	CompanyName   string `json:"companyName"`
	JDURL         string `json:"jdURL"`
	CreatedDate   int64  `json:"createdDate"`
	MinExperience string `json:"minimumExperience"`
	Placeholders  []struct {
		Type  string `json:"type"`
		Label string `json:"label"`
	} `json:"placeholders"`
	SalaryDetail struct {
		Minimum    int64  `json:"minimumSalary"`
		Maximum    int64  `json:"maximumSalary"`
		Currency   string `json:"currency"`
		HideSalary bool   `json:"hideSalary"`
	} `json:"salaryDetail"`
}

type naukriSearchPage struct {
	NoOfJobs   int             `json:"noOfJobs"`
	JobDetails []naukriListing `json:"jobDetails"`
}

// naukriDetail is the posting API, reduced to what the search row lacks.
type naukriDetail struct {
	JobDetails struct {
		JobID          string `json:"jobId"`
		Description    string `json:"description"`
		WFHType        string `json:"wfhType"`
		JobType        string `json:"jobType"`
		EmploymentType string `json:"employmentType"`
		Locations      []struct {
			Label string `json:"label"`
		} `json:"locations"`
	} `json:"jobDetails"`
}

func (s naukri) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s naukri) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	return s.FetchNewGated(ctx, e, seen, DetailGate{})
}

// FetchNewGated is FetchNew that buys no body for a posting the pipeline will discard anyway.
// Without it the budget went to exactly those postings: a discarded posting is never stored, so
// it stays unseen, and newest-first hydration bought the same covered postings every run —
// measured 2026-10-25, every one of a board's 500 details a run was skipped as "company already
// covered", and the catalogue sat at ~4k open postings for four days.
func (s naukri) FetchNewGated(ctx context.Context, e CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error) {
	if !naukriBoardRE.MatchString(e.Board) {
		return nil, fmt.Errorf("naukri: board %q is not a functional area id", e.Board)
	}
	rows, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	return s.hydrate(ctx, rows, naukriTechAreas[e.Board], seen, gate)
}

// list walks the area's freshness window newest first. The first page failing fails the board;
// a later page failing ends the walk with what was read. The walk ends at the declared total, a
// short page, or the platform's deepest page — a truncated area is logged, not failed, since
// the window is a property of the platform rather than of this crawl.
func (s naukri) list(ctx context.Context, area string) ([]naukriListing, error) {
	var out []naukriListing
	ids := map[string]bool{}
	total := 0
	for page := 1; page <= naukriMaxPages; page++ {
		q := url.Values{}
		q.Set("noOfResults", strconv.Itoa(naukriPageSize))
		q.Set("urlType", "search_by_keyword")
		q.Set("searchType", "adv")
		q.Set("functionAreaIdGid", area)
		q.Set("jobAge", strconv.Itoa(naukriJobAge))
		q.Set("sort", "f") // freshness: newest first, so the detail budget buys the newest
		q.Set("pageNo", strconv.Itoa(page))
		var p naukriSearchPage
		if err := s.http.GetJSONWithHeaders(ctx, naukriSearchURL+q.Encode(), naukriAppHeaders, &p); err != nil {
			if page == 1 {
				return nil, fmt.Errorf("naukri: area %s: %w", area, err)
			}
			log.Printf("naukri: area %s: page %d failed; keeping %d postings read: %v", area, page, len(out), err)
			break
		}
		if page == 1 {
			total = p.NoOfJobs
		}
		for _, r := range p.JobDetails {
			if r.JobID == "" || ids[r.JobID] {
				continue
			}
			ids[r.JobID] = true
			out = append(out, r)
		}
		if len(p.JobDetails) == 0 || page*naukriPageSize >= total {
			break
		}
		if page == naukriMaxPages {
			log.Printf("naukri: area %s: %d postings in the window, the platform serves the newest %d",
				area, total, naukriMaxPages*naukriPageSize)
		}
	}
	return out, nil
}

// hydrate maps the listed rows to Jobs. A seen posting is a liveness refresh with no request;
// every other posting costs one detail request, up to the board's share of the run's budget
// (claimDetail). A detail that could not be read, or that the budget did not reach, defers its
// posting to a later run rather than storing it body-less, which would never be hydrated again.
func (s naukri) hydrate(ctx context.Context, rows []naukriListing, tech bool, seen func(string) bool, gate DetailGate) ([]Job, error) {
	var covered map[string]bool
	if gate.Covered != nil {
		companies := make([]string, 0, len(rows))
		for _, r := range rows {
			companies = append(companies, strings.TrimSpace(r.CompanyName))
		}
		covered = gate.Covered(companies)
	}
	var board, asked, answered, deferred, refusedHere atomic.Int64
	jobs := fetchDetails(rows, naukriDetailWorkers, func(r naukriListing) (Job, bool) {
		base, ok := r.toJob(tech)
		if !ok {
			return Job{}, false
		}
		if seen(base.ExternalID) {
			base.SeenRefresh = true
			return base, true
		}
		// A posting the pipeline will discard is yielded body-less, for it to count.
		if covered[base.Company] || (gate.Rejected != nil && gate.Rejected(base)) {
			return base, true
		}
		if s.refused() || !s.claimDetail(&board) {
			deferred.Add(1)
			return Job{}, false
		}
		asked.Add(1)
		job, ok, err := s.withDetail(ctx, base)
		if err != nil {
			if naukriRefusal(err) {
				refusedHere.Add(1)
				if s.refusals.Add(1) == naukriRefusalLimit {
					log.Printf("naukri: posting API refused %d requests (last: %v); no more details this run", naukriRefusalLimit, err)
				}
				return Job{}, false
			}
			log.Printf("naukri: detail %s failed; deferring: %v", base.ExternalID, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, ok
	})
	// A refused run — or a board whose every answer was a refusal — fails each board that yielded
	// nothing, naming the refusal: from the outside
	// it is the same outage as "none answered", but the fix is an egress, not the adapter. A
	// board that still refreshed stored postings keeps them.
	if s.refused() || (refusedHere.Load() > 0 && answered.Load() == 0) {
		pending := deferred.Load() + asked.Load() - answered.Load()
		if len(jobs) == 0 && pending > 0 {
			return nil, fmt.Errorf("naukri: posting API refused the egress IP (HTTP 406 \"recaptcha required\"); %d new postings deferred", pending)
		}
		return jobs, nil
	}
	// A crawl that asked for details and got no answer to any of them is broken, not quiet.
	// "Answered" counts a posting the platform reported expired.
	if asked.Load() > 0 && answered.Load() == 0 {
		return nil, fmt.Errorf("naukri: requested %d posting details and none answered", asked.Load())
	}
	if deferred.Load() > 0 {
		log.Printf("naukri: detail budget reached (%d a board, %d a run); %d new postings deferred to later runs",
			naukriBoardDetailShare, naukriDetailBudget, deferred.Load())
	}
	return jobs, nil
}

// claimDetail reserves one detail request against the board's share and then the run's budget,
// and reports whether both had room. A refused claim leaves the counters past their limit,
// which only keeps later claims refused.
func (s naukri) claimDetail(board *atomic.Int64) bool {
	if board.Add(1) > naukriBoardDetailShare {
		return false
	}
	return s.spent.Add(1) <= naukriDetailBudget
}

// naukriRefusalLimit is how many refusals end a run's hydration. Through the fixed proxy exit
// the first refusal was the IP's verdict and every later request was refused too (2026-10-19);
// through the rotating gateway each request leaves from a different exit, and a refusal is one
// bad exit: on 2026-10-25 two of four runs met a single 406 that ended the whole run's
// hydration, one of them after 30 seconds. Ten refusals is still nothing beside a run's budget
// when the egress really is refused, and no longer a coin toss when it is not.
const naukriRefusalLimit = 10

// refused reports whether the run has met naukriRefusalLimit refusals.
func (s naukri) refused() bool { return s.refusals.Load() >= naukriRefusalLimit }

// naukriRefusal reports whether a posting-API error is the edge refusing the client rather than
// failing one request: 406 is its "recaptcha required" answer, 403 its Access Denied, 429 a rate
// limit. A refused posting is deferred, never stored body-less; naukriRefusalLimit of them end
// the run's hydration.
func naukriRefusal(err error) bool {
	var se *StatusError
	if !errors.As(err, &se) {
		return false
	}
	switch se.Code {
	case http.StatusNotAcceptable, http.StatusForbidden, http.StatusTooManyRequests:
		return true
	}
	return false
}

// withDetail adds the posting API's body, places, work arrangement and employment type.
// ok=false with a nil error is the platform's own answer that the posting is gone: an expired
// or unknown id answers 303 with {"metaSearch":{"isExpiredJob":"1"}} and no Location. An
// error is the absence of an answer.
func (s naukri) withDetail(ctx context.Context, base Job) (Job, bool, error) {
	var d naukriDetail
	if err := s.http.GetJSONWithHeaders(ctx, fmt.Sprintf(naukriDetailURL, base.ExternalID), naukriAppHeaders, &d); err != nil {
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusSeeOther || se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	jd := d.JobDetails
	if jd.JobID != base.ExternalID {
		return Job{}, false, fmt.Errorf("posting answered id %q", jd.JobID)
	}
	body := sanitizeHTML(jd.Description)
	if strings.TrimSpace(body) == "" {
		return Job{}, false, fmt.Errorf("posting carries no body")
	}
	base.Description = body
	var places []string
	for _, l := range jd.Locations {
		if p := strings.TrimSpace(l.Label); p != "" {
			places = append(places, p)
		}
	}
	if len(places) > 0 {
		base.Location = strings.Join(places, "; ")
	}
	base.WorkMode = naukriWorkMode(jd.WFHType)
	base.Remote = base.WorkMode == "remote"
	base.EmploymentType = naukriEmploymentType(jd.JobType, jd.EmploymentType)
	return base, true, nil
}

// toJob maps a search row to a body-less Job. The row's location placeholder stands in until
// the posting API's places replace it; it can carry a "Hybrid - " prefix the API does not.
func (r naukriListing) toJob(tech bool) (Job, bool) {
	title := strings.Join(strings.Fields(r.Title), " ")
	company := strings.TrimSpace(r.CompanyName)
	path := strings.TrimSpace(r.JDURL)
	if r.JobID == "" || title == "" || company == "" || !strings.HasPrefix(path, "/job-listings-") {
		return Job{}, false
	}
	j := Job{
		ExternalID: r.JobID,
		URL:        naukriBase + path,
		Title:      title,
		Company:    company,
		Location:   strings.TrimPrefix(r.placeholder("location"), "Hybrid - "),
		IsTechHint: tech,
	}
	if r.CreatedDate > 0 {
		t := time.UnixMilli(r.CreatedDate)
		j.PostedAt = NotFuture(&t)
	}
	// minimumExperience is the employer's stated floor; zero states none.
	if n, err := strconv.Atoi(strings.TrimSpace(r.MinExperience)); err == nil && n > 0 {
		j.ExperienceYearsMin = &n
	}
	// A shown range is per annum: the row's own label reads "95 Lacs-2.5 Cr PA". A label
	// without that suffix states some other period and is not published.
	sd := r.SalaryDetail
	if !sd.HideSalary && sd.Minimum > 0 && sd.Maximum >= sd.Minimum && strings.HasSuffix(r.placeholder("salary"), " PA") {
		lo, hi := int(sd.Minimum), int(sd.Maximum)
		j.SalaryMin, j.SalaryMax = &lo, &hi
		j.SalaryCurrency = firstNonEmpty(strings.TrimSpace(sd.Currency), "INR")
		j.SalaryPeriod = "year"
	}
	return j, true
}

func (r naukriListing) placeholder(kind string) string {
	for _, p := range r.Placeholders {
		if p.Type == kind {
			return strings.TrimSpace(p.Label)
		}
	}
	return ""
}

// naukriWorkMode maps wfhType: "0" Work from office, "2" Remote, "3" Hybrid — the labels the
// search's own wfhType facet gives them. "1" (Temp. WFH due to covid) states no lasting
// arrangement.
func naukriWorkMode(v string) string {
	switch strings.TrimSpace(v) {
	case "0":
		return "onsite"
	case "2":
		return "remote"
	case "3":
		return "hybrid"
	}
	return ""
}

// naukriEmploymentType maps the "<schedule>, <nature>" employmentType ("Full Time,
// Permanent", "Full Time, Temporary/Contractual", "Full Time, Freelance/Homebased"). A
// contractual or freelance nature is a contract whatever the schedule.
func naukriEmploymentType(jobType, employment string) string {
	if strings.EqualFold(strings.TrimSpace(jobType), "internship") {
		return "internship"
	}
	schedule, nature, _ := strings.Cut(employment, ",")
	nature = strings.ToLower(nature)
	if strings.Contains(nature, "contractual") || strings.Contains(nature, "freelance") {
		return "contract"
	}
	switch strings.ToLower(strings.TrimSpace(schedule)) {
	case "full time":
		return "full_time"
	case "part time":
		return "part_time"
	}
	return ""
}
