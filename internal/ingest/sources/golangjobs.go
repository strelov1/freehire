package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/dict/location"
	"github.com/strelov1/freehire/internal/dict/skilltag"
)

// golangJobs adapts Golang Jobs (golangjobs.tech), a niche board of Go developer postings, each
// naming its own employer — a boardless aggregator.
//
// The site is a client-rendered shell over a Supabase backend; it publishes no job sitemap and
// no server-rendered listing. The catalogue is read the way the site's own frontend reads it:
// through the project's PostgREST endpoint with the public anon key, both of which the app
// bundle carries. They are read from the bundle on every run, so a rotated key or a moved
// project is picked up by the next crawl, and a bundle that stops carrying them fails the crawl
// by name instead of quietly reading nothing.
//
// Every run reads the WHOLE table and fails unless the distinct postings it read equal the
// table's own count, which is what earns fullCatalog. The site never archives a posting, so a
// posting older than golangJobsMaxAge is dropped on the way in — see the Golang Jobs traps in
// AGENTS.md.
type golangJobs struct {
	http golangJobsHTTP
	now  func() time.Time
}

// golangJobsHTTP is the transport the adapter needs: the home page and the app bundle as text,
// and the PostgREST reads carrying the anon key headers.
type golangJobsHTTP interface {
	TextGetter
	HeaderJSONGetter
}

const (
	golangJobsHomeURL    = "https://www.golangjobs.tech/"
	golangJobsPostingURL = "https://www.golangjobs.tech/golang-jobs/"
	// golangJobsColumns are the columns a posting is read with; cities is the embedded
	// city row (city_id's foreign key), which carries the place.
	golangJobsColumns = "id,title,company,type,description,salary_min,salary_max,salary_currency," +
		"posted_at,application_url,slug,cities(name,country)"
	// golangJobsPageSize is the project's PostgREST max-rows: a larger limit is silently
	// clamped to it.
	golangJobsPageSize = 1000
	// golangJobsMaxPages is a ceiling far above anything the board has carried, never the
	// expected end: reaching it before the table's count is read FAILS the crawl.
	golangJobsMaxPages = 50
	// golangJobsMaxAge is how old a posting may be and still be ingested. The board never
	// retires a posting, and older postings are mostly closed upstream. Nothing downstream can
	// close a posting the board keeps listing, so age is the only staleness signal.
	golangJobsMaxAge = 60 * 24 * time.Hour
)

var (
	// golangJobsAssetRE reads only the bundle's path; the host it is fetched from is fixed
	// (golangJobsHomeURL), never taken from the page.
	golangJobsAssetRE = regexp.MustCompile(`src="/(assets/index-[A-Za-z0-9_-]+\.js)"`)
	// golangJobsClientRE reads the Supabase client's construction, `new X("<project url>",
	// "<anon key>")`. The project URL is accepted only on supabase.co.
	golangJobsClientRE = regexp.MustCompile(`"(https://[a-z0-9]{20}\.supabase\.co)"\s*,\s*"(eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+)"`)
)

// NewGolangJobs builds the Golang Jobs adapter over the given HTTP client.
func NewGolangJobs(c golangJobsHTTP) Source { return golangJobs{http: c, now: time.Now} }

func (golangJobs) Provider() string { return "golangjobs" }
func (golangJobs) boardless()       {}
func (golangJobs) aggregator()      {}

// fullCatalog: Fetch reads every page of the one table and fails on any read error, on an empty
// table, on a count that moves mid-read, on reaching golangJobsMaxPages and on any gap between
// the postings read and the table's count. See the fullCatalog interface (source.go) for the bar.
func (golangJobs) fullCatalog() {}

// golangJobsBackend is the Supabase project the app bundle names.
type golangJobsBackend struct {
	baseURL, anonKey string
}

func (b golangJobsBackend) headers() map[string]string {
	return map[string]string{"apikey": b.anonKey, "Authorization": "Bearer " + b.anonKey}
}

// golangJobsRow is one posting as the jobs table states it.
type golangJobsRow struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	Company        string   `json:"company"`
	Type           string   `json:"type"`
	Description    string   `json:"description"`
	SalaryMin      *float64 `json:"salary_min"`
	SalaryMax      *float64 `json:"salary_max"`
	SalaryCurrency string   `json:"salary_currency"`
	PostedAt       string   `json:"posted_at"`
	ApplicationURL string   `json:"application_url"`
	Slug           string   `json:"slug"`
	Cities         *struct {
		Name    string `json:"name"`
		Country string `json:"country"`
	} `json:"cities"`
}

func (s golangJobs) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	backend, err := s.backend(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.list(ctx, backend)
	if err != nil {
		return nil, err
	}
	// A link more than one posting points at is not either posting's own page (the board
	// re-posts a job as a new row under the same link), and a URL shared by several postings
	// would read as one posting to every URL consumer.
	links := map[string]int{}
	for _, r := range rows {
		if l := strings.TrimSpace(r.ApplicationURL); validHTTPURL(l) {
			links[l]++
		}
	}
	cutoff := s.now().Add(-golangJobsMaxAge)
	jobs := make([]Job, 0, len(rows))
	for _, r := range rows {
		if j, ok := r.toJob(links, cutoff); ok {
			jobs = append(jobs, j)
		}
	}
	return jobs, nil
}

// list reads every page of the table. Any page failing fails the crawl — the provider is swept
// by source, so a truncated read must not look like a shrunken catalogue.
func (s golangJobs) list(ctx context.Context, b golangJobsBackend) ([]golangJobsRow, error) {
	total, err := s.count(ctx, b)
	if err != nil {
		return nil, err
	}
	// The board always lists hundreds of postings; an empty table is a changed backend or a
	// key the row-level policy no longer admits, never an empty board.
	if total == 0 {
		return nil, fmt.Errorf("golangjobs: the jobs table lists no postings")
	}
	var out []golangJobsRow
	seen := map[string]bool{}
	// The offset advances by the rows actually received, so a server max-rows below
	// golangJobsPageSize costs pages rather than skipping rows.
	offset := 0
	for page := 0; len(seen) < total; page++ {
		if page == golangJobsMaxPages {
			return nil, fmt.Errorf("golangjobs: %d pages read and the table has more", golangJobsMaxPages)
		}
		rows, err := s.page(ctx, b, offset)
		if err != nil {
			return nil, fmt.Errorf("golangjobs: page %d: %w", page, err)
		}
		if len(rows) == 0 {
			break
		}
		offset += len(rows)
		for _, r := range rows {
			if r.ID == "" || seen[r.ID] {
				continue
			}
			seen[r.ID] = true
			out = append(out, r)
		}
	}
	if len(seen) != total {
		return nil, fmt.Errorf("golangjobs: read %d distinct postings of the %d the table lists", len(seen), total)
	}
	// A posting added or removed while the pages were read shifts the offsets under them.
	after, err := s.count(ctx, b)
	if err != nil {
		return nil, err
	}
	if after != total {
		return nil, fmt.Errorf("golangjobs: the table changed mid-read (%d postings, then %d)", total, after)
	}
	return out, nil
}

// count asks PostgREST for the open postings' count as an aggregate, the only form of it that
// arrives in the body rather than in a Content-Range header.
func (s golangJobs) count(ctx context.Context, b golangJobsBackend) (int, error) {
	var resp []struct {
		Count int `json:"count"`
	}
	u := b.baseURL + "/rest/v1/jobs?select=count&is_archived=eq.false"
	if err := s.http.GetJSONWithHeaders(ctx, u, b.headers(), &resp); err != nil {
		return 0, fmt.Errorf("golangjobs: count: %w", err)
	}
	if len(resp) != 1 {
		return 0, fmt.Errorf("golangjobs: count: %d rows, want 1", len(resp))
	}
	return resp[0].Count, nil
}

// page reads one page ordered by id, a key no edit moves, so consecutive offsets tile the table.
func (s golangJobs) page(ctx context.Context, b golangJobsBackend, offset int) ([]golangJobsRow, error) {
	q := url.Values{}
	q.Set("select", golangJobsColumns)
	q.Set("is_archived", "eq.false")
	q.Set("order", "id")
	q.Set("limit", strconv.Itoa(golangJobsPageSize))
	q.Set("offset", strconv.Itoa(offset))
	var rows []golangJobsRow
	if err := s.http.GetJSONWithHeaders(ctx, b.baseURL+"/rest/v1/jobs?"+q.Encode(), b.headers(), &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// backend reads the Supabase project URL and anon key from the app bundle the home page names.
func (s golangJobs) backend(ctx context.Context) (golangJobsBackend, error) {
	page, err := s.http.GetText(ctx, golangJobsHomeURL)
	if err != nil {
		return golangJobsBackend{}, fmt.Errorf("golangjobs: home page: %w", err)
	}
	asset := golangJobsAssetRE.FindStringSubmatch(page)
	if asset == nil {
		return golangJobsBackend{}, fmt.Errorf("golangjobs: home page names no app bundle")
	}
	bundle, err := s.http.GetText(ctx, golangJobsHomeURL+asset[1])
	if err != nil {
		return golangJobsBackend{}, fmt.Errorf("golangjobs: app bundle: %w", err)
	}
	m := golangJobsClientRE.FindStringSubmatch(bundle)
	if m == nil {
		return golangJobsBackend{}, fmt.Errorf("golangjobs: app bundle carries no Supabase client")
	}
	return golangJobsBackend{baseURL: m[1], anonKey: m[2]}, nil
}

func (r golangJobsRow) toJob(linkCounts map[string]int, cutoff time.Time) (Job, bool) {
	title := strings.Join(strings.Fields(r.Title), " ")
	company := strings.Join(strings.Fields(r.Company), " ")
	posted := parseRFC3339(r.PostedAt)
	if title == "" || company == "" || r.Slug == "" || posted == nil || posted.Before(cutoff) {
		return Job{}, false
	}
	jobURL := golangJobsPostingURL + url.PathEscape(r.Slug)
	if l := strings.TrimSpace(r.ApplicationURL); validHTTPURL(l) && linkCounts[l] == 1 {
		jobURL = l
	}
	job := Job{
		ExternalID:     r.ID,
		URL:            jobURL,
		Title:          title,
		Company:        company,
		Description:    sanitizeHTML(r.Description),
		PostedAt:       posted,
		EmploymentType: golangJobsEmploymentType(r.Type),
		// Every posting is filed as a Go developer position: the board's own page states
		// "<type> Golang developer position at <company>" for each one.
		IsTechHint: true,
		Skills:     skilltag.Canonicalize([]string{"go"}),
	}
	r.place(&job)
	r.salary(&job)
	return job, true
}

// place reads the posting's city row. The board files a remote posting under the city
// "Remote", whose country is the hiring scope: a country, a region ("Europe", "APAC") or
// "Global". Any other city states no arrangement, so WorkMode is left to the description.
func (r golangJobsRow) place(job *Job) {
	if r.Cities == nil {
		return
	}
	city, country := strings.TrimSpace(r.Cities.Name), strings.TrimSpace(r.Cities.Country)
	if c := location.NormalizeCountry(country); c != "" {
		job.Countries = []string{c}
	}
	if strings.EqualFold(city, "Remote") {
		job.WorkMode, job.Remote = "remote", true
		if strings.EqualFold(country, "Global") {
			country = ""
		}
		job.Location = joinNonEmpty("Remote", country)
		return
	}
	job.Location = joinNonEmpty(city, country)
}

// salary takes the posting's range. The board states no period in the row, but its own
// JobPosting markup publishes every range with unitText "YEAR", and the amounts agree (USD
// 220,000-280,000; INR 1,500,000-2,500,000). salary_currency defaults to USD on every row, so it
// counts only beside a bound.
func (r golangJobsRow) salary(job *Job) {
	lo, hi := golangJobsSalaryPart(r.SalaryMin), golangJobsSalaryPart(r.SalaryMax)
	currency := strings.ToUpper(strings.TrimSpace(r.SalaryCurrency))
	if (lo == nil && hi == nil) || len(currency) != 3 {
		return
	}
	job.SalaryMin, job.SalaryMax, job.SalaryCurrency, job.SalaryPeriod = lo, hi, currency, "year"
}

func golangJobsSalaryPart(v *float64) *int {
	if v == nil {
		return nil
	}
	return roundSalaryPart(*v)
}

// golangJobsEmploymentType maps the board's job types (its posting form offers full-time,
// part-time, contract and internship) onto vocab.EmploymentTypeValues.
func golangJobsEmploymentType(t string) string {
	switch strings.TrimSpace(t) {
	case "full-time":
		return "full_time"
	case "part-time":
		return "part_time"
	case "contract":
		return "contract"
	case "internship":
		return "internship"
	default:
		return ""
	}
}
