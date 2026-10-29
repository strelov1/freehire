package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/strelov1/freehire/internal/dict/location"
	"golang.org/x/time/rate"
)

// prospective adapts Prospective (ohws.prospective.ch), JobCloud's Swiss ATS behind the career
// sites of Coop, the federal administration (jobs.admin.ch), Stadler Rail, Insel Gruppe, the
// Universitätsspital Zürich, Helvetia, PwC Switzerland, the Universities of Zurich and Bern and
// many cantons, hospitals and utilities. A customer's career site lives on its own domain
// (jobs.coopjobs.ch, jobs.admin.ch) CNAMEd to the vendor, and every site is a front end over
// one public JSON API on ohws.prospective.ch, which this adapter reads directly:
//
//	GET /public/v1/medium/<medium>/jobs?lang=de&limit=N&offset=K   a listing: total + jobs
//	GET /public/v1/jobs/<viewkey>                                 one posting page (JobPosting)
//
// The board is the MEDIUM id — one publication channel of a customer (its careers site, its
// intranet, a regional portal such as jobiq.ch); one customer often runs several media that
// republish the same vacancies, each under its own job id, so boards are chosen so that no two
// list the same vacancy. The listing is read
// whole, to its natural end (fullBoardListing), and carries each job's body in tenant-defined
// sza_* fields. It does NOT name the employer — a group's medium lists its subsidiaries' jobs
// (fenaco: Volg, LANDI, Ernst Sutter) — so each new job is hydrated from its page's schema.org
// JobPosting for hiringOrganization, datePosted and employmentType.
//
// A job published on the medium but hosted elsewhere (its directlink is a /redirect/…/
// external_url hop to the customer's other ATS) is not this platform's posting and is skipped.
// robots.txt allows everything. Every request of the run goes through one limiter at one a
// second, and the run's posting reads are bounded across all boards (runBudget).
type prospective struct {
	http           JSONGetter
	pages          HTMLGetter
	limiter        waiter
	pageSize       int
	maxNewPerBoard int
	runBudget      *atomic.Int64
	deadlineMargin time.Duration
}

const (
	prospectiveAPI = "https://ohws.prospective.ch/public/v1"
	// prospectiveRate is one request a second across the whole run: one host serves every
	// customer.
	prospectiveRate = rate.Limit(1)
	// prospectivePageSize is the listing page. The API honours any size but answers slowly:
	// 1,000 of Coop's jobs took 66 s (2026-10-29), past even the long-timeout client.
	prospectivePageSize = 200
	// prospectiveMaxPages bounds the walk; needing more fails the crawl.
	prospectiveMaxPages = 50
	// prospectiveMaxNewPerBoard bounds the jobs one medium hydrates in a run, so Coop's ~2,900
	// do not spend the run's whole budget.
	prospectiveMaxNewPerBoard = 300
	// prospectiveMaxNewPerRun bounds the jobs the whole run hydrates: 1,200 page reads at one a
	// second is twenty minutes of the 50-minute run.
	prospectiveMaxNewPerRun = 1200
	// prospectiveDeadlineMargin is the time hydration leaves the pipeline to write what it read.
	prospectiveDeadlineMargin = 5 * time.Minute
)

// prospectiveBoard is a medium id.
var prospectiveBoard = regexp.MustCompile(`^[1-9][0-9]{0,9}$`)

// NewProspective builds the adapter: the API is read over api, the posting pages over pages.
func NewProspective(api JSONGetter, pages HTMLGetter) Source {
	b := new(atomic.Int64)
	b.Store(prospectiveMaxNewPerRun)
	return prospective{
		http:           api,
		pages:          pages,
		limiter:        rate.NewLimiter(prospectiveRate, 1),
		pageSize:       prospectivePageSize,
		maxNewPerBoard: prospectiveMaxNewPerBoard,
		runBudget:      b,
		deadlineMargin: prospectiveDeadlineMargin,
	}
}

func (prospective) Provider() string { return "prospective" }

// fullBoardListing: the listing is walked to its natural end, a short page, and any page failing
// fails the crawl. See the fullBoardListing interface and list.
func (prospective) fullBoardListing() {}

// prospectiveListing is one page of a medium's listing.
type prospectiveListing struct {
	Total *int             `json:"total"`
	Jobs  []prospectiveJob `json:"jobs"`
}

// prospectiveJob is one listed job.
type prospectiveJob struct {
	ID        string            `json:"id"`
	ViewKey   string            `json:"viewkey"`
	Title     string            `json:"title"`
	StartDate string            `json:"start_date"`
	Szas      map[string]any    `json:"szas"`
	Links     map[string]string `json:"links"`
}

// prospectivePosting is a listed job the medium hosts, with its public address.
type prospectivePosting struct {
	id, url string
	job     prospectiveJob
}

// prospectiveViewKey is a job's page key, a UUID.
var prospectiveViewKey = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// Fetch is the list-only fallback HydratingSource specifies; it hydrates every job.
func (s prospective) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew reads the medium's listing whole and buys a page only for a job the catalogue does
// not hold, at most maxNewPerBoard of them, within the run's budget and none once the run nears
// its deadline. A seen job is re-listed as a SeenRefresh; an unseen one left unread waits.
func (s prospective) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	board := strings.TrimSpace(e.Board)
	if !prospectiveBoard.MatchString(board) {
		return nil, fmt.Errorf("prospective: board %q is not a medium id", e.Board)
	}
	postings, err := s.list(ctx, board)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	var fresh []prospectivePosting
	deferred := 0
	for _, p := range postings {
		switch {
		case seen(p.id):
			jobs = append(jobs, Job{ExternalID: p.id, URL: p.url, Title: prospectiveTitle(p.job), Company: e.Company, SeenRefresh: true})
		case len(fresh) < s.maxNewPerBoard:
			fresh = append(fresh, p)
		default:
			deferred++
		}
	}
	if deferred > 0 {
		log.Printf("prospective: medium %s: hydration budget %d reached; %d new jobs deferred to a later run", board, s.maxNewPerBoard, deferred)
	}
	outOfTime, outOfBudget := 0, 0
	for _, p := range fresh {
		switch {
		case s.nearDeadline(ctx):
			outOfTime++
			continue
		case !s.spend():
			outOfBudget++
			continue
		}
		if j, ok := s.detail(ctx, e, p); ok {
			jobs = append(jobs, j)
		}
	}
	if outOfTime > 0 || outOfBudget > 0 {
		log.Printf("prospective: medium %s: %d new jobs left unread (%d near the run's deadline, %d past the run's budget of %d), deferred to later runs",
			board, outOfTime+outOfBudget, outOfTime, outOfBudget, prospectiveMaxNewPerRun)
	}
	return jobs, nil
}

// spend takes one page read from the run-wide budget, reporting false once it is gone.
func (s prospective) spend() bool {
	return s.runBudget == nil || s.runBudget.Add(-1) >= 0
}

func (s prospective) nearDeadline(ctx context.Context) bool {
	d, ok := ctx.Deadline()
	return ok && time.Until(d) < s.deadlineMargin
}

// wait spaces every request of the run. When the limiter can already tell that no slot opens
// before the deadline it refuses at once; the request will never be sent, so wait sits the
// deadline out and returns the context's own error, which the pipeline reads as an interrupted
// board rather than a failed one.
func (s prospective) wait(ctx context.Context) error {
	if s.limiter == nil {
		return nil
	}
	if err := s.limiter.Wait(ctx); err != nil {
		<-ctx.Done()
		return ctx.Err()
	}
	return nil
}

// list reads the medium's whole listing, page by page until a short page — the listing's natural
// end. Any page failing fails the crawl: a truncated walk would read as closures. The stated
// total is NOT the check: it differs from what the listing returns by a job or two even on a
// single page (2026-10-29: 12 of 165 media, 21 stated and 20 listed), so an exact match would
// fail those boards on every run. A medium the API does not know answers 400 (or 404), which is
// the board gone.
func (s prospective) list(ctx context.Context, board string) ([]prospectivePosting, error) {
	var all []prospectiveJob
	total := -1
	for page := 0; ; page++ {
		if page >= prospectiveMaxPages {
			return nil, fmt.Errorf("prospective: medium %s: %d jobs need more than %d pages", board, total, prospectiveMaxPages)
		}
		if err := s.wait(ctx); err != nil {
			return nil, err
		}
		q := url.Values{"lang": {"de"}, "limit": {strconv.Itoa(s.pageSize)}, "offset": {strconv.Itoa(page * s.pageSize)}}
		var resp prospectiveListing
		err := s.http.GetJSON(ctx, prospectiveAPI+"/medium/"+board+"/jobs?"+q.Encode(), &resp)
		var se *StatusError
		switch {
		case errors.As(err, &se) && (se.Code == http.StatusBadRequest || se.Code == http.StatusNotFound):
			return nil, fmt.Errorf("prospective: medium %s: %w", board, ErrBoardGone)
		case err != nil:
			return nil, fmt.Errorf("prospective: medium %s page %d: %w", board, page, err)
		case resp.Total == nil:
			return nil, fmt.Errorf("prospective: medium %s: the answer states no total", board)
		}
		total = *resp.Total
		all = append(all, resp.Jobs...)
		if len(resp.Jobs) < s.pageSize {
			break
		}
	}
	var out []prospectivePosting
	seen := map[string]bool{}
	external := 0
	for _, j := range all {
		id := strings.TrimSpace(j.ID)
		// An id can repeat across pages when the listing shifts between two requests.
		if id == "" || seen[id] || !prospectiveViewKey.MatchString(j.ViewKey) {
			continue
		}
		seen[id] = true
		link, ok := prospectiveLink(j.Links["directlink"])
		if !ok {
			external++
			continue
		}
		out = append(out, prospectivePosting{id: id, url: link, job: j})
	}
	if len(seen) != total {
		log.Printf("prospective: medium %s: listed %d jobs, the answer states %d", board, len(seen), total)
	}
	if external > 0 {
		log.Printf("prospective: medium %s: %d jobs hosted on another platform, skipped", board, external)
	}
	return out, nil
}

// prospectiveLink validates a job's public address: an https page that is not the vendor's
// /redirect/ hop to a job hosted elsewhere.
func prospectiveLink(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host == "" || strings.Contains(u.Path, "/redirect/") {
		return "", false
	}
	u.RawQuery, u.Fragment = "", ""
	return u.String(), true
}

// prospectiveLD is the part of the posting page's schema.org JobPosting the adapter reads.
type prospectiveLD struct {
	Description        string          `json:"description"`
	DatePosted         string          `json:"datePosted"`
	EmploymentType     json.RawMessage `json:"employmentType"`
	JobLocation        schemaPlaces    `json:"jobLocation"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
}

// detail buys one job's page. A 404/410 drops it (withdrawn since the listing); any other
// failure is an unreadable marker.
func (s prospective) detail(ctx context.Context, e CompanyEntry, p prospectivePosting) (Job, bool) {
	if err := s.wait(ctx); err != nil {
		return unreadableDetail(p.id, p.url, e.Company), true
	}
	root, err := s.pages.GetHTML(ctx, prospectiveAPI+"/jobs/"+p.job.ViewKey)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(p.id, p.url, e.Company), true
		}
		return Job{}, false
	}
	// Some customers' pages carry no JobPosting at all (fenaco, Helvetia, Bühler, 11 of 137
	// media on 2026-10-29). The listing already holds the body, so such a job keeps the board's
	// employer and the listing's dates rather than going unread forever.
	var ld prospectiveLD
	ldJobPosting(root, &ld)
	j, ok := prospectiveToJob(e, p, ld)
	if !ok {
		return unreadableDetail(p.id, p.url, e.Company), true
	}
	return j, true
}

// prospectiveBody is the order the sza_* body fields are joined in. Each customer defines its own
// set; these are the ones that carry prose (2026-10-29, twelve media sampled).
var prospectiveBody = []struct{ key, heading string }{
	{"sza_introduction", ""},
	{"sza_short_description", ""},
	{"sza_tasks", "Tasks"},
	{"sza_requirements", "Requirements"},
	{"sza_education", "Education"},
	{"sza_language_requirements", "Languages"},
	{"sza_benefits", "Benefits"},
	{"sza_company_profil", "About the company"},
	{"sza_application", "Application"},
}

// prospectiveToJob maps a listed job and its page's JobPosting to a Job. ok is false for one with
// no title, employer or body.
func prospectiveToJob(e CompanyEntry, p prospectivePosting, ld prospectiveLD) (Job, bool) {
	title := prospectiveTitle(p.job)
	company := firstNonEmpty(strings.TrimSpace(html.UnescapeString(ld.HiringOrganization.Name)), e.Company)
	var body strings.Builder
	for _, f := range prospectiveBody {
		text := prospectiveSza(p.job.Szas, f.key)
		if f.key == "sza_benefits" {
			// Some customers split their benefits into numbered fields.
			for i := 2; i <= 9; i++ {
				if more := prospectiveSza(p.job.Szas, "sza_benefits_"+strconv.Itoa(i)); more != "" {
					text += "<p>" + more + "</p>"
				}
			}
		}
		if text == "" {
			continue
		}
		if f.heading == "" {
			body.WriteString("<p>" + text + "</p>")
		} else {
			body.WriteString(headedSection(f.heading, text))
		}
	}
	description := sanitizeHTML(body.String())
	if strings.TrimSpace(description) == "" {
		description = sanitizeHTML(ld.Description)
	}
	if title == "" || company == "" || strings.TrimSpace(description) == "" {
		return Job{}, false
	}
	j := Job{
		ExternalID:     p.id,
		URL:            p.url,
		Title:          title,
		Company:        company,
		Description:    description,
		PostedAt:       NotFuture(parseRFC3339OrDate(firstNonEmpty(strings.TrimSpace(ld.DatePosted), strings.TrimSpace(p.job.StartDate)))),
		EmploymentType: schemaFixedTermEmploymentType(ld.EmploymentType),
	}
	j.Location, j.Countries = prospectiveLocation(p.job.Szas)
	if j.Location == "" {
		j.Location, j.Countries = schemaPlacesLocation(ld.JobLocation)
	}
	return j, true
}

// prospectiveTag is an HTML tag inside a title: customers break long titles with <BR /> (fenaco,
// 2026-10-29).
var prospectiveTag = regexp.MustCompile(`<[^>]*>`)

func prospectiveTitle(j prospectiveJob) string {
	raw := firstNonEmpty(j.Title, prospectiveSza(j.Szas, "sza_title"))
	return strings.Join(strings.Fields(prospectiveTag.ReplaceAllString(html.UnescapeString(raw), " ")), " ")
}

// prospectiveLocation reads the workplace, else the location, as "City, Region" and its country
// (a country NAME in the customer's language: "Schweiz", "Suisse", "Deutschland").
func prospectiveLocation(szas map[string]any) (string, []string) {
	for _, prefix := range []string{"sza_workplace", "sza_location"} {
		city := prospectiveSza(szas, prefix+".city")
		region := prospectiveSza(szas, prefix+".region")
		loc := joinNonEmpty(city, region)
		if loc == "" {
			continue
		}
		var countries []string
		if c := location.NormalizeCountry(prospectiveSza(szas, prefix+".country")); c != "" {
			countries = []string{c}
		}
		return loc, countries
	}
	return "", nil
}

// prospectiveSza reads one sza_* field as trimmed text; customers fill them with strings.
func prospectiveSza(szas map[string]any, key string) string {
	s, _ := szas[key].(string)
	return strings.TrimSpace(s)
}
