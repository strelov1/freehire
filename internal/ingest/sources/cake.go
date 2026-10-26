package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// cake adapts Cake (www.cake.me), a Taiwanese job board that also lists postings for Japan,
// Vietnam, Indonesia, Singapore and the US. It is one central catalogue with no per-employer
// tenancy, so it is boardless and an aggregator: the employer is read per posting.
//
// Measured live on 2026-10-25 (see the "Cake traps" section of AGENTS.md):
//
//   - The search answers at most 499 rows per query: `page * per_page` must stay below 500,
//     `per_page` is capped at 50, and a query matching more reports total_entries 500 with
//     total_relation "gte". The catalogue (~16k postings, ~3.3k of them under the `it_` family)
//     is therefore read as SLICES, split recursively over the platform's own facets until every
//     slice reports an exact ("eq") total that fits the window.
//   - Every posting carries exactly one profession, one job type, one seniority level, one
//     experience bracket, one remote mode and one management span — the leaf professions' totals
//     sum exactly to their families', and each other facet's values sum exactly to the slice they
//     split — so every split is a partition and nothing falls between two slices.
//   - The search's description is plain text; the detail endpoint carries the HTML body, the
//     separate requirements section, the posting date, the country code and the remote mode. A
//     detail costs a request, so only postings the catalogue lacks are hydrated.
//   - One exit IP is refused (429, Retry-After ~50s) after ~40 requests a minute, and the crawl
//     hosts' own IPs are answered with a Cloudflare challenge; the crawl egresses through the
//     rotating proxy (see proxy.go), whose exits were served 60 of 60.
type cake struct {
	http         cakeHTTP
	maxNewPerRun int
	backoff      time.Duration
}

// cakeHTTP is the transport the adapter needs: the POST search and the GET detail.
type cakeHTTP interface {
	JSONPoster
	JSONGetter
}

const (
	cakeAPI       = "https://api.cake.me/api/client/v1/jobs"
	cakeSearchURL = cakeAPI + "/search"
	cakeSiteURL   = "https://www.cake.me"
	// cakePerPage and cakeMaxPage read the largest window the search allows: 45 * 11 = 495 rows,
	// the most any per_page at or below 50 reaches under page*per_page < 500.
	cakePerPage = 45
	cakeMaxPage = 11
	cakeWindow  = cakePerPage * cakeMaxPage
)

// cakeSplitFacets is the order a slice too large to read is split in. Each is a partition of
// the slice (see the type doc); professions comes first because it is the finest, and only its
// leaf values ("family_leaf") are used, since a family value overlaps its leaves.
var cakeSplitFacets = []string{
	"professions", "job_types", "seniority_levels", "year_of_seniority", "remote",
	"number_of_management",
}

// cakeDetailWorkers keeps a few detail requests in flight; the rotating exits are not
// rate-limited per crawl, so latency, not the platform, sets the pace.
const cakeDetailWorkers = 4

// cakeMaxNewPerRun bounds how many unseen postings one run hydrates. The first runs face the
// whole ~16k catalogue; at ~1.5 s a detail over four workers this is ~15 minutes, and a posting
// past the bound is hydrated by a later run.
const cakeMaxNewPerRun = 2500

// cakeRefusalRetries is how many times a refused (403/429), failed (5xx) or unanswered request
// is retried. A retry leaves through a fresh rotating exit, and a bad exit is the usual cause:
// the first production run lost its whole listing to ONE search whose body timed out.
const cakeRefusalRetries = 3

// cakeRetryBackoff is the first retry's wait, doubling after.
const cakeRetryBackoff = 2 * time.Second

// NewCake builds the adapter over the given client.
func NewCake(c cakeHTTP) Source {
	return cake{http: c, maxNewPerRun: cakeMaxNewPerRun, backoff: cakeRetryBackoff}
}

func (cake) Provider() string { return "cake" }

// One central catalogue, no per-tenant id.
func (cake) boardless() {}

// Many employers post here, each named per posting.
func (cake) aggregator() {}

type cakeSearchRequest struct {
	Query   string              `json:"query"`
	Page    int                 `json:"page"`
	PerPage int                 `json:"per_page"`
	SortBy  string              `json:"sort_by"`
	Filters map[string][]string `json:"filters"`
}

type cakeSearchResponse struct {
	TotalEntries  int                        `json:"total_entries"`
	TotalRelation string                     `json:"total_relation"`
	Facets        map[string]json.RawMessage `json:"available_facets"`
	Data          []cakeListing              `json:"data"`
}

// cakeListing is one search row. Its description is plain text, read only as a fallback.
type cakeListing struct {
	Path      string       `json:"path"`
	Title     string       `json:"title"`
	Locations []cakeLocale `json:"locations_with_locale"`
	Page      struct {
		Path string `json:"path"`
		Name string `json:"name"`
	} `json:"page"`
}

// cakeLocale is one of a listing's places, named in each locale; the English name is read.
type cakeLocale struct {
	En string `json:"en"`
}

// cakeDetail is the part of the detail endpoint the adapter reads.
type cakeDetail struct {
	Path          string `json:"path"`
	Title         string `json:"title"`
	Profession    string `json:"profession_v2"`
	JobType       string `json:"job_type"`
	Seniority     string `json:"seniority_level"`
	MinExperience *int   `json:"min_work_exp_year"`
	Locations     []struct {
		CountryCode string `json:"country_code"`
		FullNameEn  string `json:"full_name_en"`
	} `json:"locations"`
	Remote        string    `json:"remote"`
	SalaryMin     string    `json:"salary_min"`
	SalaryMax     string    `json:"salary_max"`
	Currency      string    `json:"salary_currency"`
	SalaryType    string    `json:"salary_type"`
	HideSalary    bool      `json:"hide_salary_completely"`
	HideSalaryMax bool      `json:"hide_salary_max"`
	Description   string    `json:"description"`
	Requirements  string    `json:"requirements"`
	CreatedAt     time.Time `json:"created_at"`
}

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s cake) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew lists the whole catalogue, refreshes the postings already stored and hydrates up to
// maxNewPerRun of the rest. Any listing request failing fails the crawl: a partial list would
// read to the sweep as postings gone. A new posting whose detail cannot be read is not yielded;
// nothing is stored for it yet, so a later run tries again.
func (s cake) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	listings, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var unseen []cakeListing
	var refreshed []Job
	for _, l := range listings {
		if seen(l.Path) {
			refreshed = append(refreshed, Job{
				ExternalID: l.Path, URL: cakePostingURL(l), Title: l.Title, Company: l.Page.Name,
				SeenRefresh: true,
			})
			continue
		}
		if len(unseen) < s.maxNewPerRun {
			unseen = append(unseen, l)
		}
	}
	jobs := fetchDetails(unseen, cakeDetailWorkers, func(l cakeListing) (Job, bool) {
		return s.detail(ctx, l)
	})
	return append(jobs, refreshed...), nil
}

// list reads every slice of the catalogue and returns its postings, each once.
func (s cake) list(ctx context.Context) ([]cakeListing, error) {
	var (
		byPath    = make(map[string]bool)
		out       []cakeListing
		truncated int
	)
	add := func(rows []cakeListing) {
		for _, r := range rows {
			if r.Path == "" || byPath[r.Path] {
				continue
			}
			byPath[r.Path] = true
			out = append(out, r)
		}
	}
	var walk func(filters map[string][]string, depth int) error
	walk = func(filters map[string][]string, depth int) error {
		first, err := s.search(ctx, filters, 1)
		if err != nil {
			return err
		}
		if first.TotalRelation == "eq" && first.TotalEntries <= cakeWindow {
			add(first.Data)
			return s.readPages(ctx, filters, first.TotalEntries, add)
		}
		var facet string
		var values []string
		if depth < len(cakeSplitFacets) {
			facet = cakeSplitFacets[depth]
			values = cakeFacetValues(first.Facets, facet)
		}
		if len(values) == 0 {
			// Nothing left to split on: read what the window reaches. Measured, this is a
			// retail chain posting hundreds of near-identical store vacancies.
			add(first.Data)
			truncated++
			return s.readPages(ctx, filters, cakeWindow, add)
		}
		for _, v := range values {
			if err := walk(withFilter(filters, facet, v), depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(map[string][]string{}, 0); err != nil {
		return nil, err
	}
	if truncated > 0 {
		log.Printf("cake: %d slices exceed the %d-row search window on every facet; read their first %d rows", truncated, cakeWindow, cakeWindow)
	}
	if len(out) == 0 {
		return nil, errors.New("cake: the search listed no postings")
	}
	return out, nil
}

// readPages reads pages 2.. of a slice holding total rows (page 1 is already in hand).
func (s cake) readPages(ctx context.Context, filters map[string][]string, total int, add func([]cakeListing)) error {
	for page := 2; (page-1)*cakePerPage < total && page <= cakeMaxPage; page++ {
		resp, err := s.search(ctx, filters, page)
		if err != nil {
			return err
		}
		add(resp.Data)
	}
	return nil
}

// search issues one search request, retrying a refused one through a fresh exit.
func (s cake) search(ctx context.Context, filters map[string][]string, page int) (cakeSearchResponse, error) {
	body := cakeSearchRequest{Page: page, PerPage: cakePerPage, SortBy: "latest", Filters: filters}
	var resp cakeSearchResponse
	err := retryWhile(ctx, cakeRefusalRetries, s.backoff, isRefusedOrTransient, func() error {
		resp = cakeSearchResponse{}
		return s.http.PostJSON(ctx, cakeSearchURL, body, &resp)
	})
	if err != nil {
		return resp, fmt.Errorf("cake: search %v page %d: %w", filters, page, err)
	}
	return resp, nil
}

// cakeFacetValues returns the values a facet offers to split on. A facet that is not a list of
// strings, or is absent, offers none — the slice is then read as far as the window reaches.
func cakeFacetValues(facets map[string]json.RawMessage, facet string) []string {
	var values []string
	if err := json.Unmarshal(facets[facet], &values); err != nil {
		return nil
	}
	if facet != "professions" {
		return values
	}
	leaves := values[:0]
	for _, v := range values {
		if strings.Contains(v, "_") {
			leaves = append(leaves, v)
		}
	}
	return leaves
}

// withFilter copies filters and adds one facet value.
func withFilter(filters map[string][]string, facet, value string) map[string][]string {
	out := make(map[string][]string, len(filters)+1)
	for k, v := range filters {
		out[k] = v
	}
	out[facet] = []string{value}
	return out
}

func cakePostingURL(l cakeListing) string {
	return cakeSiteURL + "/companies/" + url.PathEscape(l.Page.Path) + "/jobs/" + url.PathEscape(l.Path)
}

// detail hydrates one new posting. A 404 is the platform's answer that it is gone.
func (s cake) detail(ctx context.Context, l cakeListing) (Job, bool) {
	var d cakeDetail
	err := retryWhile(ctx, cakeRefusalRetries, s.backoff, isRefusedOrTransient, func() error {
		d = cakeDetail{}
		return s.http.GetJSON(ctx, cakeAPI+"/"+url.PathEscape(l.Path), &d)
	})
	if err != nil || d.Path == "" {
		return Job{}, false
	}
	return cakeJob(l, d), true
}

// cakeJob maps a listing row and its detail onto a Job.
func cakeJob(l cakeListing, d cakeDetail) Job {
	var codes []string
	for _, loc := range d.Locations {
		codes = append(codes, loc.CountryCode)
	}
	j := Job{
		ExternalID:         l.Path,
		URL:                cakePostingURL(l),
		Title:              strings.TrimSpace(firstNonEmpty(d.Title, l.Title)),
		Company:            strings.TrimSpace(l.Page.Name),
		Location:           distinctJoin(l.Locations, "; ", func(x cakeLocale) string { return x.En }),
		Description:        sanitizeHTML(d.Description + headedSection("Requirements", d.Requirements)),
		WorkMode:           cakeWorkMode(d.Remote),
		Remote:             d.Remote == "full_remote_work",
		EmploymentType:     cakeEmploymentType(d.JobType),
		Seniority:          cakeSeniority(d.Seniority),
		ExperienceYearsMin: d.MinExperience,
		Countries:          countriesFromCodes(codes),
		// The platform files every IT role under the `it_` family; measured on all 49 of its
		// leaves, each is a technical craft (back-end, firmware, IC design, QA, ...).
		IsTechHint: strings.HasPrefix(d.Profession, "it_"),
	}
	if !d.CreatedAt.IsZero() {
		t := d.CreatedAt
		j.PostedAt = &t
	}
	cakeSalary(&j, d)
	return j
}

// cakeSalary states the posting's own salary unless the employer hid it.
func cakeSalary(j *Job, d cakeDetail) {
	period := map[string]string{"per_year": "year", "per_month": "month", "per_day": "day", "per_hour": "hour"}[d.SalaryType]
	if d.HideSalary || period == "" || d.Currency == "" {
		return
	}
	minV, _ := strconv.ParseFloat(d.SalaryMin, 64)
	maxV, _ := strconv.ParseFloat(d.SalaryMax, 64)
	j.SalaryMin = roundSalaryPart(minV)
	if !d.HideSalaryMax {
		j.SalaryMax = roundSalaryPart(maxV)
	}
	if j.SalaryMin == nil && j.SalaryMax == nil {
		return
	}
	j.SalaryCurrency = strings.ToUpper(d.Currency)
	j.SalaryPeriod = period
}

func cakeWorkMode(remote string) string {
	switch remote {
	case "full_remote_work":
		return "remote"
	case "partial_remote_work", "optional_remote_work":
		return "hybrid"
	case "no_remote_work":
		return "onsite"
	default:
		return ""
	}
}

func cakeEmploymentType(t string) string {
	switch t {
	case "full_time", "part_time", "internship", "contract":
		return t
	case "freelance", "temporary":
		return "contract"
	default:
		return ""
	}
}

// cakeSeniority maps only the levels that name one of ours. "associate" and
// "mid_senior_level" each span two of ours, and "director" names a role rather than a level,
// so those are left to the title dictionary.
func cakeSeniority(s string) string {
	switch s {
	case "internship_level":
		return "intern"
	case "entry_level":
		return "junior"
	case "executive":
		return "c_level"
	default:
		return ""
	}
}
