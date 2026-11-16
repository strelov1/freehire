package sources

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"github.com/strelov1/freehire/internal/dict/normalize"
)

// herkey adapts HerKey (herkey.com, formerly JobsForHer), an India-focused job platform for
// women. The whole catalogue is one search index behind the site's own backend
// (api-prod.herkey.com), so the provider is a boardless aggregator. The listing names every
// posting's employer, place and apply link but carries no body for most of them, so this is a
// HydratingSource that buys a posting's detail only when the catalogue lacks it, and
// CoverageGated because a large share of it re-lists postings from ATS boards freehire already
// crawls first-party (greenhouse, smartrecruiters, workday, taleo, lever). See the HerKey traps
// in AGENTS.md.
type herkey struct {
	http HeaderJSONGetter
}

const (
	herkeyAPI        = "https://api-prod.herkey.com/api/v1/herkey/"
	herkeySessionURL = herkeyAPI + "generate-session"
	herkeyListURL    = herkeyAPI + "jobs/es_candidate_jobs?page_no=%d&page_size=%d"
	herkeyDetailURL  = herkeyAPI + "jobs/jobs/%s"
	herkeyJobURL     = "https://www.herkey.com/jobs/%s/%s"

	// herkeyPageSize is served as asked, so the catalogue is a handful of requests.
	herkeyPageSize = 500
	// herkeyMaxPages is a ceiling far above the catalogue, never the expected end. Reaching it
	// fails the crawl: the provider is swept by source, so a truncated walk must not pass.
	herkeyMaxPages = 40
)

// The detail endpoint is slow (seconds per call), so the in-flight bound, not the rate, keeps the
// load on that origin modest; the limiter caps the burst when it answers fast. One run hydrates at
// most herkeyDetailBudget details and the rest accrete over later runs, so a cold catalogue cannot
// outlive the unit timeout and save nothing.
const (
	herkeyRequestInterval = time.Second
	herkeyRequestBurst    = 2
	herkeyDetailWorkers   = 4
	herkeyDetailBudget    = 600
)

// herkeyZone is the zone of the API's zone-less timestamps: India Standard Time.
var herkeyZone = time.FixedZone("IST", 5*3600+30*60)

// herkeyRedirectors wrap the employer's own link in a tracking redirect; the wrapped link is
// carried in the named query parameter. recruitics carries Aristocrat's Workday postings.
var herkeyRedirectors = map[string]string{"jsv3.recruitics.com": "rx_url"}

// NewHerKey builds the HerKey adapter over the given HTTP client.
func NewHerKey(c HeaderJSONGetter) Source { return herkey{http: c} }

// pacedHerKeyGetter holds every request of a run — session, listing and detail — to one
// limiter, wired once per registry build.
func pacedHerKeyGetter(c HeaderJSONGetter) HeaderJSONGetter {
	return rateLimitedHeaderJSONGetter{
		inner:   c,
		limiter: rate.NewLimiter(rate.Every(herkeyRequestInterval), herkeyRequestBurst),
	}
}

func (herkey) Provider() string { return "herkey" }
func (herkey) boardless()       {}
func (herkey) aggregator()      {}

// herkey lists its whole catalogue every run and fails any walk it cannot prove complete
// against the index's own total, so a posting the run did not see is gone.
func (herkey) fullCatalog() {}

// herkeyListing is one row of the search index.
type herkeyListing struct {
	ID          int64    `json:"id"`
	Title       string   `json:"title"`
	CompanyName string   `json:"company_name"`
	Location    string   `json:"location_name"`
	WorkMode    []string `json:"work_mode"`
	JobTypes    []string `json:"job_types"`
	MinYear     *int     `json:"min_year"`
	RedirectURL string   `json:"redirect_url"`
	CreatedOn   string   `json:"created_on"`
}

type herkeyListPage struct {
	Pagination struct {
		TotalItems int  `json:"total_items"`
		HasNext    bool `json:"has_next"`
	} `json:"pagination"`
	Body []herkeyListing `json:"body"`
}

// herkeyDetail is the posting resource, reduced to what the listing lacks.
type herkeyDetail struct {
	Body []struct {
		ID               int64   `json:"id"`
		Status           string  `json:"status"`
		Description      *string `json:"description"`
		Responsibilities *string `json:"responsibilities"`
		Requirements     *string `json:"requirements"`
		PublishedOn      *string `json:"published_on"`
	} `json:"body"`
}

func (s herkey) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s herkey) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	rows, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	return s.hydrate(ctx, rows, seen, nil)
}

// FetchNewGated is FetchNew told which postings the pipeline will discard anyway: a covered
// employer's postings are never stored, so without the coverage answer their slow details would be
// bought and thrown away on every run.
func (s herkey) FetchNewGated(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error) {
	rows, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var covered map[string]bool
	if gate.Covered != nil {
		names := make([]string, 0, len(rows))
		for _, r := range rows {
			names = append(names, strings.TrimSpace(r.CompanyName))
		}
		covered = gate.Covered(names)
	}
	return s.hydrate(ctx, rows, seen, covered)
}

// list mints the anonymous session the site's own frontend uses and walks the index to its
// end. Any page failing fails the crawl, and so does a walk whose distinct ids do not equal the
// index's total: the provider is swept by source, and a short walk would close live postings.
func (s herkey) list(ctx context.Context) ([]herkeyListing, error) {
	var sess struct {
		Body struct {
			SessionID string `json:"session_id"`
		} `json:"body"`
	}
	if err := s.http.GetJSONWithHeaders(ctx, herkeySessionURL, nil, &sess); err != nil {
		return nil, fmt.Errorf("herkey: session: %w", err)
	}
	if sess.Body.SessionID == "" {
		return nil, fmt.Errorf("herkey: session answered no session id")
	}
	headers := map[string]string{"Authorization": "Token " + sess.Body.SessionID}

	var out []herkeyListing
	ids := map[int64]bool{}
	total := -1
	for page := 1; ; page++ {
		if page > herkeyMaxPages {
			return nil, fmt.Errorf("herkey: listing did not end within %d pages", herkeyMaxPages)
		}
		var p herkeyListPage
		if err := s.http.GetJSONWithHeaders(ctx, fmt.Sprintf(herkeyListURL, page, herkeyPageSize), headers, &p); err != nil {
			return nil, fmt.Errorf("herkey: listing page %d: %w", page, err)
		}
		if total < 0 {
			total = p.Pagination.TotalItems
		} else if p.Pagination.TotalItems != total {
			return nil, fmt.Errorf("herkey: listing total moved from %d to %d mid-walk", total, p.Pagination.TotalItems)
		}
		for _, r := range p.Body {
			if r.ID == 0 || ids[r.ID] {
				continue
			}
			ids[r.ID] = true
			out = append(out, r)
		}
		if !p.Pagination.HasNext || len(p.Body) == 0 {
			break
		}
	}
	// The platform always lists postings, so an empty index is a broken answer, not a quiet day.
	if total <= 0 {
		return nil, fmt.Errorf("herkey: listing states %d postings", total)
	}
	if len(ids) != total {
		return nil, fmt.Errorf("herkey: listing read %d distinct postings of %d", len(ids), total)
	}
	return out, nil
}

// hydrate maps the listed rows to Jobs. A seen posting is a liveness refresh with no request; a
// posting the pipeline will discard — its employer covered — is yielded body-less for the pipeline to count; every other posting costs one
// detail request, up to the run's budget. A detail that could not be read defers its posting to
// a later run rather than storing it body-less, which would never be hydrated again.
func (s herkey) hydrate(ctx context.Context, rows []herkeyListing, seen func(string) bool,
	covered map[string]bool) ([]Job, error) {
	links := map[string]int{}
	for _, r := range rows {
		if l := r.applyLink(); l != "" {
			links[l]++
		}
	}
	var budget, answered atomic.Int64
	jobs := fetchDetails(rows, herkeyDetailWorkers, func(r herkeyListing) (Job, bool) {
		base, ok := r.toJob(links)
		if !ok {
			return Job{}, false
		}
		if seen(base.ExternalID) {
			base.SeenRefresh = true
			return base, true
		}
		if covered[base.Company] {
			return base, true
		}
		if budget.Add(1) > herkeyDetailBudget {
			return Job{}, false
		}
		job, ok, err := s.withDetail(ctx, base)
		if err != nil {
			log.Printf("herkey: detail %s failed; deferring: %v", base.ExternalID, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, ok
	})
	// A crawl that asked for details and got no answer to any of them is broken, not quiet.
	// "Answered" counts a posting the platform reported gone: the index keeps listing expired
	// postings, so a run whose only new rows are those must still pass.
	if wanted := min(budget.Load(), herkeyDetailBudget); wanted > 0 && answered.Load() == 0 {
		return nil, fmt.Errorf("herkey: requested %d posting details and none answered", wanted)
	}
	if budget.Load() > herkeyDetailBudget {
		log.Printf("herkey: detail budget %d spent; %d new postings deferred to later runs",
			herkeyDetailBudget, budget.Load()-herkeyDetailBudget)
	}
	return jobs, nil
}

// withDetail adds the posting resource's body and date. ok=false with a nil error is the
// withDetail adds the posting resource's body and date. ok=false with a nil error is the
// platform's own answer that the posting is gone: a 400 or 404 for the id, an empty body, or a
// status other than active (the search index keeps listing postings the posting resource has
// already expired). An error is the absence of an answer.
func (s herkey) withDetail(ctx context.Context, base Job) (Job, bool, error) {
	var d herkeyDetail
	if err := s.http.GetJSONWithHeaders(ctx, fmt.Sprintf(herkeyDetailURL, base.ExternalID), nil, &d); err != nil {
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusBadRequest || se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	if len(d.Body) == 0 || strconv.FormatInt(d.Body[0].ID, 10) != base.ExternalID {
		return Job{}, false, nil
	}
	b := d.Body[0]
	if b.Status != "active" {
		return Job{}, false, nil
	}
	var parts []string
	placeholder := false
	for _, p := range []*string{b.Description, b.Responsibilities, b.Requirements} {
		if p == nil || strings.TrimSpace(*p) == "" {
			continue
		}
		text := *p
		if herkeyPlaceholderBody(text) {
			placeholder = true
			continue
		}
		// Imported bodies (the pharma careers site's) arrive as newline-separated plain text.
		if !strings.Contains(text, "<") {
			text = plainTextToHTML(text)
		}
		parts = append(parts, text)
	}
	body := sanitizeHTML(strings.Join(parts, "\n"))
	if strings.TrimSpace(body) == "" {
		// A placeholder is the platform's own answer that the posting has no description; nothing
		// else is the absence of one.
		if placeholder {
			return Job{}, false, nil
		}
		return Job{}, false, fmt.Errorf("posting carries no body")
	}
	base.Description = body
	if b.PublishedOn != nil {
		if t := herkeyTime(*b.PublishedOn); t != nil {
			base.PostedAt = t
		}
	}
	return base, true, nil
}

// herkeyPlaceholderBody reports whether a body field holds a placeholder instead of text: the
// imported pharma postings carry a literal "TBD" (quotes included), and the API renders a
// missing value as the string "None".
func herkeyPlaceholderBody(s string) bool {
	v := strings.ToLower(strings.Trim(strings.TrimSpace(s), `"'`))
	return v == "tbd" || v == "none"
}

// toJob maps a listing row to a body-less Job. A row with no employer is dropped: the catalogue
// cannot file it, and the coverage gate cannot judge it.
func (r herkeyListing) toJob(linkCounts map[string]int) (Job, bool) {
	title := strings.Join(strings.Fields(r.Title), " ")
	company := strings.TrimSpace(r.CompanyName)
	if r.ID == 0 || title == "" || company == "" {
		return Job{}, false
	}
	id := strconv.FormatInt(r.ID, 10)
	jobURL := fmt.Sprintf(herkeyJobURL, firstNonEmpty(normalize.Slug(title), "job"), id)
	// A link several postings share is an intake page, not this posting's own, and would read
	// as one posting to every URL consumer.
	if l := r.applyLink(); l != "" && linkCounts[l] == 1 {
		jobURL = l
	}
	loc := strings.TrimSpace(r.Location)
	if strings.EqualFold(loc, "any") {
		loc = "" // the platform's "no particular place", not a place
	}
	mode := herkeyWorkMode(r.WorkMode)
	j := Job{
		ExternalID:     id,
		URL:            jobURL,
		Title:          title,
		Company:        company,
		Location:       loc,
		WorkMode:       mode,
		Remote:         mode == "remote",
		EmploymentType: herkeyEmploymentType(r.JobTypes),
		PostedAt:       herkeyTime(r.CreatedOn),
	}
	// min_year is a form field defaulting to 0, so only a stated floor above zero is a fact.
	if r.MinYear != nil && *r.MinYear > 0 {
		j.ExperienceYearsMin = r.MinYear
	}
	return j, true
}

// applyLink is the employer's own posting link when the row states a usable one, unwrapped
// from a tracking redirect.
func (r herkeyListing) applyLink() string {
	raw := strings.TrimSpace(r.RedirectURL)
	if !validHTTPURL(raw) {
		return ""
	}
	u, _ := url.Parse(raw)
	if param, ok := herkeyRedirectors[strings.ToLower(u.Hostname())]; ok {
		inner := u.Query().Get(param)
		if !validHTTPURL(inner) {
			return ""
		}
		return inner
	}
	return raw
}

// herkeyWorkMode maps the work_mode picklist. A row ticking several arrangements states none.
func herkeyWorkMode(values []string) string {
	mode := ""
	for _, v := range values {
		var m string
		switch strings.TrimSpace(v) {
		case "work_from_home":
			m = "remote"
		case "work_from_office":
			m = "onsite"
		case "hybrid":
			m = "hybrid"
		case "":
			continue
		default:
			return ""
		}
		if mode != "" && mode != m {
			return ""
		}
		mode = m
	}
	return mode
}

// herkeyEmploymentType maps the job_types picklist, spelled both "full time" and "full_time".
// returnee_program is a hiring programme for women returning to work, not a schedule, so it is
// ignored; a row ticking several schedules states none.
func herkeyEmploymentType(values []string) string {
	out := ""
	for _, v := range values {
		var t string
		switch strings.ReplaceAll(strings.TrimSpace(strings.ToLower(v)), " ", "_") {
		case "full_time":
			t = "full_time"
		case "part_time":
			t = "part_time"
		case "freelance":
			t = "contract"
		case "internship":
			t = "internship"
		case "", "returnee_program":
			continue
		default:
			return ""
		}
		if out != "" && out != t {
			return ""
		}
		out = t
	}
	return out
}

// herkeyTime parses the API's zone-less "2006-01-02 15:04:05" stamps as India time.
func herkeyTime(s string) *time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04:05", strings.TrimSpace(s), herkeyZone)
	if err != nil {
		return nil
	}
	return NotFuture(&t)
}
