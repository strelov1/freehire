package sources

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// hcshiring adapts the symplr (formerly HealthcareSource) applicant career centers served at
// <tenant>.hcshiring.com — a US healthcare ATS behind senior-living communities, rehab centres
// and physician groups. The board is the tenant subdomain.
//
// The career center is an Ember single-page app over a keyless REST API on the same host:
//
//	GET /api/jobs?page=N                 {"jobs":[...], "meta":{"totalJobs","totalPages",...}}
//	GET /api/jobDescriptions?id=<id>     {"jobDescriptions":{"description": "<html>", ...}}
//
// The listing is fixed at ten rows a page (no parameter widens it) and carries everything about
// a posting except its body: title, city/state/zip, the date it opened and the tenant's public
// fields. The body lives only on the per-posting call, so the adapter hydrates
// (HydratingSource): a detail request is spent on a posting the catalogue does not already have,
// never on re-listing one it has. The app's own path prefix (/<build>/api) is optional — the
// bare /api answers identically and does not move with each release.
type hcshiring struct {
	http JSONGetter
}

// NewHCSHiring builds the HCS Hiring adapter over the given HTTP client.
func NewHCSHiring(c JSONGetter) Source { return hcshiring{http: c} }

func (hcshiring) Provider() string { return "hcshiring" }

// fullBoardListing: the walk reads every page up to the listing's own totalPages and then
// requires the distinct ids read to equal its totalJobs. Any page failing, or the page ceiling
// being reached first, fails the crawl rather than reporting a partial board as complete.
func (hcshiring) fullBoardListing() {}

const (
	hcsApex = ".hcshiring.com"
	// hcsMaxPages is a ceiling, not an expected end: at ten rows a page it admits 5,000
	// postings, and reaching it fails the crawl.
	hcsMaxPages = 500
)

// hcsBaseURL is the tenant's career-center origin.
func hcsBaseURL(board string) string {
	return "https://" + strings.ToLower(strings.TrimSpace(board)) + hcsApex
}

// hcsListPage is one page of the listing. Jobs is a pointer so a response without the array
// (an error document) is told apart from an empty board.
type hcsListPage struct {
	Jobs *[]hcsListing `json:"jobs"`
	Meta struct {
		TotalJobs  int `json:"totalJobs"`
		TotalPages int `json:"totalPages"`
	} `json:"meta"`
}

// hcsListing is one listing row. Only the fields the adapter maps are decoded; the structured
// compensation and Google employment-type fields the record also declares are null on every
// posting, so they are not read.
type hcsListing struct {
	ID             string        `json:"id"`
	Title          string        `json:"title"`
	IsInternalOnly bool          `json:"isInternalOnly"`
	City           string        `json:"city"`
	State          string        `json:"state"`
	Zip            string        `json:"zip"`
	LastOpening    string        `json:"lastOpening"`
	PublicFields   []hcsPubField `json:"publicFieldValues"`
}

// hcsPubField is one of the tenant's public custom fields. Name is whatever the tenant typed;
// StandardName is set when the tenant mapped the field onto one of the platform's own standard
// slots, which is what makes it readable across tenants.
type hcsPubField struct {
	Name         string `json:"name"`
	Value        string `json:"value"`
	StandardName string `json:"standardName"`
}

// hcsDetail is the per-posting call's body. An empty description is the platform's way of
// sending the visitor straight to the application, so it carries nothing to hydrate with.
type hcsDetail struct {
	JobDescriptions struct {
		Description string `json:"description"`
	} `json:"jobDescriptions"`
}

// Fetch is the fallback HydratingSource specifies for a caller that cannot supply a seen set;
// it hydrates every posting. FetchNew is the path cmd/ingest drives.
func (s hcshiring) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew is the hydrating crawl: it walks the whole listing, re-lists a posting the catalogue
// already has as a liveness refresh, and fetches the body only for one it does not.
func (s hcshiring) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	base := hcsBaseURL(e.Board)
	rows, err := s.list(ctx, base)
	if err != nil {
		return nil, fmt.Errorf("hcshiring: board %s: %w", e.Board, err)
	}
	return fetchDetails(rows, defaultDetailWorkers, func(r hcsListing) (Job, bool) {
		job := r.toJob(base, e.Company)
		if seen(job.ExternalID) {
			job.SeenRefresh = true
			return job, true
		}
		var d hcsDetail
		endpoint := base + "/api/jobDescriptions?id=" + url.QueryEscape(r.ID)
		if err := s.http.GetJSON(ctx, endpoint, &d); err != nil {
			if !detailUnreadable(err) {
				return Job{}, false // the platform says the posting is gone
			}
			// The listing is authoritative for the posting's existence; a body that could
			// not be read this run is retried by the pipeline's hydration window.
			log.Printf("hcshiring: detail %s/%s failed; ingesting list-only: %v", e.Board, r.ID, err)
			return job, true
		}
		job.Description = sanitizeHTML(d.JobDescriptions.Description)
		return job, true
	}), nil
}

// list walks the listing page by page to its own totalPages and proves the read complete by
// the distinct ids equalling totalJobs. Internal-only rows are counted toward that proof but
// not returned: they are open to the tenant's employees alone.
func (s hcshiring) list(ctx context.Context, base string) ([]hcsListing, error) {
	var rows []hcsListing
	seen := map[string]bool{}
	total := 0
	for page := 1; ; page++ {
		if page > hcsMaxPages {
			return nil, fmt.Errorf("page ceiling %d reached at %d of %d postings", hcsMaxPages, len(seen), total)
		}
		var resp hcsListPage
		if err := s.http.GetJSON(ctx, base+"/api/jobs?page="+strconv.Itoa(page), &resp); err != nil {
			return nil, fmt.Errorf("listing page %d: %w", page, err)
		}
		if resp.Jobs == nil {
			return nil, fmt.Errorf("listing page %d: response carries no jobs array", page)
		}
		total = resp.Meta.TotalJobs
		for _, r := range *resp.Jobs {
			if r.ID == "" || seen[r.ID] {
				continue
			}
			seen[r.ID] = true
			if !r.IsInternalOnly {
				rows = append(rows, r)
			}
		}
		if len(*resp.Jobs) == 0 || page >= resp.Meta.TotalPages {
			break
		}
	}
	if len(seen) != total {
		return nil, fmt.Errorf("read %d distinct postings, listing declares %d", len(seen), total)
	}
	return rows, nil
}

// toJob maps a listing row onto the identity every path shares — a hydrated posting, a
// list-only one, and a liveness refresh alike.
func (r hcsListing) toJob(base, company string) Job {
	// "City, ST ZIP": the ZIP is what lets the location dictionary read a state code that is
	// also a country code ("Hershey, PA") as the US state it is.
	location := joinNonEmpty(strings.TrimSpace(r.City), strings.Join(strings.Fields(r.State+" "+r.Zip), " "))
	salaryMin, salaryMax, period := r.salary()
	var currency string
	if period != "" {
		currency = "USD"
	}
	return Job{
		ExternalID: r.ID,
		URL:        base + "/jobs/" + url.PathEscape(r.ID),
		Title:      strings.Join(strings.Fields(r.Title), " "),
		Company:    company,
		Location:   location,
		// The platform's own JobPosting markup states addressCountry "US" for every posting
		// and USD for every salary; the product serves US employers only.
		Countries:      []string{"us"},
		Remote:         isRemote(location),
		PostedAt:       parseRFC3339(r.LastOpening),
		EmploymentType: r.employmentType(),
		SalaryMin:      salaryMin,
		SalaryMax:      salaryMax,
		SalaryCurrency: currency,
		SalaryPeriod:   period,
	}
}

// employmentType reads the platform's standard Status field. Its value is tenant-typed, so
// only a value naming exactly one arrangement is mapped: "PRN", "Per Diem", "Weekends Only"
// and combined values ("Full-time Part-time") have no single reading and are left to the
// description parser.
func (r hcsListing) employmentType() string {
	for _, f := range r.PublicFields {
		if f.StandardName != "Status" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(f.Value)) {
		case "full-time", "full time":
			return "full_time"
		case "part-time", "part time":
			return "part_time"
		}
		return ""
	}
	return ""
}

// hcsSalaryPattern is the platform's own rendering of a tenant's Salary field: two-decimal
// bounds and a pay frequency ("38.50 - 55.71 Hourly", "110000.00 Annually"). Anything else is
// free text a tenant typed and is not read.
var hcsSalaryPattern = regexp.MustCompile(`^(\d+(?:\.\d+)?)(?:\s*-\s*(\d+(?:\.\d+)?))?\s+([A-Za-z]+)$`)

// salary maps the Salary public field onto the structured salary fields, returning all empty
// when the field is absent, unparseable, or in a frequency freehire has no period for.
func (r hcsListing) salary() (salaryMin, salaryMax *int, period string) {
	for _, f := range r.PublicFields {
		if strings.EqualFold(strings.TrimSpace(f.Name), "Salary") {
			return parseHCSSalary(f.Value)
		}
	}
	return nil, nil, ""
}

// parseHCSSalary parses one Salary value. A single figure states both bounds.
func parseHCSSalary(v string) (lo, hi *int, period string) {
	m := hcsSalaryPattern.FindStringSubmatch(strings.TrimSpace(v))
	if m == nil {
		return nil, nil, ""
	}
	switch strings.ToLower(m[3]) {
	case "hourly":
		period = "hour"
	case "daily":
		period = "day"
	case "monthly":
		period = "month"
	case "annually", "yearly":
		period = "year"
	default:
		return nil, nil, "" // "Weekly" and the like: no salary_period value to state it in
	}
	low, _ := strconv.ParseFloat(m[1], 64)
	high := low
	if m[2] != "" {
		high, _ = strconv.ParseFloat(m[2], 64)
	}
	lo, hi = roundSalaryPart(low), roundSalaryPart(high)
	if lo == nil || hi == nil {
		return nil, nil, ""
	}
	return lo, hi, period
}
